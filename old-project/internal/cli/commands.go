package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"soviettransistors/internal/domain"
	"soviettransistors/internal/importer"
	"soviettransistors/internal/storage"
)

func cmdInit(dbPath string, dbSet bool) (int, error) {
	path := resolveDatabasePath(dbPath, dbSet)
	db := storage.NewSqliteStore(path)
	defer db.Close()
	if err := db.EnsureCreated(); err != nil {
		return 1, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 1, err
	}
	fmt.Printf("База данных готова: %s\n", absolute)
	return 0, nil
}

func cmdParse(names []string) (int, error) {
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "Укажите обозначение, например: parse КТ315Б")
		return 1, nil
	}
	valid, failures := parseNames(names)
	for _, transistor := range valid {
		printTransistor(transistor)
	}
	if failures == 0 {
		return 0, nil
	}
	return 1, nil
}

func cmdAdd(names []string, dbPath string, dbSet bool, dryRun bool) (int, error) {
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "Укажите обозначение, например: add КТ315Б")
		return 1, nil
	}

	valid, failures := parseNames(names)

	if dryRun {
		for _, transistor := range valid {
			fmt.Printf("(проверка) %s — обозначение корректно\n", transistor.Name())
		}
		return exitByFailures(failures), nil
	}

	added := 0
	duplicates := 0
	db, err := openDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	for _, transistor := range valid {
		// Save(transistor, nil) без секций возвращает только Added или Skipped
		if outcome, err := db.Save(transistor, nil); err != nil {
			return 1, err
		} else if outcome == storage.UpsertAdded {
			added++
			fmt.Printf("Добавлено: %s\n", transistor.Name())
		} else {
			duplicates++
			fmt.Printf("Пропущено (уже есть): %s\n", transistor.Name())
		}
	}
	fmt.Printf("Итого: добавлено %d, пропущено %d, ошибок разбора %d\n", added, duplicates, failures)
	return exitByFailures(failures), nil
}

func cmdImport(files []string, dbPath string, dbSet bool, dryRun bool) (int, error) {
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Укажите путь к файлу импорта (.jsonc, .json, .yaml, .yml), например: import sample-data.jsonc")
		return 1, nil
	}
	if len(files) > 1 {
		fmt.Fprintln(os.Stderr, "Ошибка: import принимает один файл")
		return 1, nil
	}
	path := files[0]
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка: файл не найден: %s\n", path)
		return 1, nil
	}

	parsed, err := importer.ParseFile(path)
	if err != nil {
		return 1, err
	}
	for _, issue := range parsed.Issues {
		source := ""
		if issue.Source != "" {
			source = fmt.Sprintf(" (%s)", issue.Source)
		}
		prefix := ""
		if issue.EntryIndex > 0 {
			prefix = fmt.Sprintf("Запись №%d: ", issue.EntryIndex)
		}
		fmt.Fprintf(os.Stderr, "%s%s%s\n", prefix, issue.Description, source)
	}
	fmt.Printf("Файл: %s; корректных записей: %d, проблемных: %d\n", path, len(parsed.Entries), len(parsed.Issues))

	if dryRun {
		for _, entry := range parsed.Entries {
			fmt.Printf("(проверка) %s%s\n", entry.Transistor.Name(), describeSections(entry))
		}
		return exitByErrors(parsed.HasErrors()), nil
	}

	// файл, из которого не удалось прочитать ни одной записи, не должен
	// создавать базу побочным эффектом: проблемы корня прерывают импорт
	// до обращения к БД (ANALYSIS-02 B3), как и ошибки всех записей
	if len(parsed.Entries) == 0 && parsed.HasErrors() {
		return exitByErrors(parsed.HasErrors()), nil
	}

	added := 0
	updated := 0
	skipped := 0
	db, err := openDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	for _, entry := range parsed.Entries {
		details := &storage.TransistorDetails{
			Attributes:    entry.Attributes,
			Manufacturers: entry.Manufacturers,
			Parameters:    entry.Parameters,
			Ratings:       entry.Ratings,
		}
		outcome, err := db.Save(entry.Transistor, details)
		if err != nil {
			return 1, err
		}
		switch outcome {
		case storage.UpsertAdded:
			added++
			fmt.Printf("Добавлено: %s%s\n", entry.Transistor.Name(), describeSections(entry))
		case storage.UpsertUpdatedExisting:
			updated++
			fmt.Printf("Обновлено: %s%s\n", entry.Transistor.Name(), describeSections(entry))
		case storage.UpsertSkipped:
			skipped++
			fmt.Printf("Пропущено (уже есть): %s\n", entry.Transistor.Name())
		}
	}
	fmt.Printf("Итого: добавлено %d, обновлено %d, пропущено %d\n", added, updated, skipped)
	return exitByErrors(parsed.HasErrors()), nil
}

