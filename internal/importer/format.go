package importer

import (
	"path/filepath"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Format — формат файла наполнения (реестр форматов —
// docs/plan/04-module-functionality.md §4): jsonc/json и yaml/yml — документы
// целиком, ndjson — построчный поток для больших объёмов.
type Format string

// Реестр форматов наполнения.
const (
	FormatJSONC   Format = "jsonc"
	FormatYAML    Format = "yaml"
	FormatNDJSON  Format = "ndjson"
	formatJSONExt        = ".json"
)

// Known сообщает, входит ли формат в реестр.
func (f Format) Known() bool {
	switch f {
	case FormatJSONC, FormatYAML, FormatNDJSON:
		return true
	}
	return false
}

// ParseFormat разбирает имя формата (опция --format, нижний регистр).
func ParseFormat(s string) (Format, error) {
	f := Format(strings.ToLower(strings.TrimSpace(s)))
	if !f.Known() {
		return "", domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgImportFormatUnknown, s)
	}
	return f, nil
}

// FormatByFilename определяет формат по расширению имени файла. Имя «-»
// (стандартный ввод) читается как jsonc — формат экспорта по умолчанию.
func FormatByFilename(name string) (Format, error) {
	if name == "-" {
		return FormatJSONC, nil
	}
	switch ext := strings.ToLower(filepath.Ext(name)); ext {
	case ".jsonc", formatJSONExt:
		return FormatJSONC, nil
	case ".yaml", ".yml":
		return FormatYAML, nil
	case ".ndjson":
		return FormatNDJSON, nil
	default:
		return "", domain.NewErrorf(domain.CodeInvalidImportFile,
			domain.MsgImportFormatByExt, name, ext)
	}
}

// parseTree разбирает документ формата целиком (jsonc/yaml) в дерево
// значений. Ошибки синтаксиса — *domain.Error invalid_import_file:
// жёсткая ошибка прогона (docs/plan/04-module-functionality.md §4).
func parseTree(data []byte, format Format) (value, error) {
	switch format {
	case FormatJSONC:
		return parseJSONC(data)
	case FormatYAML:
		return parseYAML(data)
	default:
		return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
			domain.MsgImportFormatLineOnly, string(format))
	}
}

// parseLineJSON разбирает одну строку NDJSON (одна JSON-строка — одна
// сущность); дубликаты ключей — ошибка разбора, как и в документных
// форматах; line — номер строки файла для позиций ошибок.
func parseLineJSON(data []byte, line int) (value, error) {
	v, err := decodeJSONValue(data, line)
	if err != nil {
		return value{}, lineError(line, err)
	}
	return v, nil
}

// lineError — ошибка разбора строки NDJSON с номером строки.
func lineError(line int, err error) error {
	return domain.NewErrorf(domain.CodeInvalidImportFile,
		domain.MsgImportNdjsonLineError, line, err.Error())
}

// syntaxError — ошибка синтаксиса формата с указанием формата.
func syntaxError(format Format, err error) error {
	return domain.NewErrorf(domain.CodeInvalidImportFile,
		domain.MsgImportSyntaxError, formatSyntaxName(format), err.Error())
}

func formatSyntaxName(f Format) string {
	switch f {
	case FormatYAML:
		return "YAML"
	default:
		return "JSONC"
	}
}
