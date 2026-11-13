package api

// Защита от дрейфа локализации контракта REST (D9: каноническая локаль —
// en). api/openapi.yaml — канонический en-документ; api/overlays/ru.yaml —
// русская локаль форматом OpenAPI Overlay 1.1 (таргет — JSONPath к узлу
// канонического документа); api/dist/openapi.ru.yaml — генерируемый
// Redocly CLI артефакт (команда регенерации — api/redocly.yaml, единственное
// место с ней). Тесты закрепляют инварианты: проза канонического файла —
// только en; оверлей адресует существующие узлы, покрывает всю прозу
// и несёт русский текст (Redocly молча игнорирует несматченный таргет —
// дрейф ловится здесь); сгенерированный артефакт глубоко равен документу
// «канонический + оверлей».

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/goccy/go-yaml"
)

// proseKeys — ключи OpenAPI-узлов с локализуемым текстом.
var proseKeys = map[string]bool{"summary": true, "description": true}

// cyrillicAllowlist — узлы канонической прозы, где кириллица легальна:
// примеры-данные (обозначения), не текст документации. Сироты (записи
// без узла) запрещаются тестом.
var cyrillicAllowlist = map[string]bool{
	"$['components']['schemas']['CardSynonym']['description']": true, // пример С5-35 = ПЭВ
}

// localeNeutralUpdates — узлы, где update оверлея легально без кириллицы:
// текст-код, одинаковый в обеих локалях. Прочие update обязаны содержать
// русский текст — иначе непереведённая подстановка пройдёт молча.
var localeNeutralUpdates = map[string]bool{
	"$['components']['schemas']['CatalogSnapshot']['properties']['revision']['description']": true, // catalog_revision.
}

// maxReportedDiffs — потолок расхождений в диагностике глубинного
// сравнения dist.
const maxReportedDiffs = 10

type segKind int

const (
	segChild segKind = iota
	segIndex
	segFilter
)

// pathSegment — сегмент поддерживаемого подмножества JSONPath оверлея:
// дочерний ключ, индекс массива либо фильтр элементов массива по ключу.
type pathSegment struct {
	kind  segKind
	key   string
	value string
	index int
}

var (
	reIndex  = regexp.MustCompile(`^\[(\d+)\]`)
	reChild  = regexp.MustCompile(`^\['([^']+)'`)
	reFilter = regexp.MustCompile(`^\[\?@\['([^']+)'\]=='([^']+)'\]`)
)

// parseTarget разбирает поддерживаемое подмножество JSONPath-таргетов
// оверлея: $.a.b, $['a/b'], $[0], $[?@['name']=='x'].
func parseTarget(target string) ([]pathSegment, error) {
	rest := strings.TrimPrefix(target, "$")
	if len(rest) == len(target) {
		return nil, fmt.Errorf("таргет без корня $: %s", target)
	}
	var segs []pathSegment
	for len(rest) > 0 {
		switch {
		case strings.HasPrefix(rest, "."):
			name := rest[1:]
			if i := strings.IndexAny(name, ".["); i >= 0 {
				name = name[:i]
			}
			if name == "" {
				return nil, fmt.Errorf("пустой сегмент в %s", target)
			}
			segs = append(segs, pathSegment{kind: segChild, key: name})
			rest = rest[1+len(name):]
		case strings.HasPrefix(rest, "['"):
			m := reChild.FindStringSubmatch(rest)
			if m == nil {
				return nil, fmt.Errorf("битый сегмент в %s", target)
			}
			segs = append(segs, pathSegment{kind: segChild, key: m[1]})
			rest = rest[3+len(m[1])+1:]
		case strings.HasPrefix(rest, "[?"):
			m := reFilter.FindStringSubmatch(rest)
			if m == nil {
				return nil, fmt.Errorf("неподдерживаемый фильтр в %s", target)
			}
			segs = append(segs, pathSegment{kind: segFilter, key: m[1], value: m[2]})
			rest = rest[len(m[0]):]
		case strings.HasPrefix(rest, "["):
			m := reIndex.FindStringSubmatch(rest)
			if m == nil {
				return nil, fmt.Errorf("битый индекс в %s", target)
			}
			n := 0
			for _, d := range m[1] {
				n = n*10 + int(d-'0')
			}
			segs = append(segs, pathSegment{kind: segIndex, index: n})
			rest = rest[len(m[0]):]
		default:
			return nil, fmt.Errorf("неподдерживаемый сегмент в %s", target)
		}
	}
	return segs, nil
}

// nodePath — выбранный узел и его путь в индексной форме (совпадает
// с формой путей walkProse: фильтр раскрывается в индекс элемента).
type nodePath struct {
	node any
	path string
}

