// Package storage — переносимое реляционное ядро справочника на database/sql:
// DML использует только переносимые конструкции (именованные параметры @имя,
// LIMIT, производные таблицы), совместимые с SQLite, PostgreSQL и MariaDB.
// Булевы значения хранятся как INTEGER 0/1, отсутствие необязательного
// значения — NULL; сравнение колонок, допускающих NULL, — переносимый предикат
// «col = @p OR (col IS NULL AND @p IS NULL)» без сенти́нелов.
package storage

import (
	"soviettransistors/internal/domain"
)

// UpsertOutcome — исход upsert: Added — вставлена новая запись; UpdatedExisting —
// запись существовала, применены заданные секции details; Skipped — запись
// существует, применять было нечего.
type UpsertOutcome int

const (
	UpsertAdded UpsertOutcome = iota
	UpsertUpdatedExisting
	UpsertSkipped
)

// TransistorDetails — данные транзистора; nil-секция означает «раздел не задан —
// не менять», пустая непустая-nil — очистить.
type TransistorDetails struct {
	Attributes    *domain.TransistorAttributes
	Manufacturers []string
	Parameters    []domain.ElectricalParameter
	Ratings       *domain.MaximumRatings
}

// TransistorQuery — фильтр выборки обозначений; nil-поля не участвуют в фильтре.
type TransistorQuery struct {
	Material          *domain.SemiconductorMaterial
	Subclass          *rune
	IsAssembly        *bool
	Feature           *int
	DevelopmentNumber *int
	Letters           string
	Modification      *int
	ChipVariant       *int
	Limit             *int
}

// TransistorDatabase — хранилище транзисторов: обозначения, электрические
// параметры, предельные данные. Соединение — одно на экземпляр: открывается
// лениво при первом обращении и закрывается в Close.
type TransistorDatabase interface {
	EnsureCreated() error
	Save(transistor domain.Transistor, details *TransistorDetails) (UpsertOutcome, error)
	FindId(transistor domain.Transistor) (int, bool, error)
	FindMaterialEquivalents(transistor domain.Transistor) ([]domain.Transistor, error)
	Delete(transistor domain.Transistor) (bool, error)
	CountAll() (int, error)
	Query(query TransistorQuery) ([]domain.Transistor, error)
	GetAttributes(transistorId int) (*domain.TransistorAttributes, error)
	GetParameters(transistorId int) ([]domain.ElectricalParameter, error)
	GetRatings(transistorId int) (*domain.MaximumRatings, error)
	GetManufacturers(transistorId int) ([]string, error)
	Close() error
}
