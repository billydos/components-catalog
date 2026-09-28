package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"

	"soviettransistors/internal/domain"
)

// parseJsonc разбирает jsonc (JSON с комментариями и висячими запятыми):
// hujson нормализует текст до стандартного JSON (комментарии и лишние запятые
// заменяются пробелами по месту — смещения и номера строк сохраняются), затем
// потоковый декодер stdlib строит дерево value.
func parseJsonc(data []byte) (value, error) {
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return value{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(standardized))
	decoder.UseNumber()
	return decodeValue(decoder, standardized)
}

// ParseJsoncText разбирает jsonc-текст. Синтаксически некорректный текст —
// *domain.UserError; проблемы структуры корня — Issues уровня файла.
func ParseJsoncText(text string) (*ParseResult, error) {
	root, err := parseJsonc([]byte(text))
	if err != nil {
		return nil, domain.NewUserError("файл не является корректным JSONC: %s", err.Error())
	}
	return parseRoot(root), nil
}

// decodeValue строит дерево value из потока json-токенов (UseNumber — числа
// как текст, без потери точности). Повторяющиеся ключи объекта — ошибка
// разбора, как в yaml: «выигрывающий» первый ключ не должен молча отбрасывать
// второй (тихая потеря данных при случайном дублировании).
func decodeValue(decoder *json.Decoder, source []byte) (value, error) {
	token, err := decoder.Token()
	if err != nil {
		return value{}, err
	}
	switch t := token.(type) {
	case nil:
		return value{kind: kindNull}, nil
	case bool:
		return value{kind: kindBool, boolean: t}, nil
	case string:
		return value{kind: kindString, str: t}, nil
	case json.Number:
		return value{kind: kindNumber, num: t.String()}, nil
	case json.Delim:
		switch t {
		case '{':
			object := value{kind: kindObject}
			names := make(map[string]bool)
			for decoder.More() {
				nameToken, err := decoder.Token()
				if err != nil {
					return value{}, err
				}
				name, ok := nameToken.(string)
				if !ok {
					return value{}, errors.New("неверный ключ объекта")
				}
				if names[name] {
					return value{}, fmt.Errorf("повторяющийся ключ «%s» (строка %d)", name, lineOfOffset(source, decoder.InputOffset()))
				}
				names[name] = true
				item, err := decodeValue(decoder, source)
				if err != nil {
					return value{}, err
				}
				object.members = append(object.members, member{name: name, value: item})
			}
			if _, err := decoder.Token(); err != nil { // закрывающая }
				return value{}, err
			}
			return object, nil
		case '[':
			array := value{kind: kindArray}
			for decoder.More() {
				item, err := decodeValue(decoder, source)
				if err != nil {
					return value{}, err
				}
				array.items = append(array.items, item)
			}
			if _, err := decoder.Token(); err != nil { // закрывающая ]
				return value{}, err
			}
			return array, nil
		}
	}
	return value{}, fmt.Errorf("неожидаемый токен %v", token)
}

// lineOfOffset — номер строки (с 1) байтового смещения в стандартизованном
// тексте. Номер совпадает с исходным файлом: hujson.Standardize заменяет
// комментарии и висячие запятые пробелами по месту, сохраняя смещения
// и переводы строк.
func lineOfOffset(source []byte, offset int64) int {
	return 1 + bytes.Count(source[:offset], []byte("\n"))
}
