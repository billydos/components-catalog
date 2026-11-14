package storage

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

// introspectMethods диалектов — только тесты эквивалентности «свежая ≡
// мигрированная» (план 09 §9): строки «таблица | колонки | индексы»,
// отсортированные; в производственный путь не входит и в интерфейс
// Dialect не поднимается. Снимки сравниваются строго в пределах одного
// диалекта (fresh против мигрированной той же СУБД): паритет типов v5
// приводит диалектные имена к переносимым (BIGINT ≡ INTEGER,
// DOUBLE PRECISION ≡ REAL, varchar/character varying ≡ TEXT), но прочие
// детали (метки PK-колонок, имена автоиндексов уникальности sqlite против
// имён констрейнтов postgres) между диалектами дословно не совпадают
// и не обязаны.

// normalizeTypeName приводит имя типа колонки к переносимому имени DDL
// (INTEGER/REAL/TEXT); неизвестные типы — в верхнем регистре как есть
// (рассинхрон заметен сравнением, а не глотается).
func normalizeTypeName(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "integer", "bigint", "int", "int8", "smallint":
		return "INTEGER"
	case "real", "double precision", "float8", "float4":
		return "REAL"
	case "text", "varchar", "character varying", "character", "cstring":
		return "TEXT"
	}
	return strings.ToUpper(strings.TrimSpace(t))
}

// introspectTables — имена пользовательских таблиц, отсортированные:
// внутренние таблицы статистики sqlite (sqlite_*) исключаются — их
// создаёт ANALYZE массовой записи, к схеме модуля они не относятся.
func (sqliteDialect) introspect(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]string, 0, len(tables))
	for _, table := range tables {
		cols, err := sqliteColumns(ctx, db, table)
		if err != nil {
			return nil, err
		}
		idxs, err := sqliteIndexes(ctx, db, table)
		if err != nil {
			return nil, err
		}
		out = append(out, table+"|"+cols+"|"+idxs)
	}
	return out, nil
}

// sqliteColumns — «имя тип, …» по порядку объявления колонок.
func sqliteColumns(ctx context.Context, db *sql.DB, table string) (string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	parts := make([]string, 0, 8)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return "", err
		}
		part := name + " " + normalizeTypeName(colType)
		if notNull == 1 {
			part += " NOT NULL"
		}
		if pk == 1 {
			part += " PK"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ","), rows.Err()
}

// sqliteIndexes — «имя(колонки)[u], …» по алфавиту имён: уникальные
// ограничения (sqlite_autoindex_*) включаются как индексы уникальности,
// автоиндексы первичного ключа исключаются (origin pk) — паритет
// postgres-интроспекции, где PK-индексы также исключены.
func sqliteIndexes(ctx context.Context, db *sql.DB, table string) (string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA index_list("+table+")")
	if err != nil {
		return "", err
	}
	type idx struct {
		name, cols string
		unique     bool
	}
	var list []idx
	for rows.Next() {
		var seq int
		var name string
		var unique, partial int
		var origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			return "", err
		}
		if origin == "pk" {
			continue
		}
		i := idx{name: name, unique: unique == 1}
		irows, err := db.QueryContext(ctx, "PRAGMA index_info("+name+")")
		if err != nil {
			rows.Close()
			return "", err
		}
		var cols []string
		for irows.Next() {
			var seqno, cid int
			var colName sql.NullString
			if err := irows.Scan(&seqno, &cid, &colName); err != nil {
				irows.Close()
				rows.Close()
				return "", err
			}
			cols = append(cols, colName.String)
		}
		irows.Close()
		if err := irows.Err(); err != nil {
			rows.Close()
			return "", err
		}
		i.cols = strings.Join(cols, ",")
		list = append(list, i)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()
	sort.Slice(list, func(a, b int) bool { return list[a].name < list[b].name })
	parts := make([]string, 0, len(list))
	for _, i := range list {
		if i.unique {
			parts = append(parts, i.name+"("+i.cols+")[u]")
			continue
		}
		parts = append(parts, i.name+"("+i.cols+")")
	}
	return strings.Join(parts, ","), nil
}

// introspect postgres — information_schema.columns для колонок и
// pg_index/pg_attribute для индексов; PK-индексы исключены (паритет
// sqlite-интроспекции), уникальные ограничения включены.
func (postgresDialect) introspect(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'
ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]string, 0, len(tables))
	for _, table := range tables {
		crow, err := db.QueryContext(ctx, `
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = $1
ORDER BY ordinal_position`, table)
		if err != nil {
			return nil, err
		}
		parts := make([]string, 0, 8)
		for crow.Next() {
			var name, dataType, nullable string
			if err := crow.Scan(&name, &dataType, &nullable); err != nil {
				crow.Close()
				return nil, err
			}
			part := name + " " + normalizeTypeName(dataType)
			if nullable == "NO" {
				part += " NOT NULL"
			}
			parts = append(parts, part)
		}
		if err := crow.Err(); err != nil {
			crow.Close()
			return nil, err
		}
		crow.Close()
		irow, err := db.QueryContext(ctx, `
SELECT i.relname, ix.indisunique,
       (SELECT string_agg(a.attname, ',' ORDER BY k.ord)
          FROM unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum)
FROM pg_index ix
JOIN pg_class c ON c.oid = ix.indrelid
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relname = $1 AND n.nspname = current_schema() AND NOT ix.indisprimary
ORDER BY i.relname`, table)
		if err != nil {
			return nil, err
		}
		var idxs []string
		for irow.Next() {
			var name, cols string
			var unique bool
			if err := irow.Scan(&name, &unique, &cols); err != nil {
				irow.Close()
				return nil, err
			}
			if unique {
				idxs = append(idxs, name+"("+cols+")[u]")
				continue
			}
			idxs = append(idxs, name+"("+cols+")")
		}
		if err := irow.Err(); err != nil {
			irow.Close()
			return nil, err
		}
		irow.Close()
		out = append(out, table+"|"+strings.Join(parts, ",")+"|"+strings.Join(idxs, ","))
	}
	return out, nil
}
