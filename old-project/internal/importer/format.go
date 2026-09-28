package importer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"soviettransistors/internal/domain"
)

// fileFormat — формат импорта: расширения имён файлов и разбор текста.
type fileFormat struct {
	extensions []string
	parseText  func(text string) (*ParseResult, error)
}

// fileFormats — реестр форматов импорта; новый формат — файл format_<имя>.go
// (разбор текста в дерево value) и запись здесь.
var fileFormats = []fileFormat{
	{extensions: []string{".jsonc", ".json"}, parseText: ParseJsoncText},
	{extensions: []string{".yaml", ".yml"}, parseText: ParseYamlText},
}

// ParseFile читает и разбирает файл справочника; формат выбирается по
// расширению. Ошибки файловой системы возвращаются как есть; неизвестное
// расширение и синтаксические ошибки формата — *domain.UserError; проблемы
// структуры корня — Issues уровня файла в ParseResult.
func ParseFile(path string) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	extension := strings.ToLower(filepath.Ext(path))
	for _, format := range fileFormats {
		if slices.Contains(format.extensions, extension) {
			return format.parseText(string(data))
		}
	}
	return nil, domain.NewUserError(
		"неподдерживаемое расширение файла «%s» (допустимые: %s)", extension, supportedExtensions())
}

// supportedExtensions — список допустимых расширений для сообщений об ошибках.
func supportedExtensions() string {
	var extensions []string
	for _, format := range fileFormats {
		extensions = append(extensions, format.extensions...)
	}
	return strings.Join(extensions, ", ")
}
