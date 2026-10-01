package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// parseJSONC разбирает jsonc (JSON с комментариями и висячими запятыми):
// hujson.Standardize заменяет комментарии и лишние запятые пробелами по
// месту (смещения и номера строк сохраняются), затем потоковый декодер
// stdlib строит дерево value. Повторяющиеся ключи объекта — ошибка
// разбора файла с номером строки: «выигрывающий» ключ не должен молча
// отбрасывать другой (тихая потеря данных недопустима).
func parseJSONC(data []byte) (value, error) {
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return value{}, syntaxError(FormatJSONC, err)
	}
	v, err := decodeJSONDocument(standardized)
	if err != nil {
		return value{}, syntaxError(FormatJSONC, err)
	}
	return v, nil
}

// decodeJSONDocument — документ целиком (jsonc): после значения данных
// быть не должно.
func decodeJSONDocument(data []byte) (value, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	v, err := decodeJSONFrom(decoder, data)
	if err != nil {
		return value{}, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err == nil {
		return value{}, errors.New("после корневого значения идут лишние данные")
	}
	return v, nil
}

// decodeJSONValue — одно значение из буфера (строка NDJSON).
func decodeJSONValue(data []byte) (value, error) {
	return decodeJSONDocument(data)
}

// decodeJSONFrom строит дерево value из потока json-токенов (UseNumber —
// числа как текст, без потери точности). Повторяющиеся ключи объекта —
// ошибка разбора с номером строки.
func decodeJSONFrom(decoder *json.Decoder, source []byte) (value, error) {
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
					return value{}, fmt.Errorf("повторяющийся ключ «%s» (строка %d)",
						name, lineOfOffset(source, decoder.InputOffset()))
				}
				names[name] = true
				item, err := decodeJSONFrom(decoder, source)
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
				item, err := decodeJSONFrom(decoder, source)
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

// lineOfOffset — номер строки (с 1) байтового смещения в
// стандартизованном тексте: заменённые комментарии и запятые сохраняют
// смещения исходного файла.
func lineOfOffset(source []byte, offset int64) int {
	end := int(offset)
	if end > len(source) {
		end = len(source)
	}
	return 1 + bytes.Count(source[:end], []byte("\n"))
}

// jsonScalar — сериализация скаляра дерева в JSON-текст (строки — через
// json.Marshal, экранирование единообразно; допустимо и в YAML в двойных
// кавычках). Используется эмиттерами экспорта.
func jsonScalar(v value) string {
	switch v.kind {
	case kindNull:
		return "null"
	case kindBool:
		if v.boolean {
			return "true"
		}
		return "false"
	case kindNumber:
		return v.num
	case kindString:
		b, err := json.Marshal(v.str)
		if err != nil {
			return `""`
		}
		return string(b)
	}
	return "null"
}
