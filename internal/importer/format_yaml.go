package importer

import (
	"errors"
	"fmt"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// parseYaml разбирает yaml в дерево value через AST goccy/go-yaml: узлы
// сохраняют порядок ключей документа. Комментарии — родной синтаксис yaml;
// повторяющиеся ключи библиотека отвергает при разборе — так же, как jsonc,
// поэтому имена ключей в дереве value уникальны. Якоря и алиасы
// раскрываются в значения (plan/05-work-plan.md задача 4.1).
func parseYAML(data []byte) (value, error) {
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return value{}, syntaxError(FormatYAML, err)
	}
	switch len(file.Docs) {
	case 0:
		return value{kind: kindNull}, nil // пустой файл — null-документ
	case 1:
		if file.Docs[0].Body == nil {
			return value{kind: kindNull}, nil
		}
		v, err := yamlToValue(file.Docs[0].Body)
		if err != nil {
			return value{}, syntaxError(FormatYAML, err)
		}
		return v, nil
	}
	return value{}, syntaxError(FormatYAML, errors.New("файл содержит несколько yaml-документов, ожидается один"))
}

// yamlConverter конвертирует узлы yaml-AST в value; якоря и алиасы
// раскрываются в значения: парсер goccy алиасы не разрешает (AliasNode
// хранит имя, а не узел якоря), поэтому конвертер ведёт реестр якорей
// документа сам. Циклическая ссылка и алиас без предшествующего якоря —
// ошибка разбора. Теги и литеральные блоки разворачиваются в обёрнутое
// значение.
type yamlConverter struct {
	anchors    map[string]value
	converting map[string]bool
}

func yamlToValue(node ast.Node) (value, error) {
	converter := &yamlConverter{anchors: make(map[string]value), converting: make(map[string]bool)}
	return converter.convert(node)
}

func (c *yamlConverter) convert(node ast.Node) (value, error) {
	switch n := node.(type) {
	case *ast.NullNode:
		return value{kind: kindNull}, nil
	case *ast.BoolNode:
		return value{kind: kindBool, boolean: n.Value}, nil
	case *ast.IntegerNode:
		return value{kind: kindNumber, num: n.GetToken().Value}, nil
	case *ast.FloatNode:
		return value{kind: kindNumber, num: n.GetToken().Value}, nil
	case *ast.InfinityNode:
		return value{kind: kindNumber, num: n.GetToken().Value}, nil
	case *ast.NanNode:
		return value{kind: kindNumber, num: n.GetToken().Value}, nil
	case *ast.StringNode:
		return value{kind: kindString, str: n.Value}, nil
	case *ast.LiteralNode:
		return value{kind: kindString, str: n.Value.Value}, nil
	case *ast.AnchorNode:
		name := n.Name.GetToken().Value
		if c.converting[name] {
			return value{}, fmt.Errorf(
				"циклическая ссылка: алиас «*%s» участвует в значении якоря «&%s»", name, name)
		}
		c.converting[name] = true
		converted, err := c.convert(n.Value)
		delete(c.converting, name)
		if err != nil {
			return value{}, err
		}
		c.anchors[name] = converted
		return converted, nil
	case *ast.AliasNode:
		name := n.Value.GetToken().Value
		if converted, ok := c.anchors[name]; ok {
			return converted, nil
		}
		if c.converting[name] {
			return value{}, fmt.Errorf(
				"циклическая ссылка: алиас «*%s» участвует в значении якоря «&%s»", name, name)
		}
		return value{}, fmt.Errorf(
			"неизвестный алиас «*%s» — якорь «&%s» не определён до использования", name, name)
	case *ast.TagNode:
		return c.convert(n.Value)
	case *ast.MappingNode:
		object := value{kind: kindObject}
		for _, item := range n.Values {
			name, err := yamlKeyName(item.Key)
			if err != nil {
				return value{}, err
			}
			converted, err := c.convert(item.Value)
			if err != nil {
				return value{}, err
			}
			object.members = append(object.members, member{name: name, value: converted})
		}
		return object, nil
	case *ast.SequenceNode:
		array := value{kind: kindArray}
		for _, item := range n.Values {
			converted, err := c.convert(item)
			if err != nil {
				return value{}, err
			}
			array.items = append(array.items, converted)
		}
		return array, nil
	}
	return value{}, fmt.Errorf("неподдерживаемый элемент yaml: %T", node)
}

// yamlKeyName — текст ключа отображения: обычные ключи и ключи явной формы
// «? ключ»; прочие (числовые, merge-ключ «<<») — ошибка, как в jsonc.
func yamlKeyName(key ast.MapKeyNode) (string, error) {
	switch k := key.(type) {
	case *ast.StringNode:
		return k.Value, nil
	case *ast.MappingKeyNode:
		if s, ok := k.Value.(*ast.StringNode); ok {
			return s.Value, nil
		}
	}
	return "", errors.New("неверный ключ объекта")
}