// resolve возвращает узлы документа, выбранные сегментами таргета,
// вместе с их индексными путями.
func resolve(root any, segs []pathSegment) []nodePath {
	current := []nodePath{{node: root, path: "$"}}
	for _, seg := range segs {
		var next []nodePath
		for _, np := range current {
			switch seg.kind {
			case segChild:
				m, ok := np.node.(map[string]any)
				if !ok {
					continue
				}
				if v, exists := m[seg.key]; exists {
					next = append(next, nodePath{node: v, path: np.path + "['" + seg.key + "']"})
				}
			case segIndex:
				s, ok := np.node.([]any)
				if !ok || seg.index >= len(s) {
					continue
				}
				next = append(next, nodePath{node: s[seg.index], path: fmt.Sprintf("%s[%d]", np.path, seg.index)})
			case segFilter:
				s, ok := np.node.([]any)
				if !ok {
					continue
				}
				for i, item := range s {
					m, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if v, exists := m[seg.key]; exists && v == seg.value {
						next = append(next, nodePath{node: item, path: fmt.Sprintf("%s[%d]", np.path, i)})
					}
				}
			}
		}
		current = next
	}
	return current
}

// walkProse обходит документ и возвращает пути (индексная форма) и
// значения узлов прозы — строк по ключам summary/description.
func walkProse(doc any) map[string]string {
	out := map[string]string{}
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch t := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				child := path + "['" + k + "']"
				if proseKeys[k] {
					if s, ok := t[k].(string); ok {
						out[child] = s
					}
					continue
				}
				walk(t[k], child)
			}
		case []any:
			for i, item := range t {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(doc, "$")
	return out
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func readYAML(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение %s: %v", path, err)
	}
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("разбор %s: %v", path, err)
	}
	return doc
}

type overlayAction struct {
	Target string `yaml:"target"`
	Update any    `yaml:"update"`
}

type overlayDoc struct {
	Overlay string          `yaml:"overlay"`
	Actions []overlayAction `yaml:"actions"`
}

func loadOverlay(t *testing.T) overlayDoc {
	t.Helper()
	data, err := os.ReadFile("overlays/ru.yaml")
	if err != nil {
		t.Fatalf("чтение оверлея: %v", err)
	}
	var doc overlayDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("разбор оверлея: %v", err)
	}
	if !strings.HasPrefix(doc.Overlay, "1.") {
		t.Fatalf("версия оверлея: %q, ожидалась 1.x", doc.Overlay)
	}
	if len(doc.Actions) == 0 {
		t.Fatal("оверлей без действий")
	}
	return doc
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = deepCopy(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = deepCopy(val)
		}
		return out
	default:
		return v
	}
}

// applyOverlay применяет оверлей к глубокой копии канонического документа
// и возвращает её — эталон для сравнения со сгенерированным артефактом.
// Последний сегмент таргета обязан быть ключом прозы (инвариант
// TestOverlayCoversCanonicalProse).
func applyOverlay(t *testing.T, canonical any, overlay overlayDoc) any {
	t.Helper()
	doc := deepCopy(canonical)
	for _, act := range overlay.Actions {
		segs, err := parseTarget(act.Target)
		if err != nil {
			t.Fatalf("действие %q: %v", act.Target, err)
		}
		last := segs[len(segs)-1]
		if last.kind != segChild {
			t.Fatalf("таргет %q: последний сегмент не ключ — применитель не поддерживает", act.Target)
		}
		applied := false
		for _, p := range resolve(doc, segs[:len(segs)-1]) {
			m, ok := p.node.(map[string]any)
			if !ok {
				continue
			}
			if _, exists := m[last.key]; exists {
				m[last.key] = deepCopy(act.Update)
				applied = true
			}
		}
		if !applied {
			t.Fatalf("таргет %q не применён в памяти — дрейф структуры", act.Target)
		}
	}
	return doc
}

