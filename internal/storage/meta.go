package storage

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Ключи schema_meta (docs/plan/02-database.md §2.1).
const (
	metaSchemaVersion   = "schema_version"
	metaCatalogRevision = "catalog_revision"
	metaDataRevision    = "data_revision"
)

// EnsureCreated выполняет полный DDL (CREATE … IF NOT EXISTS) и записывает
// schema_version, catalog_revision и data_revision, если их ещё нет
// (docs/plan/02-database.md §5). Гонка-безопасна: вставка меты подавляет
// конфликт уникальности атомарно (ON CONFLICT DO NOTHING) — конкурентное
// создание той же меты ошибки не даёт. Возвращает true, если база
// создана этим вызовом (мета была вставлена).
func (d *DB) EnsureCreated(ctx context.Context) (bool, error) {
	tx, err := d.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции

	for _, stmt := range ddlFor(d.dialect) {
		for attempt := 0; ; attempt++ {
			_, err := tx.exec(ctx, stmt, nil)
			if err == nil {
				break
			}
			// Гонка параллельного EnsureCreated: CREATE … IF NOT EXISTS не
			// атомарно в проверке существования — конкурент успел создать
			// объект, повтор оператора видит его созданным.
			if attempt < 3 && isAlreadyExistsError(err) {
				continue
			}
			return false, domain.NewErrorf(domain.CodeStorageDDLFailed,
				domain.MsgStorageDDL, err.Error())
		}
	}
	created := false
	for key, value := range map[string]string{
		metaSchemaVersion:   strconv.Itoa(SchemaVersion),
		metaCatalogRevision: "1",
		metaDataRevision:    "1",
	} {
		res, err := tx.exec(ctx, `
INSERT INTO schema_meta(key, value)
SELECT @key, @value
WHERE NOT EXISTS (SELECT 1 FROM schema_meta WHERE key = @key)
ON CONFLICT DO NOTHING`,
			map[string]any{"key": key, "value": value})
		if err != nil {
			return false, err
		}
		if n, _ := res.RowsAffected(); n > 0 && key == metaSchemaVersion {
			created = true
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if err := d.CheckSchema(ctx); err != nil {
		return false, err
	}
	return created, nil
}

// CheckSchema читает schema_meta.schema_version и сверяет с версией модуля
// (docs/plan/02-database.md §5.2): отсутствие таблицы — database_not_initialized
// (не инициализирована, чужая или повреждённая база), несовпадение версии —
// schema_version_mismatch; продолжение работы запрещено.
func (d *DB) CheckSchema(ctx context.Context) error {
	var version string
	err := d.queryRow(ctx, `SELECT value FROM schema_meta WHERE key = @key`,
		map[string]any{"key": metaSchemaVersion}).Scan(&version)
	if err != nil {
		if isNoTableError(err) {
			return domain.DatabaseNotInitialized()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DatabaseNotInitialized()
		}
		return err
	}
	v, convErr := strconv.Atoi(strings.TrimSpace(version))
	if convErr != nil {
		return domain.DatabaseNotInitialized()
	}
	if v != SchemaVersion {
		return domain.SchemaVersionMismatch(v, SchemaVersion)
	}
	return nil
}

// isNoTableError распознаёт отсутствие таблицы в обоих диалектах:
// postgres — только по SQLState 42P01 (undefined_table) — тексты других
// ошибок «… does not exist» (например 42703 undefined_column) таблицей
// не являются; sqlite-драйвер SQLState не даёт — по тексту ошибки.
func isNoTableError(err error) bool {
	if err == nil {
		return false
	}
	var pg interface{ SQLState() string }
	if errors.As(err, &pg) {
		return pg.SQLState() == "42P01"
	}
	return strings.Contains(err.Error(), "no such table")
}

// isAlreadyExistsError распознаёт «объект уже существует» — ретрай гонки
// параллельного EnsureCreated: postgres — SQLState 42P07 (duplicate_table,
// в том числе индекс) и 42710 (duplicate_object), sqlite — по тексту.
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	var pg interface{ SQLState() string }
	if errors.As(err, &pg) {
		return pg.SQLState() == "42P07" || pg.SQLState() == "42710"
	}
	return strings.Contains(err.Error(), "already exists")
}

// Revisions возвращает текущие счётчики catalog_revision и data_revision
// (для ETag REST — docs/plan/02-database.md §5.5).
func (d *DB) Revisions(ctx context.Context) (catalog, data int64, err error) {
	return readRevisions(ctx, d.reads)
}

// Revisions в текущей транзакции — согласованные агрегаты (один снимок
// чтения с прочими чтениями вызывающего, например /stats).
func (t *Tx) Revisions(ctx context.Context) (catalog, data int64, err error) {
	return readRevisions(ctx, t.tx)
}

func readRevisions(ctx context.Context, q queryer) (catalog, data int64, err error) {
	rows, err := queryContext(ctx, q,
		`SELECT key, value FROM schema_meta WHERE key IN (@a, @b)`,
		map[string]any{"a": metaCatalogRevision, "b": metaDataRevision})
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return 0, 0, err
		}
		v, cerr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if cerr != nil {
			return 0, 0, cerr
		}
		switch key {
		case metaCatalogRevision:
			catalog = v
		case metaDataRevision:
			data = v
		}
	}
	return catalog, data, rows.Err()
}

// CatalogRevision возвращает catalog_revision — проверка кэша каталога на
// каждом обращении к сервисам (единственный SELECT schema_meta).
func (d *DB) CatalogRevision(ctx context.Context) (int64, error) {
	return catalogRevision(ctx, d.reads)
}

// CatalogRevision в текущей транзакции — перепроверка свежести снимка,
// по которому валидировалась запись (валидированное = записанному).
func (t *Tx) CatalogRevision(ctx context.Context) (int64, error) {
	return catalogRevision(ctx, t.tx)
}

// catalogRevision читает catalog_revision одним SELECT schema_meta —
// из пула чтения или внутри транзакции чтения снимка каталога
// (ревизия и определения — одно согласованное чтение).
func catalogRevision(ctx context.Context, q queryer) (int64, error) {
	var value string
	err := queryRowContext(ctx, q,
		`SELECT value FROM schema_meta WHERE key = @key`,
		map[string]any{"key": metaCatalogRevision}).Scan(&value)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
}

// bumpRevision инкрементирует счётчик ревизий атомарно внутри транзакции
// записи (docs/plan/02-database.md §5.5); CAST переносим между диалектами
// (value — TEXT). Отсутствие строки schema_meta — повреждение базы:
// молчаливый ноль инкремента заморозил бы ETag и кэш без диагностики.
func (t *Tx) bumpRevision(ctx context.Context, key string) error {
	res, err := t.exec(ctx,
		`UPDATE schema_meta SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT) WHERE key = @key`,
		map[string]any{"key": key})
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.NewErrorf(domain.CodeInternal, domain.MsgStorageRevisionRowMissing, key)
	}
	return nil
}

// BumpCatalogRevision инкрементирует catalog_revision (транзакция меняет
// каталог).
func (t *Tx) BumpCatalogRevision(ctx context.Context) error {
	return t.bumpRevision(ctx, metaCatalogRevision)
}

// BumpDataRevision инкрементирует data_revision (транзакция меняет
// устройства).
func (t *Tx) BumpDataRevision(ctx context.Context) error {
	return t.bumpRevision(ctx, metaDataRevision)
}
