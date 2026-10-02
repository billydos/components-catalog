package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Интеграционный набор этапа 6 (план работ 6.1–6.3): проверенные данные
// data/ — импорт на SQLite (память/файл) и PostgreSQL, идемпотентность,
// разбор обозначений всех классов, матрица исполнений К50-35, ряд
// мощностей С2-33Н, направленность аналогов и пин прослеживаемости
// (каждая запись атрибутирована ТУ либо даташитом — отчёт
// docs/plan/08-data-verification.md). Критерии этапа: значения наполнения
// прослеживаются до ТУ/даташитов/листов спецификаций; примеры
// обозначений всех классов парсятся и импортируются.

// verifiedFiles — файлы проверенного наполнения по классам.
var verifiedFiles = []string{
	"transistors.jsonc", "diodes.jsonc", "resistors.jsonc", "capacitors.jsonc",
}

// dataPath — путь к проверенным данным этапа 6.
func dataPath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "data", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("data/%s недоступен: %v", name, err)
	}
	return path
}

// readVerifiedDocument читает файл data/ в документ записей.
func readVerifiedDocument(t *testing.T, app *service.App, name string) (*Document, map[string][]service.DeviceInput) {
	t.Helper()
	path := dataPath(t, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("data/%s: %v", name, err)
	}
	root, err := parseTree(data, FormatJSONC)
	if err != nil {
		t.Fatalf("разбор data/%s: %v", name, err)
	}
	snap, err := app.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("снимок: %v", err)
	}
	doc, issues := ReadDocument(root, snap)
	if len(issues) != 0 {
		t.Fatalf("data/%s: проблемы чтения: %v", name, issueMessages(issues))
	}
	byKind := map[string][]service.DeviceInput{}
	for _, rec := range doc.Records {
		byKind[string(rec.Kind)] = append(byKind[string(rec.Kind)], rec.Input)
	}
	return doc, byKind
}

func TestVerifiedSQLiteMemory(t *testing.T) { runVerifiedSuite(t, memoryConfig) }
func TestVerifiedSQLiteFile(t *testing.T)   { runVerifiedSuite(t, fileConfig) }
func TestVerifiedPostgres(t *testing.T)     { runVerifiedSuite(t, postgresConfig) }

