package importer

// Прогресс прогона импорта (features.log CC-6): большие файлы читаются
// потоком (NDJSON — построчно, без загрузки файла целиком), встраивающий
// сайт наблюдает продвижение колбэком опции WithProgress.

// Progress — снимок прогресса прогона импорта: нарастающие счётчики
// чтения файла и применения записей. Колбэк WithProgress вызывается по
// ходу прогона: NDJSON — после каждой прочитанной записи (строки catalog
// записями не считаются), форматы-документы (jsonc/yaml) читаются целиком
// — чтение отмечается одним вызовом после разбора; применение записей
// (все форматы, включая dry-run) — по итоговому исходу каждой записи.
// Отвергнутые чтением записи входят в Read и не входят в Applied —
// применяются только прошедшие чтение.
type Progress struct {
	Read    int // записей прочитано из файла (включая отвергнутых чтением)
	Applied int // записей доведено до исхода применения (Added/Updated/Skipped/Rejected)
	Line    int // номер последней прочитанной строки NDJSON (0 — не определён)
}

// ImportOption — опция прогона импорта (Import/ImportCatalogFile).
type ImportOption func(*importOptions)

// WithProgress — колбэк прогресса прогона: снимки нарастающих счётчиков
// по ходу чтения и применения (см. Progress). Вызывается синхронно
// в горутине прогона: быстрый возврат, долгая обработка останавливает
// импорт (отмена — контекст прогона).
func WithProgress(fn func(Progress)) ImportOption {
	return func(o *importOptions) { o.onProgress = fn }
}

// importOptions — опции прогона и накопленное по ходу состояние
// прогресса (счётчики — достояние прогона, а не конфигурации).
type importOptions struct {
	onProgress   func(Progress)
	recordsRead  int
	appliedTotal int
	lastLine     int
}

// newImportOptions применяет опции к свежему состоянию прогона.
func newImportOptions(opts []ImportOption) *importOptions {
	o := &importOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

// recordRead — прочитана запись NDJSON (счётчик — нарастающий итог
// прогона records).
func (o *importOptions) recordRead(line, records int) {
	o.lastLine, o.recordsRead = line, records
	o.notify()
}

// recordsLoaded — записи документа разобраны целиком (jsonc/yaml).
func (o *importOptions) recordsLoaded(records int) {
	o.recordsRead = records
	o.notify()
}

// appliedOutcome — итоговый исход записи применения.
func (o *importOptions) appliedOutcome() {
	o.appliedTotal++
	o.notify()
}

// notify — выстрел колбэка прогресса (без колбэка — no-op).
func (o *importOptions) notify() {
	if o.onProgress != nil {
		o.onProgress(Progress{Read: o.recordsRead, Applied: o.appliedTotal, Line: o.lastLine})
	}
}
