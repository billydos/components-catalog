package storage

import (
	"database/sql"
	"fmt"
	"strings"

	"soviettransistors/internal/domain"
)

// Центральные спецификации чтения: из одного массива собирается и текст SELECT,
// и разрешение ординалов по именам (rowReader) — порядок колонок в запросе не
// влияет на чтение, а рассинхронизация SELECT и материализации невозможна.
var (
	transistorColumns = []string{"Material", "Subclass", "Assembly", "Feature", "DevNumber", "Letters", "Modification", "ChipVariant"}

	// список полей атрибутов — не копия, а domain.AttributeFieldNames (единый
	// источник вместе с chk_any_attribute и проверкой «задано хоть одно поле»)
	attributeColumns = domain.AttributeFieldNames()

	ratingColumns = []string{
		"UkeMax", "UkbMax", "UbeMax", "UkeoMax", "IkMax", "IbMax", "PkMax",
		"IkPulseMax", "PkPulseMax", "PulseDuration",
		"TempMin", "TempMax", "TempJunctionMax", "Rth",
	}

	// условия измерения — из того же источника, из которого собирается DDL
	// electrical_parameters
	conditionColumns = buildConditionColumns()

	parameterColumns = append([]string{"Parameter", "ValueMin", "ValueMax"}, conditionColumns...)
)

func buildConditionColumns() []string {
	columns := make([]string, 0, len(domain.AllConditionKeys)+1)
	for _, key := range domain.AllConditionKeys {
		columns = append(columns, string(key))
	}
	return append(columns, "Temp")
}

func selectFrom(table string, columns []string) string {
	return "SELECT " + strings.Join(columns, ", ") + " FROM " + table
}

// Dialect — диалектозависимые элементы: соединение (PRAGMA и т.п.), DDL и запрос
// id последней вставленной строки. Для новой СУБД достаточно задать другую
// реализацию; остальной DML менять не нужно.
type Dialect struct {
	Open            func() (*sql.DB, error)
	CreateTableSql  string
	LastInsertIdSql string
}

// RelationalStore — переносимая реляционная реализация справочника.
// Многошаговые операции записи (Save, Delete) выполняются в транзакции
// соединения: сбой в середине не оставляет частично применённую запись.
type RelationalStore struct {
	dialect Dialect
	db      *sql.DB
}

func NewRelationalStore(dialect Dialect) *RelationalStore {
	return &RelationalStore{dialect: dialect}
}

// connection — единственное соединение экземпляра: создаётся при первом
// обращении и переиспользуется всеми методами до Close.
func (s *RelationalStore) connection() (*sql.DB, error) {
	if s.db == nil {
		db, err := s.dialect.Open()
		if err != nil {
			return nil, err
		}
		s.db = db
	}
	return s.db, nil
}

func (s *RelationalStore) Close() error {
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *RelationalStore) EnsureCreated() error {
	db, err := s.connection()
	if err != nil {
		return err
	}
	_, err = db.Exec(s.dialect.CreateTableSql)
	return err
}

// queryBuilder собирает текст SQL и именованные параметры @имя в одном месте.
type queryBuilder struct {
	sql  strings.Builder
	args []any
}

func (b *queryBuilder) named(name string, value any) {
	b.args = append(b.args, sql.Named(name, value))
}

