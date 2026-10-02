package importer

import (
	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// ReadRecordJSON читает одну запись наполнения из тела REST-запроса
// (POST/PUT /api/v1/components — plan/04-module-functionality.md §2):
// строка-обозначение либо объект «name/system + секции». Разбор — общим
// читателем формата (дубликаты ключей, форма секций, тексты проблем);
// класс записи определяется автодетектом по обозначению (для system other
// создание адресуется PUT с классом в пути). Возвращается первая проблема
// (REST применяет одну запись, Issues-накопление прогона здесь нет).
func ReadRecordJSON(data []byte, snap *catalog.Snapshot) (service.DeviceInput, error) {
	v, err := parseTree(data, FormatJSONC)
	if err != nil {
		return service.DeviceInput{}, err
	}
	r := &reader{snap: snap}
	in, ok := r.record("", v, 0, "тело запроса")
	if !ok {
		issue := r.issues[0]
		return service.DeviceInput{}, domain.NewError(issue.Code, issue.Message)
	}
	return in, nil
}