func describeSections(entry importer.TransistorEntryData) string {
	var parts []string
	if entry.Attributes != nil {
		parts = append(parts, "атрибуты")
	}
	if entry.Manufacturers != nil {
		parts = append(parts, fmt.Sprintf("производителей %d", len(entry.Manufacturers)))
	}
	if entry.Parameters != nil {
		parts = append(parts, fmt.Sprintf("параметров %d", len(entry.Parameters)))
	}
	if entry.Ratings != nil {
		parts = append(parts, "предельные данные")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + joinComma(parts) + ")"
}

func cmdList(args []string, dbPath string, dbSet bool) (int, error) {
	allowed := []string{
		"--material", "--subclass", "--assembly", "--feature", "--number",
		"--letters", "--modification", "--chip", "--limit",
	}
	options, err := parseOptions(args, allowed)
	if err != nil {
		return 1, err
	}

	var query storage.TransistorQuery
	if value, ok := options["--material"]; ok {
		material, err := parseMaterialOption(value)
		if err != nil {
			return 1, err
		}
		query.Material = &material
	}
	if value, ok := options["--subclass"]; ok {
		subclass, err := parseSubclassOption(value)
		if err != nil {
			return 1, err
		}
		query.Subclass = &subclass
	}
	if value, ok := options["--assembly"]; ok {
		assembly, err := parseBoolOption("--assembly", value)
		if err != nil {
			return 1, err
		}
		query.IsAssembly = &assembly
	}
	if value, ok := options["--feature"]; ok {
		feature, err := parseIntOption("--feature", value, 1, 9)
		if err != nil {
			return 1, err
		}
		query.Feature = &feature
	}
	if value, ok := options["--number"]; ok {
		number, err := parseIntOption("--number", value, 1, 999)
		if err != nil {
			return 1, err
		}
		query.DevelopmentNumber = &number
	}
	if value, ok := options["--letters"]; ok {
		letters, err := parseLettersOption(value)
		if err != nil {
			return 1, err
		}
		query.Letters = letters
	}
	if value, ok := options["--modification"]; ok {
		modification, err := parseIntOption("--modification", value, 1, 9)
		if err != nil {
			return 1, err
		}
		query.Modification = &modification
	}
	if value, ok := options["--chip"]; ok {
		chip, err := parseIntOption("--chip", value, 1, 6)
		if err != nil {
			return 1, err
		}
		query.ChipVariant = &chip
	}
	if value, ok := options["--limit"]; ok {
		limit, err := parseIntOption("--limit", value, 1, maxInt())
		if err != nil {
			return 1, err
		}
		query.Limit = &limit
	}

	db, err := openExistingDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	rows, err := db.Query(query)
	if err != nil {
		return 1, err
	}
	printTable(rows)
	fmt.Printf("Записей: %d\n", len(rows))
	return 0, nil
}

func cmdInfo(names []string, dbPath string, dbSet bool) (int, error) {
	if len(names) != 1 {
		fmt.Fprintln(os.Stderr, "Укажите одно обозначение, например: info КТ315Б")
		return 1, nil
	}
	transistor, err := domain.ParseDesignation(names[0])
	if err != nil {
		return 1, err
	}
	db, err := openExistingDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	id, found, err := db.FindId(transistor)
	if err != nil {
		return 1, err
	}
	if !found {
		fmt.Printf("Не найдено: %s\n", transistor.Name())
		return 1, nil
	}

	printTransistor(transistor)
	attributes, err := db.GetAttributes(id)
	if err != nil {
		return 1, err
	}
	manufacturers, err := db.GetManufacturers(id)
	if err != nil {
		return 1, err
	}
	parameters, err := db.GetParameters(id)
	if err != nil {
		return 1, err
	}
	ratings, err := db.GetRatings(id)
	if err != nil {
		return 1, err
	}

	if attributes != nil {
		fmt.Println()
		fmt.Println("Атрибуты:")
		if attributes.Structure != nil {
			fmt.Printf("  Структура: %s\n", *attributes.Structure)
		}
		if attributes.Technology != nil {
			fmt.Printf("  Технология: %s\n", *attributes.Technology)
		}
		if attributes.Package != nil {
			fmt.Printf("  Корпус: %s\n", *attributes.Package)
		}
		if attributes.PackageMaterial != nil {
			fmt.Printf("  Материал корпуса: %s\n", *attributes.PackageMaterial)
		}
		if attributes.ColorMarking != nil {
			fmt.Printf("  Цветовая маркировка: %s\n", *attributes.ColorMarking)
		}
		if attributes.Pinout != nil {
			fmt.Printf("  Цоколёвка: %s\n", *attributes.Pinout)
		}
		if attributes.EsdSensitive != nil {
			fmt.Printf("  Повышенная чувствительность к статическому напряжению: %s\n", yesNo(*attributes.EsdSensitive))
		}
		if attributes.MilitaryGrade != nil {
			fmt.Printf("  Военное исполнение: %s\n", yesNo(*attributes.MilitaryGrade))
		}
		if attributes.RadiationHardened != nil {
			fmt.Printf("  Радиационная стойкость: %s\n", yesNo(*attributes.RadiationHardened))
		}
		if attributes.Tu != nil {
			fmt.Printf("  ТУ: %s\n", *attributes.Tu)
		}
		if attributes.YearFrom != nil {
			if attributes.YearTo != nil {
				fmt.Printf("  Годы выпуска: %d–%d\n", *attributes.YearFrom, *attributes.YearTo)
			} else {
				fmt.Printf("  Выпускается с: %d\n", *attributes.YearFrom)
			}
		}
		if attributes.MassMax != nil {
			fmt.Printf("  Масса: не более %s г\n", domain.Fmt(*attributes.MassMax))
		}
		if attributes.DatasheetUrl != nil {
			fmt.Printf("  Документация: %s\n", *attributes.DatasheetUrl)
		}
		if attributes.Notes != nil {
			fmt.Printf("  Примечание: %s\n", *attributes.Notes)
		}
	}

	if len(manufacturers) > 0 {
		fmt.Println()
		fmt.Println("Производители:")
		fmt.Printf("  %s\n", joinComma(manufacturers))
	}

	if len(parameters) > 0 {
		fmt.Println()
		fmt.Println("Электрические параметры:")
		for _, parameter := range parameters {
			info := domain.ParameterInfoOf(parameter.Kind)
			fmt.Printf("  %s (%s): %s %s\n", info.DisplayName, info.Code,
				domain.FormatParameterValue(parameter), domain.FormatParameterConditions(parameter))
		}
	}

	if ratings != nil {
		fmt.Println()
		fmt.Println("Предельные эксплуатационные данные:")
		if ratings.UkeMax != nil {
			fmt.Printf("  Постоянное напряжение коллектор-эмиттер: %s В\n", domain.Fmt(*ratings.UkeMax))
		}
		if ratings.UkeoMax != nil {
			fmt.Printf("  Постоянное напряжение коллектор-эмиттер при разомкнутой базе: %s В\n", domain.Fmt(*ratings.UkeoMax))
		}
		if ratings.UkbMax != nil {
			fmt.Printf("  Постоянное напряжение коллектор-база: %s В\n", domain.Fmt(*ratings.UkbMax))
		}
		if ratings.UbeMax != nil {
			fmt.Printf("  Постоянное напряжение база-эмиттер: %s В\n", domain.Fmt(*ratings.UbeMax))
		}
		if ratings.IkMax != nil {
			fmt.Printf("  Постоянный ток коллектора: %s мА\n", domain.Fmt(*ratings.IkMax))
		}
		if ratings.IbMax != nil {
			fmt.Printf("  Постоянный ток базы: %s мА\n", domain.Fmt(*ratings.IbMax))
		}
		if ratings.PkMax != nil {
			fmt.Printf("  Постоянная рассеиваемая мощность коллектора: %s мВт\n", domain.Fmt(*ratings.PkMax))
		}
		if ratings.IkPulseMax != nil {
			line := fmt.Sprintf("  Импульсный ток коллектора: %s мА", domain.Fmt(*ratings.IkPulseMax))
			if ratings.PulseDuration != nil {
				line += fmt.Sprintf(" при длительности %s мкс", domain.Fmt(*ratings.PulseDuration))
			}
			fmt.Println(line)
		}
		if ratings.PkPulseMax != nil {
			line := fmt.Sprintf("  Импульсная рассеиваемая мощность коллектора: %s мВт", domain.Fmt(*ratings.PkPulseMax))
			if ratings.PulseDuration != nil {
				line += fmt.Sprintf(" при длительности %s мкс", domain.Fmt(*ratings.PulseDuration))
			}
			fmt.Println(line)
		}
		if ratings.TempMin != nil && ratings.TempMax != nil {
			fmt.Printf("  Температура среды: от %s до %s °C\n", domain.Fmt(*ratings.TempMin), domain.Fmt(*ratings.TempMax))
		}
		if ratings.TempJunctionMax != nil {
			fmt.Printf("  Максимальная температура перехода: %s °C\n", domain.Fmt(*ratings.TempJunctionMax))
		}
		if ratings.Rth != nil {
			fmt.Printf("  Тепловое сопротивление переход-корпус: %s °C/Вт\n", domain.Fmt(*ratings.Rth))
		}
	}
	return 0, nil
}

func cmdFind(names []string, dbPath string, dbSet bool) (int, error) {
	if len(names) != 1 {
		fmt.Fprintln(os.Stderr, "Укажите одно обозначение, например: find КТ315Б")
		return 1, nil
	}
	query, err := domain.ParseDesignation(names[0])
	if err != nil {
		return 1, err
	}
	db, err := openExistingDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	if _, found, err := db.FindId(query); err != nil {
		return 1, err
	} else if found {
		printTransistor(query)
		return 0, nil
	}
	fmt.Printf("Не найдено: %s\n", query.Name())
	equivalents, err := db.FindMaterialEquivalents(query)
	if err != nil {
		return 1, err
	}
	if len(equivalents) > 0 {
		names := make([]string, len(equivalents))
		for i, equivalent := range equivalents {
			names[i] = equivalent.Name()
		}
		fmt.Printf("Есть равнозначная по материалу запись: %s (символы Г/1, К/2, А/3, И/4 обозначают один материал, но записи раздельные)\n",
			joinComma(names))
	}
	return 1, nil
}

func cmdDelete(names []string, dbPath string, dbSet bool, dryRun bool) (int, error) {
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "Укажите обозначение, например: delete КТ315Б")
		return 1, nil
	}

	valid, failures := parseNames(names)

	if dryRun {
		for _, transistor := range valid {
			fmt.Printf("(проверка) будет удалён: %s (с параметрами и предельными данными)\n", transistor.Name())
		}
		return exitByFailures(failures), nil
	}

	removed := 0
	notFound := 0
	db, err := openExistingDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	for _, transistor := range valid {
		deleted, err := db.Delete(transistor)
		if err != nil {
			return 1, err
		}
		if deleted {
			removed++
			fmt.Printf("Удалено: %s\n", transistor.Name())
		} else {
			notFound++
			fmt.Printf("Не найдено: %s\n", transistor.Name())
		}
	}
	fmt.Printf("Итого: удалено %d, не найдено %d, ошибок разбора %d\n", removed, notFound, failures)
	if failures == 0 && notFound == 0 {
		return 0, nil
	}
	return 1, nil
}