func (s *RelationalStore) Save(transistor domain.Transistor, details *TransistorDetails) (UpsertOutcome, error) {
	if problems := domain.ValidateTransistor(transistor); len(problems) > 0 {
		return 0, domain.NewUserError("некорректный транзистор «%s»: %s", transistor.Name(), strings.Join(problems, "; "))
	}
	if err := validateDetails(transistor, details); err != nil {
		return 0, err
	}

	// «секций нет» ⟺ details nil либо все секции пусты: запись без секций либо
	// вставляется, либо пропускается — id и чистка сирот в этих путях не нужны.
	hasSections := details != nil && (details.Attributes != nil || details.Manufacturers != nil ||
		details.Parameters != nil || details.Ratings != nil)

	db, err := s.connection()
	if err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var id int
	var outcome UpsertOutcome
	inserted, err := insertTransistorIfAbsent(tx, transistor)
	if err != nil {
		return 0, err
	}
	switch {
	case inserted:
		outcome = UpsertAdded
		if hasSections {
			if id, err = s.lastInsertId(tx); err != nil {
				return 0, err
			}
		}
	case !hasSections:
		outcome = UpsertSkipped
	default:
		foundId, found, err := findIdCore(tx, transistor)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, domain.NewUserError("не удалось определить id записи «%s»", transistor.Name())
		}
		id = foundId
		outcome = UpsertUpdatedExisting
	}

	if details != nil {
		if details.Attributes != nil {
			if domain.AttributesHaveAnyValue(details.Attributes) {
				if err = setAttributes(tx, id, details.Attributes); err != nil {
					return 0, err
				}
			} else if details.Manufacturers == nil {
				// секция задана пустой (без manufacturers) — очистить атрибуты;
				// если задан только manufacturers — атрибуты не трогаем
				if err = clearAttributes(tx, id); err != nil {
					return 0, err
				}
			}
		}
		if details.Manufacturers != nil {
			if err = setManufacturers(tx, id, details.Manufacturers); err != nil {
				return 0, err
			}
		}
		if details.Parameters != nil {
			if err = replaceParameters(tx, id, details.Parameters); err != nil {
				return 0, err
			}
		}
		if details.Ratings != nil {
			if err = setRatings(tx, id, details.Ratings); err != nil {
				return 0, err
			}
		}
	}

	if hasSections {
		// применялись секции — могла отвязаться ссылка производителя
		if err = deleteOrphanManufacturers(tx); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return outcome, nil
}

func (s *RelationalStore) FindId(transistor domain.Transistor) (int, bool, error) {
	db, err := s.connection()
	if err != nil {
		return 0, false, err
	}
	return findIdCore(db, transistor)
}

func findIdCore(db queryExecer, transistor domain.Transistor) (int, bool, error) {
	var b queryBuilder
	b.sql.WriteString("SELECT Id FROM transistors WHERE ")
	b.sql.WriteString(exactMatchSql(&b, transistor))
	row := db.QueryRow(b.sql.String(), b.args...)
	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}
	return int(id), true, nil
}

func (s *RelationalStore) FindMaterialEquivalents(transistor domain.Transistor) ([]domain.Transistor, error) {
	db, err := s.connection()
	if err != nil {
		return nil, err
	}
	var b queryBuilder
	b.sql.WriteString(selectFrom("transistors", transistorColumns) + " WHERE ")
	appendMaterialCounterpartMatch(&b, transistor)
	return queryTransistors(db, b.sql.String(), b.args...)
}

