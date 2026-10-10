package importer

import (
	"fmt"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// parseYaml разбирает yaml в дерево value через AST goccy/go-yaml: узлы
// сохраняют порядок ключей документа. Комментарии — родной синтаксис yaml;
// повторяющиеся ключи библиотека отвергает при разборе — так же, как jsonc,
// поэтому имена ключей в дереве value уникальны. Якоря и алиасы
// раскрываются в значения (docs/plan/05-work-plan.md задача 4.1).
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
			if _, ok := domain.AsError(err); ok {
				return value{}, err // ошибка формата с собственным MsgID
			}
			return value{}, syntaxError(FormatYAML, err)
		}
		return v, nil
	}
	return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
		domain.MsgImportYamlMultiDoc, yamlDocLine(file.Docs[1]))
}

// yamlNonFiniteText — текст скаляра YAML, обозначающий не-конечное число
// (формы ядра YAML с необязательным знаком: .inf/.Inf/.INF, .nan/.NaN/.NAN).
func yamlNonFiniteText(s string) bool {
	body := s
	if len(body) > 0 && (body[0] == '-' || body[0] == '+') {
		body = body[1:]
	}
	switch body {
	case ".inf", ".Inf", ".INF", ".nan", ".NaN", ".NAN":
		return true
	}
	return false
}

// yamlConverter конвертирует узлы yaml-AST в value; якоря и алиасы
// раскрываются в значения: парсер goccy алиасы не разрешает (AliasNode
// хранит имя, а не узел якоря), поэтому конвертер ведёт реестр якорей
// документа сам. Циклическая ссылка и алиас без предшествующего якоря —
// ошибка разбора. Теги и литеральные блоки разворачиваются в обёрнутое
// значение. Не-конечные числа (inf/nan во всех формах, которые даёт
// goccy/go-yaml) отвергаются: канонические числа каталога конечны.
type yamlConverter struct {
	anchors    map[string]value
	converting map[string]bool
}

func yamlToValue(node ast.Node) (value, error) {
	converter := &yamlConverter{anchors: make(map[string]value), converting: make(map[string]bool)}
	return converter.convert(node, "")
}

// nonFinite — ошибка формата о не-конечном числе: ключ отображения и
// позиция токена (как у соседних ошибок формата).
func (c *yamlConverter) nonFinite(key string, tok *token.Token) error {
	return domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgImportYamlNonFinite,
		key, tok.Value, tok.Position.Line)
}

// convert конвертирует узел; key — ключ отображения, под которым стоит
// узел (для сообщений об ошибках, «» — корень документа).
func (c *yamlConverter) convert(node ast.Node, key string) (value, error) {
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
		return value{}, c.nonFinite(key, n.GetToken())
	case *ast.NanNode:
		return value{}, c.nonFinite(key, n.GetToken())
	case *ast.StringNode:
		// Явно закавыченные скаляры — строки по намерению автора (экспорт
		// закавычивает все похожие на inf/nan); отвергаются только plain-
		// скаляры не-конечных чисел.
		if n.GetToken().Type == token.StringType && yamlNonFiniteText(n.Value) {
			return value{}, c.nonFinite(key, n.GetToken())
		}
		return value{kind: kindString, str: n.Value}, nil
	case *ast.LiteralNode:
		return value{kind: kindString, str: n.Value.Value}, nil
	case *ast.AnchorNode:
		name := n.Name.GetToken().Value
		if c.converting[name] {
			return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
				domain.MsgImportYamlAliasCycle, name, yamlNodeLine(n))
		}
		c.converting[name] = true
		converted, err := c.convert(n.Value, key)
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
			return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
				domain.MsgImportYamlAliasCycle, name, yamlNodeLine(n))
		}
		return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
			domain.MsgImportYamlAliasUnknown, name, yamlNodeLine(n))
	case *ast.TagNode:
		return c.convert(n.Value, key)
	case *ast.MappingNode:
		object := value{kind: kindObject}
		for _, item := range n.Values {
			name, err := yamlKeyName(item.Key)
			if err != nil {
				return value{}, err
			}
			converted, err := c.convert(item.Value, name)
			if err != nil {
				return value{}, err
			}
			object.members = append(object.members, member{name: name, value: converted})
		}
		return object, nil
	case *ast.SequenceNode:
		array := value{kind: kindArray}
		for _, item := range n.Values {
			converted, err := c.convert(item, key)
			if err != nil {
				return value{}, err
			}
			array.items = append(array.items, converted)
		}
		return array, nil
	}
	return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
		domain.MsgImportYamlNodeUnsupported, fmt.Sprintf("%T", node), yamlNodeLine(node))
}

// yamlNodeLine — строка узла yaml-AST (1 — первая строка файла); у узла
// без токена — 0.
func yamlNodeLine(n ast.Node) int {
	if tok := n.GetToken(); tok != nil {
		return tok.Position.Line
	}
	return 0
}

// yamlDocLine — строка начала документа (первый токен тела).
func yamlDocLine(doc *ast.DocumentNode) int {
	if doc == nil || doc.Body == nil {
		return 0
	}
	return yamlNodeLine(doc.Body)
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
	text := ""
	if tok := key.GetToken(); tok != nil {
		text = tok.Value
	}
	return "", domain.NewErrorf(domain.CodeInvalidImportFile,
		domain.MsgImportYamlKeyInvalid, text, yamlNodeLine(key))
}