func cmdCount(dbPath string, dbSet bool) (int, error) {
	db, err := openExistingDatabase(dbPath, dbSet)
	if err != nil {
		return 1, err
	}
	defer db.Close()
	count, err := db.CountAll()
	if err != nil {
		return 1, err
	}
	fmt.Println(count)
	return 0, nil
}

// parseNames разбирает список обозначений: корректные возвращает, для
// некорректных печатает ошибку и считает их.
func parseNames(names []string) ([]domain.Transistor, int) {
	var valid []domain.Transistor
	failures := 0
	for _, name := range names {
		if transistor, message, ok := domain.TryParseDesignation(name); ok {
			valid = append(valid, transistor)
		} else {
			failures++
			fmt.Fprintf(os.Stderr, "%s: %s\n", name, message)
		}
	}
	return valid, failures
}

// openDatabase — база для команд записи (init/add/import): создаётся при
// необходимости, DDL выполняется один раз за запуск.
func openDatabase(dbPath string, dbSet bool) (storage.TransistorDatabase, error) {
	database := storage.NewSqliteStore(resolveDatabasePath(dbPath, dbSet))
	if err := database.EnsureCreated(); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

// openExistingDatabase — база для команд чтения/delete: существующий файл, без DDL.
func openExistingDatabase(dbPath string, dbSet bool) (storage.TransistorDatabase, error) {
	path := resolveDatabasePath(dbPath, dbSet)
	if _, err := os.Stat(path); err != nil {
		absolute, _ := filepath.Abs(path)
		return nil, domain.NewUserError(
			"база данных не найдена: %s (сначала выполните init или import)", absolute)
	}
	return storage.NewSqliteStore(path), nil
}

func resolveDatabasePath(dbPath string, dbSet bool) string {
	if dbSet {
		return dbPath
	}
	if fromEnv, ok := os.LookupEnv("TRANSISTOR_DB"); ok {
		return fromEnv
	}
	return defaultDatabaseFile
}

func parseOptions(args []string, allowed []string) (map[string]string, error) {
	options := map[string]string{}
	allowedSet := map[string]bool{}
	for _, name := range allowed {
		allowedSet[name] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var name, value string
		if eq := strings.IndexByte(arg, '='); strings.HasPrefix(arg, "--") && eq > 0 {
			name, value = arg[:eq], arg[eq+1:]
		} else if strings.HasPrefix(arg, "--") {
			name = arg
			if i+1 >= len(args) {
				return nil, domain.NewUserError("для опции %s ожидается значение", arg)
			}
			i++
			value = args[i]
		} else {
			return nil, domain.NewUserError("неожидаемый аргумент «%s» (для фильтров используйте --опция=значение)", arg)
		}
		if !allowedSet[name] {
			return nil, domain.NewUserError("неизвестная опция %s", name)
		}
		options[name] = value
	}
	return options, nil
}

func parseMaterialOption(value string) (domain.SemiconductorMaterial, error) {
	runes := []rune(value)
	if len(runes) == 1 {
		if kind, ok := domain.TryMaterialKind(runes[0]); ok {
			return kind, nil
		}
	}
	return 0, domain.NewUserError("--material: ожидалось Г/1, К/2, А/3 или И/4, получено «%s»", value)
}

func parseSubclassOption(value string) (rune, error) {
	runes := []rune(value)
	if len(runes) == 1 && (runes[0] == 'Т' || runes[0] == 'П') {
		return runes[0], nil
	}
	return 0, domain.NewUserError("--subclass: ожидалось Т или П, получено «%s»", value)
}

func parseLettersOption(value string) (string, error) {
	letters := strings.ToUpper(value)
	if domain.IsUpperLetters(letters) {
		return letters, nil
	}
	return "", domain.NewUserError("--letters: ожидалась одна или две заглавные русские буквы, получено «%s»", value)
}

func parseBoolOption(name, value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "да":
		return true, nil
	case "false", "нет":
		return false, nil
	}
	return false, domain.NewUserError("%s: ожидалось true/false, получено «%s»", name, value)
}

func parseIntOption(name, value string, min, max int) (int, error) {
	result, err := strconv.Atoi(value)
	if err != nil || result < min || result > max {
		rangeText := fmt.Sprintf("не меньше %d", min)
		if max != maxInt() {
			rangeText = fmt.Sprintf("от %d до %d", min, max)
		}
		return 0, domain.NewUserError("%s: ожидалось целое %s, получено «%s»", name, rangeText, value)
	}
	return result, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func exitByFailures(failures int) int {
	if failures == 0 {
		return 0
	}
	return 1
}

func exitByErrors(hasErrors bool) int {
	if hasErrors {
		return 1
	}
	return 0
}

func yesNo(value bool) string {
	if value {
		return "да"
	}
	return "нет"
}