func runVerifiedSuite(t *testing.T, factory configFactory) {
	// Минимальный состав выборки этапа 6.3: транзисторы ≈20 записей
	// (не менее), записи каждого из остальных классов (не менее 8).
	minRecords := map[domain.Kind]int{
		domain.KindTransistor: 20, domain.KindDiode: 8,
		domain.KindResistor: 8, domain.KindCapacitor: 8,
	}

	app := openApp(t, factory)
	total := 0
	for _, name := range verifiedFiles {
		doc, byKind := readVerifiedDocument(t, app, name)
		for kind, inputs := range byKind {
			if len(inputs) < minRecords[domain.Kind(kind)] {
				t.Errorf("data/%s: класс %s: %d записей, минимум этапа 6.3 — %d",
					name, kind, len(inputs), minRecords[domain.Kind(kind)])
			}
		}
		total += len(doc.Records)

		rep := importFile(t, app, dataPath(t, name), false)
		if rep.HasIssues() {
			t.Fatalf("data/%s: проблемы: %v", name, issueMessages(rep.Issues))
		}
		if rep.Records != len(doc.Records) || rep.Added != len(doc.Records) {
			t.Fatalf("data/%s: записей %d, добавлено %d (в файле %d)",
				name, rep.Records, rep.Added, len(doc.Records))
		}
		// Повторный импорт — Skipped по всем записям без записи в БД.
		rep = importFile(t, app, dataPath(t, name), false)
		if rep.HasIssues() || rep.Skipped != len(doc.Records) || rep.Added != 0 || rep.Updated != 0 {
			t.Fatalf("повторный data/%s: %+v; проблемы: %v", name, rep, issueMessages(rep.Issues))
		}
	}

	t.Run("DesignationsParse", func(t *testing.T) {
		// Примеры обозначений всех классов парсятся (критерий этапа 6);
		// класс разбора совпадает с секцией файла. Система other
		// автодетекта не имеет — проверяется с явной системой записи.
		for _, name := range verifiedFiles {
			_, byKind := readVerifiedDocument(t, app, name)
			for kind, inputs := range byKind {
				for _, in := range inputs {
					var (
						p   domain.ParsedDesignation
						err error
					)
					if in.System == domain.SystemOther {
						p, err = app.Services().Designations.ParseForSystem(
							context.Background(), in.Name, in.System, domain.Kind(kind))
					} else {
						p, err = app.Services().Designations.Parse(context.Background(), in.Name)
					}
					if err != nil {
						t.Errorf("%s: обозначение «%s» не парсится: %v", name, in.Name, err)
						continue
					}
					if string(p.Kind) != kind {
						t.Errorf("%s: «%s»: класс разбора %s, секция %s", name, in.Name, p.Kind, kind)
					}
					if in.System != "" && p.System != in.System {
						t.Errorf("%s: «%s»: система разбора %s, в записи %s", name, in.Name, p.System, in.System)
					}
				}
			}
		}
	})

	t.Run("TraceabilityPin", func(t *testing.T) {
		// Прослеживаемость: каждая запись несёт ТУ либо ссылку на
		// даташит/лист спецификации (источники — отчёт 08).
		for _, name := range verifiedFiles {
			_, byKind := readVerifiedDocument(t, app, name)
			for kind, inputs := range byKind {
				for _, in := range inputs {
					var tu, url bool
					for _, a := range in.Attributes {
						switch a.Attribute {
						case "tu":
							tu = a.Text != nil && *a.Text != ""
						case "datasheetUrl":
							url = a.Text != nil && *a.Text != ""
						}
					}
					if !tu && !url {
						t.Errorf("data/%s: %s «%s»: нет ни ТУ, ни datasheetUrl", name, kind, in.Name)
					}
				}
			}
		}
	})

	t.Run("CapacitorVariantMatrix", func(t *testing.T) {
		// Матрица исполнений К50-35: ряд напряжений с ёмкостью и
		// цилиндрическими габаритами («ёмкость × напряжение → габариты»).
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindCapacitor, "К50-35")
		if err != nil || !found {
			t.Fatalf("карточка К50-35: %v %v", found, err)
		}
		if len(card.Variants) < 8 {
			t.Fatalf("К50-35: исполнений %d, ожидался ряд напряжений (≥8)", len(card.Variants))
		}
		seenUnom := map[float64]bool{}
		for _, v := range card.Variants {
			var unom, diameter, mass float64
			hasCnom, hasDiameter := false, false
			for _, g := range v.Groups {
				for _, val := range g.Values {
					switch val.Parameter {
					case "Unom":
						if val.Exact != nil {
							unom = *val.Exact
						}
					case "Cnom":
						if val.Min != nil && val.Max != nil {
							hasCnom = true
						}
					case "diameter":
						if val.Exact != nil {
							diameter, hasDiameter = *val.Exact, true
						}
					case "massMax":
						if val.Exact != nil {
							mass = *val.Exact
						}
					}
				}
			}
			if !hasCnom {
				t.Errorf("вариант «%s»: нет диапазона Cnom", v.Label)
			}
			if hasDiameter && mass <= 0 {
				t.Errorf("вариант «%s»: габариты без массы", v.Label)
			}
			if diameter > 0 && diameter < 4 {
				t.Errorf("вариант «%s»: подозрительный диаметр %v мм", v.Label, diameter)
			}
			if seenUnom[unom] {
				t.Errorf("вариант «%s»: Unom %v повторяется", v.Label, unom)
			}
			seenUnom[unom] = true
		}
	})

	t.Run("ResistorPowerVariants", func(t *testing.T) {
		// Ряд мощностей С2-33Н задан исполнениями с обязательным Pnom
		// (D6): 0.125–2 Вт.
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindResistor, "С2-33Н")
		if err != nil || !found {
			t.Fatalf("карточка С2-33Н: %v %v", found, err)
		}
		want := map[float64]bool{0.125: false, 0.25: false, 0.5: false, 1: false, 2: false}
		for _, v := range card.Variants {
			var pnom float64
			hasPnom := false
			for _, g := range v.Groups {
				for _, val := range g.Values {
					if val.Parameter == "Pnom" && val.Exact != nil {
						pnom, hasPnom = *val.Exact, true
					}
				}
			}
			if !hasPnom {
				t.Fatalf("вариант «%s»: нет Pnom", v.Label)
			}
			if _, ok := want[pnom]; !ok {
				t.Errorf("вариант «%s»: неожиданный Pnom %v", v.Label, pnom)
				continue
			}
			want[pnom] = true
		}
		for p, ok := range want {
			if !ok {
				t.Errorf("С2-33Н: отсутствует исполнение Pnom=%v Вт", p)
			}
		}
	})

	t.Run("DirectedAnalogLinks", func(t *testing.T) {
		// Аналоги направлены (D8), источник каждого ребра — отчёт 08 §5:
		// КТ315Б → 2N3904, КТ3102Б → BC547B (таблица аналогов qrx),
		// Д226 → 1N4007 и встречное 1N4007 → Д226 — оба направления
		// самостоятельны. Встречная ссылка не создаёт обратной исходящей.
		edges := []struct{ from, to string }{
			{"КТ315Б", "2N3904"},
			{"КТ3102Б", "BC547B"},
			{"Д226", "1N4007"},
			{"1N4007", "Д226"},
		}
		for _, e := range edges {
			card, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, e.from)
			if e.from == "Д226" || e.from == "1N4007" {
				card, found, err = app.Services().Devices.Get(context.Background(), domain.KindDiode, e.from)
			}
			if err != nil || !found {
				t.Fatalf("карточка %s: %v %v", e.from, found, err)
			}
			ok := false
			for _, a := range card.Analogs {
				if a.Designation == e.to {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s: исходящая ссылка «%s» отсутствует", e.from, e.to)
			}
			peer, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, e.to)
			if e.to == "Д226" || e.to == "1N4007" {
				peer, found, err = app.Services().Devices.Get(context.Background(), domain.KindDiode, e.to)
			}
			if err != nil || !found {
				t.Fatalf("карточка %s: %v %v", e.to, found, err)
			}
			back := false
			for _, b := range peer.Backlinks {
				if b.Designation == e.from {
					back = true
				}
			}
			if !back {
				t.Errorf("%s: нет встречной ссылки от %s", e.to, e.from)
			}
		}
		// 2N3904 не заявляет КТ315Б аналогом: направленность не создаёт
		// обратного ребра автоматически.
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, "2N3904")
		if err != nil || !found {
			t.Fatalf("карточка 2N3904: %v %v", found, err)
		}
		for _, a := range card.Analogs {
			if a.Designation == "КТ315Б" {
				t.Error("2N3904: встречная ссылка ошибочно создала исходящую на КТ315Б")
			}
		}
	})
}
