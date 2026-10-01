// Package storage — переносимое реляционное ядро на database/sql с двумя
// диалектами: sqlite (modernc.org/sqlite) и postgres (pgx/v5 stdlib).
//
// Бизнес-правил не содержит: только переносимый DML, диалектные различия
// (DDL, открытие, пулы) и версия схемы (plan/01-architecture.md §2.3).
package storage
