// Пин-тест полноты фасада: внутренний сервисный слой — публичный API
// библиотеки (план 01 §2.4), его типы открываются фасадом целиком, а сам
// фасад остаётся тонким — кроме собственных App и Config в нём только
// алиасы. Новый тип в internal/service без алиаса — видимый отказ:
// расширение публичной поверхности — осознанное решение, а не случайность.
package components_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// facadeTypeExceptions — типы сервисного слоя, не открываемые алиасом:
// App (фасадный App объединяет сервисный слой и Handler — план 01 §2.4)
// и Config (фасадная Config включает и опции REST-транспорта).
var facadeTypeExceptions = map[string]bool{
	"App":    true,
	"Config": true,
}

// collectTypes собирает экспортируемые имена типов пакета: обычные
// объявления и алиасы («=») отдельными наборами.
func collectTypes(t *testing.T, dir string) (plain, aliases map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	plain, aliases = map[string]bool{}, map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					if ts.Assign.IsValid() {
						aliases[ts.Name.Name] = true
						continue
					}
					plain[ts.Name.Name] = true
				}
			}
		}
	}
	return plain, aliases
}

// TestFacadeCoversServiceTypes — каждый экспортированный тип сервисного
// слоя имеет алиас в фасаде (кроме исключений выше).
func TestFacadeCoversServiceTypes(t *testing.T) {
	svcPlain, _ := collectTypes(t, filepath.Join("..", "internal", "service"))
	_, facadeAliases := collectTypes(t, ".")
	for name := range svcPlain {
		if facadeTypeExceptions[name] {
			continue
		}
		if !facadeAliases[name] {
			t.Errorf("тип сервисного слоя %s не открыт фасадом (алиас в components)", name)
		}
	}
}

// TestFacadeStaysThin — в фасаде только алиасы и два собственных типа:
// логика фасаду запрещена (AGENTS.md), реэкспорт без перестройки слоёв.
func TestFacadeStaysThin(t *testing.T) {
	plain, _ := collectTypes(t, ".")
	for name := range plain {
		if !facadeTypeExceptions[name] {
			t.Errorf("собственный тип %s в фасаде: кроме App/Config — только алиасы", name)
		}
	}
}
