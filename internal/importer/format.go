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
		return "", domain.NewError(domain.CodeInvalidImportFile,
			"неизвестный формат «"+s+"» (допустимы: jsonc, yaml, ndjson)")
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
		return "", domain.NewError(domain.CodeInvalidImportFile, strings.Join([]string{
			"файл «" + name + "»: не удалось определить формат по расширению «" + ext + "»",
			"(допустимы: .jsonc/.json, .yaml/.yml, .ndjson — либо задайте --format)",
		}, " "))
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
		return value{}, domain.NewError(domain.CodeInvalidImportFile,
			"формат "+string(format)+" читается построчно, а не деревом целиком")
	}
}

// parseLineJSON разбирает одну строку NDJSON (одна JSON-строка — одна
// сущность); дубликаты ключей — ошибка разбора, как и в документных
// форматах.
func parseLineJSON(data []byte, line int) (value, error) {
	v, err := decodeJSONValue(data)
	if err != nil {
		return value{}, lineError(line, err)
	}
	return v, nil
}

// lineError — ошибка разбора строки NDJSON с номером строки.
func lineError(line int, err error) error {
	return domain.NewError(domain.CodeInvalidImportFile,
		"строка "+itoa(line)+": "+err.Error())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// syntaxError — ошибка синтаксиса формата с указанием формата.
func syntaxError(format Format, err error) error {
	return domain.NewError(domain.CodeInvalidImportFile,
		"файл не является корректным "+formatSyntaxName(format)+": "+err.Error())
}

func formatSyntaxName(f Format) string {
	switch f {
	case FormatYAML:
		return "YAML"
	default:
		return "JSONC"
	}
}
