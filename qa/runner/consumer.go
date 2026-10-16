// Фаза consumer: пример встраивания examples/embed — внешний модуль,
// зависимость на библиотеку подключена require + replace на локальный
// путь, как у сайт-проекта. Компилируемость фасада components у внешнего
// потребителя — часть контракта (CC-9): внутримодульные тесты позволяют
// импорты internal/ и подлинности не доказывают. Фаза собирает пример
// его собственным go build и запускает бинарник (одноразовая база sqlite
// — пример создаёт и удаляет её сам).
package main

import (
	"context"
	"path/filepath"
)

// consumerPhase собирает и исполняет пример встраивания.
func consumerPhase(ctx context.Context, r *report, root, tmp string) {
	r.section("phase: consumer")
	dir := filepath.Join(root, "examples", "embed")
	bin := filepath.Join(tmp, "embed-example"+exeSuffix())
	res := runCmd(ctx, dir, buildTimeout, "go", "build", "-o", bin, ".")
	if !r.check("consumer/build", "go build examples/embed (внешний модуль, replace ../..)",
		res.code == 0, envDetails(res, 20)...) {
		return
	}
	res = runCmd(ctx, tmp, cmdTimeout, bin)
	r.check("consumer/run", "embed-пример: импорт, поиск, REST, экспорт",
		res.code == 0, envDetails(res, 30)...)
}
