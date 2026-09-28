package storage

import (
	"fmt"
	"strings"

	"soviettransistors/internal/domain"
)

// electricalParametersCreateTableSql — сборка DDL таблицы electrical_parameters
// из каталога: список кодов, правила границ и комбинации условий генерируются
// из ParameterCatalog и не дублируются вручную.
func electricalParametersCreateTableSql() string {
	var sql strings.Builder
	sql.WriteString("CREATE TABLE IF NOT EXISTS electrical_parameters (\n")
	sql.WriteString("    Id            INTEGER PRIMARY KEY AUTOINCREMENT,\n")
	sql.WriteString("    TransistorId  INTEGER NOT NULL REFERENCES transistors(Id) ON DELETE CASCADE,\n")
	fmt.Fprintf(&sql, "    Parameter     TEXT    NOT NULL,   -- %s\n", strings.Join(orderedParameterCodes(), " | "))
	sql.WriteString("    ValueMin      REAL        NULL,   -- «не менее» / нижняя граница (единица — из каталога параметров)\n")
	sql.WriteString("    ValueMax      REAL        NULL,   -- «не более» / верхняя граница\n")
	for _, key := range domain.AllConditionKeys {
		fmt.Fprintf(&sql, "    %-14s%-12sNULL,   -- условие: %s\n", string(key), "REAL", domain.ConditionDescription(key))
	}
	sql.WriteString("    Temp          REAL        NULL,   -- условие: температура среды, °C (допустима у любого параметра)\n")
	fmt.Fprintf(&sql, "    CONSTRAINT chk_param_code CHECK (Parameter IN (%s)),\n", quotedCodeList())
	appendValueBoundsSql(&sql)
	appendConditionsSql(&sql)
	sql.WriteString("    );")
	return sql.String()
}

func appendValueBoundsSql(sql *strings.Builder) {
	sql.WriteString("    CONSTRAINT chk_param_value CHECK (\n")
	sql.WriteString("        CASE Parameter\n")
	for _, info := range domain.ParameterCatalog {
		bounds := ""
		switch info.Direction {
		case domain.AtLeast:
			bounds = "ValueMin IS NOT NULL AND ValueMax IS NULL"
		case domain.AtMost:
			bounds = "ValueMax IS NOT NULL AND ValueMin IS NULL"
		case domain.AtLeastOrRange:
			bounds = "ValueMin IS NOT NULL AND (ValueMax IS NULL OR ValueMin <= ValueMax)"
		}
		if info.ValueCeiling != nil {
			limit := domain.Fmt(*info.ValueCeiling)
			bounds += fmt.Sprintf(" AND (ValueMin IS NULL OR ValueMin <= %s) AND (ValueMax IS NULL OR ValueMax <= %s)", limit, limit)
		}
		fmt.Fprintf(sql, "            WHEN '%s' THEN CASE WHEN %s THEN 1 ELSE 0 END\n", info.Code, bounds)
	}
	sql.WriteString("            ELSE 0\n")
	sql.WriteString("        END = 1\n")
	sql.WriteString("        AND (ValueMin IS NULL OR ValueMin > 0)\n")
	sql.WriteString("        AND (ValueMax IS NULL OR ValueMax > 0)\n")
	sql.WriteString("    ),\n")
}

func appendConditionsSql(sql *strings.Builder) {
	sql.WriteString("    CONSTRAINT chk_param_conditions CHECK (\n")
	sql.WriteString("        CASE Parameter\n")
	for _, info := range domain.ParameterCatalog {
		if len(info.Conditions.Variants) == 1 {
			fmt.Fprintf(sql, "            WHEN '%s' THEN CASE WHEN %s THEN 1 ELSE 0 END\n", info.Code, variantSql(info.Conditions.Variants[0]))
			continue
		}
		fmt.Fprintf(sql, "            WHEN '%s' THEN CASE WHEN\n", info.Code)
		for index, variant := range info.Conditions.Variants {
			expression := variantSql(variant)
			if index == 0 {
				fmt.Fprintf(sql, "                %s\n", expression)
			} else {
				fmt.Fprintf(sql, "             OR %s\n", expression)
			}
		}
		sql.WriteString("                THEN 1 ELSE 0 END\n")
	}
	sql.WriteString("            ELSE 0\n")
	sql.WriteString("        END = 1\n")
	// позитивность условий (кроме Temp — температура может быть отрицательной),
	// как в Domain-валидаторе
	for _, key := range domain.AllConditionKeys {
		fmt.Fprintf(sql, "        AND (%s IS NULL OR %s > 0)\n", string(key), string(key))
	}
	sql.WriteString("    )")
}

func variantSql(variant domain.ConditionVariant) string {
	var parts []string
	for _, key := range variant.RequiredKeys() {
		parts = append(parts, fmt.Sprintf("%s IS NOT NULL", string(key)))
	}
	for _, key := range forbiddenConditionKeys(variant) {
		parts = append(parts, fmt.Sprintf("%s IS NULL", string(key)))
	}
	return strings.Join(parts, " AND ")
}

func forbiddenConditionKeys(variant domain.ConditionVariant) []domain.ConditionKey {
	var keys []domain.ConditionKey
	for _, key := range domain.AllConditionKeys {
		if !variant.IsRequired(key) && !variant.IsOptional(key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// anyAttributeCheckSql — предикат «задан хотя бы один атрибут»; по три колонки
// на строку, как в остальном DDL. Список полей — единый источник
// domain.AttributeFieldNames.
func anyAttributeCheckSql() string {
	names := domain.AttributeFieldNames()
	var lines []string
	for index := 0; index < len(names); index += 3 {
		end := index + 3
		if end > len(names) {
			end = len(names)
		}
		parts := make([]string, 0, end-index)
		for _, name := range names[index:end] {
			parts = append(parts, fmt.Sprintf("%s IS NOT NULL", name))
		}
		lines = append(lines, strings.Join(parts, " OR "))
	}
	return strings.Join(lines, "\n        OR ")
}

func orderedParameterCodes() []string {
	codes := make([]string, len(domain.ParameterCatalog))
	for i, info := range domain.ParameterCatalog {
		codes[i] = info.Code
	}
	return codes
}

func quotedCodeList() string {
	codes := orderedParameterCodes()
	quoted := make([]string, len(codes))
	for i, code := range codes {
		quoted[i] = "'" + code + "'"
	}
	return strings.Join(quoted, ", ")
}
