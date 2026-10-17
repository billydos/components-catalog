package components

import (
	"github.com/billydos/components-catalog/internal/importer"
)

// Importer — импорт/экспорт файлов наполнения поверх сервисного слоя
// (docs/plan/04-module-functionality.md §1.1 ImportService). Семантика
// секций и тексты ошибок формата — план 04 §4.

type Importer = importer.Importer

// ImportReport — итог прогона импорта: счётчики исходов записей
// (Added/Updated/Skipped/Rejected) и накопленные проблемы.

type ImportReport = importer.Report

// ImportIssue — проблема прогона импорта: код и сообщение каталога
// (Message — канонический en-рендер), позиция записи в файле.

type ImportIssue = importer.Issue

// Format — формат файла наполнения (реестр форматов — план 04 §4).

type Format = importer.Format

const (
	FormatJSONC  = importer.FormatJSONC
	FormatYAML   = importer.FormatYAML
	FormatNDJSON = importer.FormatNDJSON
)

// Прогресс прогона импорта: большие файлы читаются потоком
// (NDJSON — построчно), колбэк WithProgress наблюдает продвижение.

type (
	// ImportOption — опция прогона Importer.Import.
	ImportOption = importer.ImportOption

	// Progress — снимок прогресса прогона: нарастающие счётчики чтения
	// файла и применения записей.
	Progress = importer.Progress
)

// WithProgress — колбэк прогресса прогона импорта: вызывается после
// каждой прочитанной записи потока NDJSON (форматы-документы отмечают
// чтение одним вызовом) и по итоговому исходу применения каждой записи.
func WithProgress(fn func(Progress)) ImportOption { return importer.WithProgress(fn) }
