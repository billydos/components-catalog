package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"soviettransistors/internal/domain"
)

func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}

func printTransistor(t domain.Transistor) {
	kind, _ := domain.MaterialKindOf(t.Material)
	subclassText := "полевой"
	if t.Subclass == 'Т' {
		subclassText = "биполярный"
	}
	modification := "—"
	if t.Modification != nil {
		modification = fmt.Sprintf("%d", *t.Modification)
	}
	chip := "—"
	if t.ChipVariant != nil {
		chip = fmt.Sprintf("-%d", *t.ChipVariant)
	}
	fmt.Printf("Обозначение:      %s\n", t.Name())
	fmt.Printf("Материал:         %c — %s\n", t.Material, domain.MaterialDisplayName(kind))
	fmt.Printf("Подкласс:         %c — %s\n", t.Subclass, subclassText)
	fmt.Printf("Сборка:           %s\n", yesNoAssembly(t.IsAssembly))
	fmt.Printf("Признак:          %d\n", t.Feature)
	fmt.Printf("Номер разработки: %02d\n", t.DevelopmentNumber)
	fmt.Printf("Классификация:    %s\n", t.Letters)
	fmt.Printf("Модификация:      %s\n", modification)
	fmt.Printf("Бескорпусное:     %s\n", chip)
	fmt.Println()
}

func yesNoAssembly(value bool) string {
	if value {
		return "да (С)"
	}
	return "нет"
}

func printTable(rows []domain.Transistor) {
	if len(rows) == 0 {
		fmt.Println("Записей нет.")
		return
	}

	headers := []string{"Обозначение", "Материал", "Подкл.", "Сборка", "Признак", "№", "Буквы", "Мод.", "Бескорп."}
	table := make([][]string, len(rows))
	for i, r := range rows {
		kind, _ := domain.MaterialKindOf(r.Material)
		modification := "—"
		if r.Modification != nil {
			modification = fmt.Sprintf("%d", *r.Modification)
		}
		chip := "—"
		if r.ChipVariant != nil {
			chip = fmt.Sprintf("-%d", *r.ChipVariant)
		}
		assembly := "—"
		if r.IsAssembly {
			assembly = "С"
		}
		table[i] = []string{
			r.Name(),
			fmt.Sprintf("%c %s", r.Material, domain.MaterialShortName(kind)),
			string(r.Subclass),
			assembly,
			fmt.Sprintf("%d", r.Feature),
			fmt.Sprintf("%02d", r.DevelopmentNumber),
			r.Letters,
			modification,
			chip,
		}
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
		for _, row := range table {
			if width := utf8.RuneCountInString(row[i]); width > widths[i] {
				widths[i] = width
			}
		}
	}

	printPaddedRow(headers, widths)
	separator := make([]string, len(widths))
	for i, width := range widths {
		separator[i] = strings.Repeat("-", width)
	}
	fmt.Println(strings.Join(separator, "  "))
	for _, row := range table {
		printPaddedRow(row, widths)
	}
}

func printPaddedRow(values []string, widths []int) {
	padded := make([]string, len(values))
	for i, value := range values {
		padded[i] = value + strings.Repeat(" ", widths[i]-utf8.RuneCountInString(value))
	}
	fmt.Println(strings.Join(padded, "  "))
}
