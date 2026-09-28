package domain

import (
	"fmt"
	"strings"
)

// Transistor — советский транзистор по системе обозначений ГОСТ 10862.
type Transistor struct {
	// Материал: Г|1, К|2, А|3, И|4 (хранится как в обозначении).
	Material rune
	// Подкласс: Т — биполярный, П — полевой.
	Subclass rune
	// Сборка (буква С в обозначении).
	IsAssembly bool
	// Характерный эксплуатационный признак: 1–9.
	Feature int
	// Порядковый номер разработки: 1–999 (в обозначении 01–999).
	DevelopmentNumber int
	// Классификация по параметрам: одна или две заглавные русские буквы.
	Letters string
	// Модификация: 1–9 или nil.
	Modification *int
	// Бескорпусное исполнение (через дефис): 1–6 или nil.
	ChipVariant *int
}

// Name — полное обозначение, например КТ315Б или 2Т914А-1.
func (t Transistor) Name() string {
	var sb strings.Builder
	sb.WriteRune(t.Material)
	sb.WriteRune(t.Subclass)
	if t.IsAssembly {
		sb.WriteRune('С')
	}
	fmt.Fprintf(&sb, "%d", t.Feature)
	fmt.Fprintf(&sb, "%02d", t.DevelopmentNumber)
	sb.WriteString(t.Letters)
	if t.Modification != nil {
		fmt.Fprintf(&sb, "%d", *t.Modification)
	}
	if t.ChipVariant != nil {
		fmt.Fprintf(&sb, "-%d", *t.ChipVariant)
	}
	return sb.String()
}