// treeDiffs собирает до maxReportedDiffs расхождений двух деревьев
// (пути в индексной форме) — диагностика глубинного сравнения dist.
func treeDiffs(want, got any, path string, out *[]string) {
	if len(*out) >= maxReportedDiffs {
		return
	}
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: ожидается отображение, в dist %T", path, got))
			return
		}
		for _, k := range sortedKeys(w) {
			treeDiffs(w[k], g[k], path+"['"+k+"']", out)
		}
		for _, k := range sortedKeys(g) {
			if _, exists := w[k]; !exists {
				*out = append(*out, fmt.Sprintf("%s['%s']: лишний ключ в dist", path, k))
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: ожидается список, в dist %T", path, got))
			return
		}
		if len(w) != len(g) {
			*out = append(*out, fmt.Sprintf("%s: %d элементов, в dist %d", path, len(w), len(g)))
			return
		}
		for i := range w {
			treeDiffs(w[i], g[i], fmt.Sprintf("%s[%d]", path, i), out)
		}
	default:
		if !reflect.DeepEqual(want, got) {
			*out = append(*out, fmt.Sprintf("%s: %v, в dist %v", path, want, got))
		}
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestCanonicalProseIsEnglish — каноническая спецификация документируется
// на en (D9); кириллица в прозе — признак непереведённого или случайно
// попавшего текста (кроме данных-примеров по allowlist). Сироты allowlist
// запрещены: запись без узла — вечное разрешение кириллицы в позиции.
func TestCanonicalProseIsEnglish(t *testing.T) {
	prose := walkProse(readYAML(t, "openapi.yaml"))
	if len(prose) < 100 {
		t.Fatalf("прозы в каноническом файле %d узлов — обход сломан?", len(prose))
	}
	paths := make([]string, 0)
	for path, text := range prose {
		if hasCyrillic(text) && !cyrillicAllowlist[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		t.Errorf("кириллица в en-прозе: %s", path)
	}
	orphans := make([]string, 0)
	for path := range cyrillicAllowlist {
		if _, ok := prose[path]; !ok {
			orphans = append(orphans, path)
		}
	}
	sort.Strings(orphans)
	for _, path := range orphans {
		t.Errorf("allowlist кириллицы без узла в каноническом файле: %s", path)
	}
}

// TestOverlayCoversCanonicalProse — оверлей адресует существующие узлы
// (Redocly молча пропускает несматченный таргет), каждый таргет ведёт
// к узлу прозы, покрывает её целиком и несёт русский текст: ни забытых
// переводов, ни английских подстановок, ни посторонних изменений
// структуры.
func TestOverlayCoversCanonicalProse(t *testing.T) {
	canonical := readYAML(t, "openapi.yaml")
	prose := walkProse(canonical)
	overlay := loadOverlay(t)

	covered := map[string]bool{}
	coveredCyrillic := map[string]bool{}
	for _, act := range overlay.Actions {
		segs, err := parseTarget(act.Target)
		if err != nil {
			t.Errorf("действие %q: %v", act.Target, err)
			continue
		}
		if last := segs[len(segs)-1]; last.kind != segChild || !proseKeys[last.key] {
			t.Errorf("таргет %q ведёт не к узлу прозы", act.Target)
			continue
		}
		nodes := resolve(canonical, segs)
		if len(nodes) == 0 {
			t.Errorf("таргет %q не разрешается узлом — дрейф структуры", act.Target)
			continue
		}
		if len(nodes) > 1 {
			t.Errorf("таргет %q разрешается %d узлами, ожидался один", act.Target, len(nodes))
			continue
		}
		if _, ok := nodes[0].node.(string); !ok {
			t.Errorf("таргет %q выбрал нестроковый узел", act.Target)
			continue
		}
		s, ok := act.Update.(string)
		if !ok || s == "" {
			t.Errorf("таргет %q: update обязан быть непустой строкой", act.Target)
			continue
		}
		path := nodes[0].path
		if !hasCyrillic(s) && !localeNeutralUpdates[path] {
			t.Errorf("таргет %q: update без русского текста — непереведённая подстановка", act.Target)
		}
		if covered[path] {
			t.Errorf("повторный таргет %q", act.Target)
		}
		covered[path] = true
		coveredCyrillic[path] = hasCyrillic(s)
	}
	missing := make([]string, 0)
	for path := range prose {
		if !covered[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	for _, path := range missing {
		t.Errorf("проза без перевода в оверлее: %s", path)
	}
	for _, path := range sortedStringKeys(localeNeutralUpdates) {
		if !covered[path] {
			t.Errorf("локале-нейтральный allowlist без узла: %s", path)
		} else if coveredCyrillic[path] {
			t.Errorf("лишняя запись локале-нейтрального allowlist (update с кириллицей): %s", path)
		}
	}
}

func sortedStringKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestDistBundleFresh — сгенерированный артефакт глубоко равен документу
// «канонический + оверлей»: любое расхождение (проза, коды, enum,
// структура) означает устаревший артефакт или поведение генератора,
// отличное от ожидаемого.
func TestDistBundleFresh(t *testing.T) {
	canonical := readYAML(t, "openapi.yaml")
	dist := readYAML(t, "dist/openapi.ru.yaml")
	expected := applyOverlay(t, canonical, loadOverlay(t))

	var diffs []string
	treeDiffs(expected, dist, "$", &diffs)
	if len(diffs) > 0 {
		t.Errorf("dist разошёлся с canonical+оверлеем (%d расхождений; регенерируйте по инструкции в api/redocly.yaml): %s",
			len(diffs), strings.Join(diffs, "; "))
	}
}