func (s *RelationalStore) Delete(transistor domain.Transistor) (bool, error) {
	db, err := s.connection()
	if err != nil {
		return false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var b queryBuilder
	b.sql.WriteString("DELETE FROM transistors WHERE ")
	b.sql.WriteString(exactMatchSql(&b, transistor))
	result, err := tx.Exec(b.sql.String(), b.args...)
	if err != nil {
		return false, err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if removed > 0 {
		if err = deleteOrphanManufacturers(tx); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return removed > 0, nil
}

func (s *RelationalStore) CountAll() (int, error) {
	db, err := s.connection()
	if err != nil {
		return 0, err
	}
	var count int64
	if err = db.QueryRow("SELECT COUNT(*) FROM transistors").Scan(&count); err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *RelationalStore) Query(query TransistorQuery) ([]domain.Transistor, error) {
	db, err := s.connection()
	if err != nil {
		return nil, err
	}

	var b queryBuilder
	b.sql.WriteString(selectFrom("transistors", transistorColumns))

	var conditions []string
	if query.Material != nil {
		letter, digit := domain.MaterialSymbolsOf(*query.Material)
		conditions = append(conditions, "Material IN (@matLetter, @matDigit)")
		b.named("matLetter", string(letter))
		b.named("matDigit", string(digit))
	}
	if query.Subclass != nil {
		conditions = append(conditions, "Subclass = @subclass")
		b.named("subclass", string(*query.Subclass))
	}
	if query.IsAssembly != nil {
		conditions = append(conditions, "Assembly = @assembly")
		if *query.IsAssembly {
			b.named("assembly", 1)
		} else {
			b.named("assembly", 0)
		}
	}
	if query.Feature != nil {
		conditions = append(conditions, "Feature = @feature")
		b.named("feature", *query.Feature)
	}
	if query.DevelopmentNumber != nil {
		conditions = append(conditions, "DevNumber = @dev_number")
		b.named("dev_number", *query.DevelopmentNumber)
	}
	if query.Letters != "" {
		conditions = append(conditions, "Letters = @letters")
		b.named("letters", query.Letters)
	}
	if query.Modification != nil {
		conditions = append(conditions, "Modification = @modification")
		b.named("modification", *query.Modification)
	}
	if query.ChipVariant != nil {
		conditions = append(conditions, "ChipVariant = @chip")
		b.named("chip", *query.ChipVariant)
	}
	if len(conditions) > 0 {
		b.sql.WriteString(" WHERE " + strings.Join(conditions, " AND "))
	}

	b.sql.WriteString(" ORDER BY Subclass, Feature, DevNumber, Letters, Material, Modification, ChipVariant")

	if query.Limit != nil && *query.Limit > 0 {
		b.sql.WriteString(" LIMIT @limit")
		b.named("limit", *query.Limit)
	}

	return queryTransistors(db, b.sql.String(), b.args...)
}

func (s *RelationalStore) GetAttributes(transistorId int) (*domain.TransistorAttributes, error) {
	db, err := s.connection()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(selectFrom("transistor_attributes", attributeColumns)+" WHERE TransistorId = @id", sql.Named("id", transistorId))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reader, err := newRowReader(rows, attributeColumns)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := reader.scan(rows); err != nil {
		return nil, err
	}
	return &domain.TransistorAttributes{
		Structure:         reader.getStringOrNull("Structure"),
		Technology:        reader.getStringOrNull("Technology"),
		Package:           reader.getStringOrNull("Package"),
		PackageMaterial:   reader.getStringOrNull("PackageMaterial"),
		ColorMarking:      reader.getStringOrNull("ColorMarking"),
		Pinout:            reader.getStringOrNull("Pinout"),
		EsdSensitive:      reader.getBoolOrNull("EsdSensitive"),
		MilitaryGrade:     reader.getBoolOrNull("MilitaryGrade"),
		RadiationHardened: reader.getBoolOrNull("RadiationHardened"),
		Tu:                reader.getStringOrNull("Tu"),
		Notes:             reader.getStringOrNull("Notes"),
		YearFrom:          reader.getIntOrNull("YearFrom"),
		YearTo:            reader.getIntOrNull("YearTo"),
		MassMax:           reader.getFloatOrNull("MassMax"),
		DatasheetUrl:      reader.getStringOrNull("DatasheetUrl"),
	}, nil
}

func (s *RelationalStore) GetManufacturers(transistorId int) ([]string, error) {
	db, err := s.connection()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
            SELECT m.Name
            FROM manufacturers m
            JOIN transistor_manufacturers tm ON tm.ManufacturerId = m.Id
            WHERE tm.TransistorId = @id
            ORDER BY tm.ManufacturerId`, sql.Named("id", transistorId))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (s *RelationalStore) GetParameters(transistorId int) ([]domain.ElectricalParameter, error) {
	db, err := s.connection()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(selectFrom("electrical_parameters", parameterColumns)+" WHERE TransistorId = @id ORDER BY Id", sql.Named("id", transistorId))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reader, err := newRowReader(rows, parameterColumns)
	if err != nil {
		return nil, err
	}
	var parameters []domain.ElectricalParameter
	for rows.Next() {
		if err := reader.scan(rows); err != nil {
			return nil, err
		}
		code := reader.getString("Parameter")
		kind, ok := domain.TryParameterKindByCode(code)
		if !ok {
			return nil, domain.NewUserError("в базе найден неизвестный код параметра «%s»", code)
		}
		parameters = append(parameters, domain.ElectricalParameter{
			Kind:     kind,
			ValueMin: reader.getFloatOrNull("ValueMin"),
			ValueMax: reader.getFloatOrNull("ValueMax"),
			Uke:      reader.getFloatOrNull("Uke"),
			Ukb:      reader.getFloatOrNull("Ukb"),
			Ueb:      reader.getFloatOrNull("Ueb"),
			Ik:       reader.getFloatOrNull("Ik"),
			Ie:       reader.getFloatOrNull("Ie"),
			Ib:       reader.getFloatOrNull("Ib"),
			Freq:     reader.getFloatOrNull("Freq"),
			Rg:       reader.getFloatOrNull("Rg"),
			Rbe:      reader.getFloatOrNull("Rbe"),
			Temp:     reader.getFloatOrNull("Temp"),
		})
	}
	return parameters, rows.Err()
}

func (s *RelationalStore) GetRatings(transistorId int) (*domain.MaximumRatings, error) {
	db, err := s.connection()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(selectFrom("maximum_ratings", ratingColumns)+" WHERE TransistorId = @id", sql.Named("id", transistorId))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reader, err := newRowReader(rows, ratingColumns)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := reader.scan(rows); err != nil {
		return nil, err
	}
	return &domain.MaximumRatings{
		UkeMax:          reader.getFloatOrNull("UkeMax"),
		UkbMax:          reader.getFloatOrNull("UkbMax"),
		UbeMax:          reader.getFloatOrNull("UbeMax"),
		UkeoMax:         reader.getFloatOrNull("UkeoMax"),
		IkMax:           reader.getFloatOrNull("IkMax"),
		IbMax:           reader.getFloatOrNull("IbMax"),
		PkMax:           reader.getFloatOrNull("PkMax"),
		IkPulseMax:      reader.getFloatOrNull("IkPulseMax"),
		PkPulseMax:      reader.getFloatOrNull("PkPulseMax"),
		PulseDuration:   reader.getFloatOrNull("PulseDuration"),
		TempMin:         reader.getFloatOrNull("TempMin"),
		TempMax:         reader.getFloatOrNull("TempMax"),
		TempJunctionMax: reader.getFloatOrNull("TempJunctionMax"),
		Rth:             reader.getFloatOrNull("Rth"),
	}, nil
}

// queryExecer — общая часть *sql.DB и *sql.Tx для QueryRow.
type queryExecer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// lastInsertId возвращает id строки, вставленной в этом соединении последней
// (сразу после INSERT). Диалектозависимый запрос — единственный такой элемент DML.
func (s *RelationalStore) lastInsertId(tx *sql.Tx) (int, error) {
	var id int64
	if err := tx.QueryRow(s.dialect.LastInsertIdSql).Scan(&id); err != nil {
		return 0, domain.NewUserError("не удалось получить id вставленной записи")
	}
	return int(id), nil
}

func validateDetails(transistor domain.Transistor, details *TransistorDetails) error {
	if details == nil {
		return nil
	}
	if details.Attributes != nil {
		if errors := domain.ValidateTransistorAttributes(details.Attributes); len(errors) > 0 {
			return domain.NewUserError("«%s», атрибуты: %s", transistor.Name(), strings.Join(errors, "; "))
		}
	}
	for _, manufacturer := range details.Manufacturers {
		if strings.TrimSpace(manufacturer) == "" {
			return domain.NewUserError("«%s»: название производителя не может быть пустым", transistor.Name())
		}
	}
	for _, parameter := range details.Parameters {
		if errors := domain.ValidateElectricalParameter(parameter); len(errors) > 0 {
			code := domain.ParameterInfoOf(parameter.Kind).Code
			return domain.NewUserError("«%s», параметр %s: %s", transistor.Name(), code, strings.Join(errors, "; "))
		}
	}
	if details.Ratings != nil {
		if errors := domain.ValidateMaximumRatings(details.Ratings); len(errors) > 0 {
			return domain.NewUserError("«%s», предельные данные: %s", transistor.Name(), strings.Join(errors, "; "))
		}
	}
	return nil
}

// insertTransistorIfAbsent — вставка «если нет точной записи»: одна команда
// вместо «найти, затем вставить» — без гонки на дубликат и без лишнего SELECT.
// Предикат тождества совпадает с UNIQUE-ограничением uq_transistor.
func insertTransistorIfAbsent(tx *sql.Tx, transistor domain.Transistor) (bool, error) {
	var b queryBuilder
	b.sql.WriteString(`
            INSERT INTO transistors
                (Material, Subclass, Assembly, Feature, DevNumber, Letters, Modification, ChipVariant)
            SELECT @material, @subclass, @assembly, @feature, @dev_number, @letters, @modification, @chip_variant
            FROM (SELECT 1) AS src
            WHERE NOT EXISTS (SELECT 1 FROM transistors WHERE ` + " " + exactMatchSql(&b, transistor) + ")")
	b.named("material", string(transistor.Material))
	b.named("subclass", string(transistor.Subclass))
	if transistor.IsAssembly {
		b.named("assembly", 1)
	} else {
		b.named("assembly", 0)
	}
	b.named("feature", transistor.Feature)
	b.named("dev_number", transistor.DevelopmentNumber)
	b.named("letters", transistor.Letters)
	b.named("modification", intOrNull(transistor.Modification))
	b.named("chip_variant", intOrNull(transistor.ChipVariant))
	result, err := tx.Exec(b.sql.String(), b.args...)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return inserted > 0, nil
}

func clearAttributes(tx *sql.Tx, transistorId int) error {
	_, err := tx.Exec("DELETE FROM transistor_attributes WHERE TransistorId = @id", sql.Named("id", transistorId))
	return err
}

func setAttributes(tx *sql.Tx, transistorId int, attributes *domain.TransistorAttributes) error {
	addValues := func(b *queryBuilder) {
		b.named("structure", trimmedOrNull(attributes.Structure))
		b.named("technology", trimmedOrNull(attributes.Technology))
		b.named("package", trimmedOrNull(attributes.Package))
		b.named("packageMaterial", trimmedOrNull(attributes.PackageMaterial))
		b.named("color", trimmedOrNull(attributes.ColorMarking))
		b.named("pinout", trimmedOrNull(attributes.Pinout))
		b.named("esd", intBoolOrNull(attributes.EsdSensitive))
		b.named("military", intBoolOrNull(attributes.MilitaryGrade))
		b.named("radiation", intBoolOrNull(attributes.RadiationHardened))
		b.named("tu", trimmedOrNull(attributes.Tu))
		b.named("notes", trimmedOrNull(attributes.Notes))
		b.named("yearFrom", intOrNull(attributes.YearFrom))
		b.named("yearTo", intOrNull(attributes.YearTo))
		b.named("massMax", floatOrNull(attributes.MassMax))
		b.named("url", trimmedOrNull(attributes.DatasheetUrl))
	}

	var update queryBuilder
	update.sql.WriteString(`
            UPDATE transistor_attributes
            SET Structure = @structure, Technology = @technology, Package = @package,
                PackageMaterial = @packageMaterial, ColorMarking = @color, Pinout = @pinout,
                EsdSensitive = @esd, MilitaryGrade = @military, RadiationHardened = @radiation,
                Tu = @tu, Notes = @notes, YearFrom = @yearFrom, YearTo = @yearTo,
                MassMax = @massMax, DatasheetUrl = @url
            WHERE TransistorId = @id`)
	update.named("id", transistorId)
	addValues(&update)
	result, err := tx.Exec(update.sql.String(), update.args...)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated > 0 {
		return nil
	}

	var insert queryBuilder
	insert.sql.WriteString(`
            INSERT INTO transistor_attributes
                (TransistorId, Structure, Technology, Package, PackageMaterial, ColorMarking, Pinout,
                 EsdSensitive, MilitaryGrade, RadiationHardened, Tu, Notes,
                 YearFrom, YearTo, MassMax, DatasheetUrl)
            VALUES
                (@id, @structure, @technology, @package, @packageMaterial, @color, @pinout,
                 @esd, @military, @radiation, @tu, @notes, @yearFrom, @yearTo, @massMax, @url)`)
	insert.named("id", transistorId)
	addValues(&insert)
	_, err = tx.Exec(insert.sql.String(), insert.args...)
	return err
}

func setManufacturers(tx *sql.Tx, transistorId int, manufacturers []string) error {
	if _, err := tx.Exec("DELETE FROM transistor_manufacturers WHERE TransistorId = @id", sql.Named("id", transistorId)); err != nil {
		return err
	}
	// дубликаты в списке — одно и то же имя (множество)
	seen := map[string]bool{}
	var names []string
	for _, rawName := range manufacturers {
		name := strings.TrimSpace(rawName)
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}

	// «создать имя, если его нет» + «связать с транзистором» на каждое имя
	const ensureManufacturerSql = `
            INSERT INTO manufacturers (Name)
            SELECT @name FROM (SELECT 1) AS src
            WHERE NOT EXISTS (SELECT 1 FROM manufacturers WHERE Name = @name)`
	const linkSql = `
            INSERT INTO transistor_manufacturers (TransistorId, ManufacturerId)
            SELECT @id, Id FROM manufacturers WHERE Name = @name`

	for _, name := range names {
		if _, err := tx.Exec(ensureManufacturerSql, sql.Named("name", name)); err != nil {
			return err
		}
		if _, err := tx.Exec(linkSql, sql.Named("id", transistorId), sql.Named("name", name)); err != nil {
			return err
		}
	}
	return nil
}

// deleteOrphanManufacturers — чистка бесхозных имён производителей: связи
// transistor_manufacturers удаляются при замене списка и каскадом при delete
// транзистора, а сами строки manufacturers FK-каскад не трогает — без этой
// чистки они копились бы бесконечно. NOT IN переносим; ManufacturerId NOT NULL —
// ловушки NULL в подзапросе нет.
func deleteOrphanManufacturers(tx *sql.Tx) error {
	_, err := tx.Exec("DELETE FROM manufacturers WHERE Id NOT IN (SELECT ManufacturerId FROM transistor_manufacturers)")
	return err
}

func setRatings(tx *sql.Tx, transistorId int, ratings *domain.MaximumRatings) error {
	addValues := func(b *queryBuilder) {
		b.named("uKe", floatOrNull(ratings.UkeMax))
		b.named("uKb", floatOrNull(ratings.UkbMax))
		b.named("uBe", floatOrNull(ratings.UbeMax))
		b.named("uKeo", floatOrNull(ratings.UkeoMax))
		b.named("iK", floatOrNull(ratings.IkMax))
		b.named("iB", floatOrNull(ratings.IbMax))
		b.named("pK", floatOrNull(ratings.PkMax))
		b.named("iKP", floatOrNull(ratings.IkPulseMax))
		b.named("pKP", floatOrNull(ratings.PkPulseMax))
		b.named("dur", floatOrNull(ratings.PulseDuration))
		b.named("tMin", floatOrNull(ratings.TempMin))
		b.named("tMax", floatOrNull(ratings.TempMax))
		b.named("tJ", floatOrNull(ratings.TempJunctionMax))
		b.named("rTh", floatOrNull(ratings.Rth))
	}

	var update queryBuilder
	update.sql.WriteString(`
            UPDATE maximum_ratings
            SET UkeMax = @uKe, UkbMax = @uKb, UbeMax = @uBe, UkeoMax = @uKeo,
                IkMax = @iK, IbMax = @iB, PkMax = @pK,
                IkPulseMax = @iKP, PkPulseMax = @pKP, PulseDuration = @dur,
                TempMin = @tMin, TempMax = @tMax, TempJunctionMax = @tJ, Rth = @rTh
            WHERE TransistorId = @id`)
	update.named("id", transistorId)
	addValues(&update)
	result, err := tx.Exec(update.sql.String(), update.args...)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated > 0 {
		return nil
	}

	var insert queryBuilder
	insert.sql.WriteString(`
            INSERT INTO maximum_ratings
                (TransistorId, UkeMax, UkbMax, UbeMax, UkeoMax, IkMax, IbMax, PkMax,
                 IkPulseMax, PkPulseMax, PulseDuration, TempMin, TempMax, TempJunctionMax, Rth)
            VALUES
                (@id, @uKe, @uKb, @uBe, @uKeo, @iK, @iB, @pK, @iKP, @pKP, @dur, @tMin, @tMax, @tJ, @rTh)`)
	insert.named("id", transistorId)
	addValues(&insert)
	_, err = tx.Exec(insert.sql.String(), insert.args...)
	return err
}

func replaceParameters(tx *sql.Tx, transistorId int, parameters []domain.ElectricalParameter) error {
	if _, err := tx.Exec("DELETE FROM electrical_parameters WHERE TransistorId = @id", sql.Named("id", transistorId)); err != nil {
		return err
	}
	if len(parameters) == 0 {
		return nil
	}

	const insertSql = `
            INSERT INTO electrical_parameters
                (TransistorId, Parameter, ValueMin, ValueMax, Uke, Ukb, Ueb, Ik, Ie, Ib, Freq, Rg, Rbe, Temp)
            VALUES
                (@id, @parameter, @valueMin, @valueMax, @uKe, @uKb, @uEb, @iK, @iE, @iB, @freq, @rg, @rbe, @temp)`

	for _, parameter := range parameters {
		_, err := tx.Exec(insertSql,
			sql.Named("id", transistorId),
			sql.Named("parameter", domain.ParameterInfoOf(parameter.Kind).Code),
			sql.Named("valueMin", floatOrNull(parameter.ValueMin)),
			sql.Named("valueMax", floatOrNull(parameter.ValueMax)),
			sql.Named("uKe", floatOrNull(parameter.Uke)),
			sql.Named("uKb", floatOrNull(parameter.Ukb)),
			sql.Named("uEb", floatOrNull(parameter.Ueb)),
			sql.Named("iK", floatOrNull(parameter.Ik)),
			sql.Named("iE", floatOrNull(parameter.Ie)),
			sql.Named("iB", floatOrNull(parameter.Ib)),
			sql.Named("freq", floatOrNull(parameter.Freq)),
			sql.Named("rg", floatOrNull(parameter.Rg)),
			sql.Named("rbe", floatOrNull(parameter.Rbe)),
			sql.Named("temp", floatOrNull(parameter.Temp)),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func queryTransistors(db *sql.DB, query string, args ...any) ([]domain.Transistor, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reader, err := newRowReader(rows, transistorColumns)
	if err != nil {
		return nil, err
	}
	var transistors []domain.Transistor
	for rows.Next() {
		if err := reader.scan(rows); err != nil {
			return nil, err
		}
		transistor, err := readTransistor(reader)
		if err != nil {
			return nil, err
		}
		transistors = append(transistors, transistor)
	}
	return transistors, rows.Err()
}

func readTransistor(reader *rowReader) (domain.Transistor, error) {
	material, err := reader.getRune("Material")
	if err != nil {
		return domain.Transistor{}, err
	}
	subclass, err := reader.getRune("Subclass")
	if err != nil {
		return domain.Transistor{}, err
	}
	assembly, err := reader.getBool("Assembly")
	if err != nil {
		return domain.Transistor{}, err
	}
	feature, err := reader.getInt("Feature")
	if err != nil {
		return domain.Transistor{}, err
	}
	devNumber, err := reader.getInt("DevNumber")
	if err != nil {
		return domain.Transistor{}, err
	}
	letters := reader.getString("Letters")
	return domain.Transistor{
		Material:          material,
		Subclass:          subclass,
		IsAssembly:        assembly,
		Feature:           feature,
		DevelopmentNumber: devNumber,
		Letters:           letters,
		Modification:      reader.getIntOrNull("Modification"),
		ChipVariant:       reader.getIntOrNull("ChipVariant"),
	}, nil
}

// rowReader — чтение строки по именам колонок: ординалы один раз разрешаются
// из той же спецификации колонок, из которой собран SELECT, поэтому порядок
// колонок в запросе не влияет на результат, а опечатка в имени падает громко,
// а не молча читает соседнюю колонку. Булевы читаются из INTEGER 0/1.
type rowReader struct {
	ordinals map[string]int
	values   []any
}

func newRowReader(rows *sql.Rows, columns []string) (*rowReader, error) {
	actual, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	ordinals := make(map[string]int, len(actual))
	for index, column := range actual {
		ordinals[column] = index
	}
	for _, column := range columns {
		if _, ok := ordinals[column]; !ok {
			return nil, fmt.Errorf("в SELECT нет колонки «%s»", column)
		}
	}
	return &rowReader{ordinals: ordinals, values: make([]any, len(actual))}, nil
}

func (r *rowReader) scan(rows *sql.Rows) error {
	targets := make([]any, len(r.values))
	for i := range targets {
		targets[i] = &r.values[i]
	}
	return rows.Scan(targets...)
}

func (r *rowReader) ordinal(column string) (int, error) {
	if ordinal, ok := r.ordinals[column]; ok {
		return ordinal, nil
	}
	return 0, fmt.Errorf("в SELECT нет колонки «%s»", column)
}

func (r *rowReader) rawValue(column string) (any, error) {
	ordinal, err := r.ordinal(column)
	if err != nil {
		return nil, err
	}
	return r.values[ordinal], nil
}

func (r *rowReader) getString(column string) string {
	value, _ := r.rawValue(column)
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func (r *rowReader) getRune(column string) (rune, error) {
	value, err := r.rawValue(column)
	if err != nil {
		return 0, err
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return 0, fmt.Errorf("колонка «%s» не является строкой", column)
	}
	return []rune(s)[0], nil
}

func (r *rowReader) getInt(column string) (int, error) {
	value, err := r.rawValue(column)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	}
	return 0, fmt.Errorf("колонка «%s» не является целым", column)
}

func (r *rowReader) getBool(column string) (bool, error) {
	value, err := r.getInt(column)
	if err != nil {
		return false, err
	}
	return value != 0, nil
}

func (r *rowReader) getStringOrNull(column string) *string {
	value, _ := r.rawValue(column)
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		return &s
	}
	return nil
}

func (r *rowReader) getIntOrNull(column string) *int {
	value, _ := r.rawValue(column)
	switch v := value.(type) {
	case int64:
		return domain.Ptr(int(v))
	case int:
		return domain.Ptr(v)
	}
	return nil
}

func (r *rowReader) getBoolOrNull(column string) *bool {
	if value := r.getIntOrNull(column); value != nil {
		return domain.Ptr(*value != 0)
	}
	return nil
}

func (r *rowReader) getFloatOrNull(column string) *float64 {
	value, _ := r.rawValue(column)
	if v, ok := value.(float64); ok {
		return &v
	}
	return nil
}

func intOrNull(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func floatOrNull(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func trimmedOrNull(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}

func intBoolOrNull(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}

// exactMatchSql — предикат тождества записи (совпадает с uq_transistor);
// добавляет параметры с префиксом eq в построитель.
func exactMatchSql(b *queryBuilder, t domain.Transistor) string {
	b.named("eqMaterial", string(t.Material))
	return "Material = @eqMaterial AND " + designationColumnsSql(b, t, "eq")
}

func appendMaterialCounterpartMatch(b *queryBuilder, t domain.Transistor) {
	letter, digit := domain.MaterialSymbolsOf(materialKindOf(t.Material))
	counterpart := letter
	if t.Material == letter {
		counterpart = digit
	}
	b.named("cpMaterial", string(counterpart))
	b.sql.WriteString("Material = @cpMaterial AND ")
	b.sql.WriteString(designationColumnsSql(b, t, "cp"))
}

// designationColumnsSql — обязательные колонки и Assembly (NOT NULL 0/1) —
// простое равенство; опциональные — NULL-безопасное равенство без сенти́нелов.
func designationColumnsSql(b *queryBuilder, t domain.Transistor, p string) string {
	b.named(p+"Subclass", string(t.Subclass))
	b.named(p+"Feature", t.Feature)
	b.named(p+"DevNumber", t.DevelopmentNumber)
	b.named(p+"Letters", t.Letters)
	if t.IsAssembly {
		b.named(p+"Assembly", 1)
	} else {
		b.named(p+"Assembly", 0)
	}
	b.named(p+"Modification", intOrNull(t.Modification))
	b.named(p+"ChipVariant", intOrNull(t.ChipVariant))
	return fmt.Sprintf(`
            Subclass = @%[1]sSubclass
            AND Feature = @%[1]sFeature
            AND DevNumber = @%[1]sDevNumber
            AND Letters = @%[1]sLetters
            AND Assembly = @%[1]sAssembly
            AND (Modification = @%[1]sModification OR (Modification IS NULL AND @%[1]sModification IS NULL))
            AND (ChipVariant = @%[1]sChipVariant OR (ChipVariant IS NULL AND @%[1]sChipVariant IS NULL))`, p)
}

func materialKindOf(symbol rune) domain.SemiconductorMaterial {
	kind, err := domain.MaterialKindOf(symbol)
	if err != nil {
		panic(err)
	}
	return kind
}
