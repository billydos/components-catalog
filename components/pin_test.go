// Пин-тест полноты фасада: внутренний сервисный слой — публичный API
// библиотеки (план 01 §2.4), его экспортированная поверхность открывается
// фасадом целиком, а сам фасад остаётся тонким — собственные объявления
// только App/Config/Open/AsError, всё прочее — алиасы внутренних пакетов.
// Новая сущность в internal/service без алиаса — видимый отказ: расширение
// публичной поверхности — осознанное решение, а не случайность.
package components_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// serviceExceptions — сущности сервисного слоя, не открытые алиасом:
// App и Config (фасадные объединяют сервисный слой, Handler и опции REST —
// план 01 §2.4) и Open (фасадный Open принимает расширенную Config).
var serviceExceptions = map[string]bool{
	"App":    true,
	"Config": true,
	"Open":   true,
}

// ownFacadeNames — собственные (не алиасные) экспортируемые имена фасада:
// App/Config/Open и тонкая обёртка AsError над domain.AsError.
var ownFacadeNames = map[string]bool{
	"App":     true,
	"Config":  true,
	"Open":    true,
	"AsError": true,
}

// internalPrefixes — пакеты, на которые фасаду разрешено ссылаться в
// целях алиасов (только внутренние слои; реэкспорт stdlib и внешних
// библиотек — не задача фасада).
var internalPrefixes = map[string]bool{
	"service":  true,
	"httpapi":  true,
	"importer": true,
	"domain":   true,
	"catalog":  true,
}

// pkgAPI — экспортированная поверхность пакета по AST (без _test.go):
// обычные типы, алиасы (типы и константы — с префиксом цели «service» и
// т.п.), собственные константы, функции и переменные.
type pkgAPI struct {
	plainTypes map[string]bool
	aliasTypes map[string]string
	aliasConst map[string]string
	ownConst   map[string]bool
	funcs      map[string]bool
	vars       map[string]bool
}

// collectAPI собирает поверхность пакета в dir. Ошибкой считается пустой
// результат — сломанный путь теста не должен проходить молча.
func collectAPI(t *testing.T, dir string) pkgAPI {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(d fs.FileInfo) bool {
		return !strings.HasSuffix(d.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	api := pkgAPI{
		plainTypes: map[string]bool{},
		aliasTypes: map[string]string{},
		aliasConst: map[string]string{},
		ownConst:   map[string]bool{},
		funcs:      map[string]bool{},
		vars:       map[string]bool{},
	}
	seen := false
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			seen = true
			for _, d := range f.Decls {
				switch decl := d.(type) {
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						collectSpec(t, api, decl, spec)
					}
				case *ast.FuncDecl:
					if decl.Recv == nil && decl.Name.IsExported() {
						api.funcs[decl.Name.Name] = true
					}
				}
			}
		}
	}
	if !seen {
		t.Fatalf("parse %s: файлов не найдено", dir)
	}
	return api
}

// collectSpec разбирает TypeSpec и ValueSpec декларации уровня пакета.
func collectSpec(t *testing.T, api pkgAPI, decl *ast.GenDecl, spec ast.Spec) {
	t.Helper()
	switch s := spec.(type) {
	case *ast.TypeSpec:
		if !s.Name.IsExported() {
			return
		}
		if s.Assign.IsValid() {
			api.aliasTypes[s.Name.Name] = targetPrefix(s.Type)
			return
		}
		api.plainTypes[s.Name.Name] = true
	case *ast.ValueSpec:
		for i, name := range s.Names {
			if !name.IsExported() {
				continue
			}
			switch decl.Tok {
			case token.CONST:
				// Алиас-константа — единственное имя со значением-селектором
				// внутреннего пакета (X = service.X); прочие — собственные.
				if len(s.Names) == 1 && i == 0 {
					if prefix := targetPrefix(s.Values[i]); prefix != "" {
						api.aliasConst[name.Name] = prefix
						continue
					}
				}
				api.ownConst[name.Name] = true
			case token.VAR:
				api.vars[name.Name] = true
			}
		}
	}
}

// targetPrefix — имя пакета-цели селектора (pkg.X → "pkg"), не селектор —
// пустая строка.
func targetPrefix(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// TestFacadeCoversServiceAPI — каждая экспортированная сущность сервисного
// слоя открыта фасадом: типы и константы — алиасами на service, функции —
// именем фасада (кроме исключений выше).
func TestFacadeCoversServiceAPI(t *testing.T) {
	svc := collectAPI(t, filepath.Join("..", "internal", "service"))
	facade := collectAPI(t, ".")
	for name := range svc.plainTypes {
		if serviceExceptions[name] {
			continue
		}
		if facade.aliasTypes[name] != "service" {
			t.Errorf("тип сервисного слоя %s не открыт алиасом components", name)
		}
	}
	for name := range svc.ownConst {
		if serviceExceptions[name] {
			continue
		}
		if facade.aliasConst[name] != "service" {
			t.Errorf("константа сервисного слоя %s не открыта алиасом components", name)
		}
	}
	for name := range svc.funcs {
		if serviceExceptions[name] {
			continue
		}
		if !facade.funcs[name] {
			t.Errorf("функция сервисного слоя %s не открыта в components", name)
		}
	}
}

// TestFacadeStaysThin — в фасаде только собственные App/Config/Open/AsError
// и алиасы внутренних пакетов: логика фасаду запрещена (AGENTS.md),
// реэкспорт без перестройки слоёв.
func TestFacadeStaysThin(t *testing.T) {
	facade := collectAPI(t, ".")
	for _, check := range []struct {
		names map[string]bool
		what  string
	}{
		{facade.plainTypes, "собственный тип"},
		{facade.funcs, "собственная функция"},
		{facade.vars, "переменная"},
	} {
		for name := range check.names {
			if !ownFacadeNames[name] {
				t.Errorf("%s %s в фасаде: собственные объявления — только %v",
					check.what, name, facadeOwnNames())
			}
		}
	}
	if len(facade.ownConst) != 0 {
		t.Errorf("собственные константы %v в фасаде: константы — только алиасы внутренних пакетов",
			keys(facade.ownConst))
	}
	for name, prefix := range facade.aliasTypes {
		if !internalPrefixes[prefix] {
			t.Errorf("алиас типа %s -> %s: цель вне внутренних пакетов", name, prefix)
		}
	}
	for name, prefix := range facade.aliasConst {
		if !internalPrefixes[prefix] {
			t.Errorf("алиас константы %s -> %s: цель вне внутренних пакетов", name, prefix)
		}
	}
}

// facadeOwnNames — собственные имена фасада для сообщений об отказе.
func facadeOwnNames() []string {
	return keys(ownFacadeNames)
}

// keys — отсортированные ключи отображения (стабильные сообщения теста).
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
