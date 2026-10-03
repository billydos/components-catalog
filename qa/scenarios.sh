#!/usr/bin/env bash
# qa/scenarios.sh — сквозные сценарные прогоны CLI и REST на SQLite и
# PostgreSQL (этап 7.1, docs/plan/05-work-plan.md). SQLite — всегда; PostgreSQL —
# при заданном CATALOG_TEST_POSTGRES_DSN (в CI поднимается сервисом).
#
# Прогон: ./qa/scenarios.sh
# Код выхода 0 — все проверки зелёные; иначе печатается число отказов.

set -euo pipefail
cd "$(dirname "$0")/.."

PASS=0
FAIL=0

check() { # check <имя> <статус (0 — успех)>
  if [ "$2" -eq 0 ]; then
    PASS=$((PASS + 1))
    echo "  ok    $1"
  else
    FAIL=$((FAIL + 1))
    echo "  FAIL  $1"
  fi
}

ok_if() { # ok_if <условие-команда...> — статус для check
  "$@"
}

contains() { printf '%s' "$1" | grep -qF "$2"; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
go build -o "$TMP/catalogctl" ./cmd/catalogctl
go build -o "$TMP/restsrv" ./qa/restsrv
CTL="$TMP/catalogctl"

# ---------------------------------------------------------------------------
# Сценарии CLI
# ---------------------------------------------------------------------------
# cli_suite <метка> <флаги подключения...>
cli_suite() {
  local label="$1"
  shift
  local db=("$@")

  echo "== CLI: $label =="

  local out rc
  out="$("$CTL" init "${db[@]}" 2>&1)"
  check "init: база инициализирована" $(ok_if contains "$out" "инициализирована"; echo $?)

  local total=0 imported n_tr=0 n_caps=0 n_diodes=0
  for f in data/transistors.jsonc data/diodes.jsonc data/resistors.jsonc data/capacitors.jsonc; do
    out="$("$CTL" import "$f" "${db[@]}" 2>&1)"
    imported="$(printf '%s' "$out" | sed -n 's/.*записей: \([0-9]*\),.*/\1/p')"
    total=$((total + imported))
    case "$f" in
      *transistors*) n_tr="$imported" ;;
      *capacitors*) n_caps="$imported" ;;
      *diodes*) n_diodes="$imported" ;;
    esac
  done
  check "import: выверенная выборка целиком ($total записей)" $([ "$total" -gt 0 ]; echo $?)

  out="$("$CTL" import data/transistors.jsonc "${db[@]}" 2>&1)"
  check "идемпотентность: повторный импорт — без изменений, без отказов" \
    $(ok_if contains "$out" "без изменений: $n_tr" && ! contains "$out" "отвергнуто"; echo $?)

  out="$("$CTL" count "${db[@]}" 2>&1)"
  check "count: $total записей" $([ "$out" = "$total" ]; echo $?)

  out="$("$CTL" list --kind transistor --q КТ3 --limit 10 "${db[@]}" 2>&1)"
  check "list: подстрока обозначения и фильтр класса" $(ok_if contains "$out" "КТ315Б"; echo $?)

  out="$("$CTL" list --kind resistor --system gost "${db[@]}" 2>&1)"
  check "list: фильтр системы обозначений" $(ok_if contains "$out" "С2-33Н"; echo $?)

  out="$("$CTL" info КТ315Б "${db[@]}" 2>&1)"
  check "info: карточка транзистора (система gost)" $(ok_if contains "$out" "(gost)"; echo $?)

  out="$("$CTL" info К50-35 "${db[@]}" 2>&1)"
  check "info: матрица исполнений конденсатора" $(ok_if contains "$out" "Исполнение"; echo $?)

  out="$("$CTL" find КТ315Б "${db[@]}" 2>&1)"
  check "find: точное совпадение" $(ok_if contains "$out" "КТ315Б — "; echo $?)

  rc=0
  out="$("$CTL" find 2Т315Б "${db[@]}" 2>&1)" || rc=$?
  check "find: подсказка равнозначного по материалу, код выхода 1" \
    $([ $rc -eq 1 ] && contains "$out" "равнозначная по материалу: КТ315Б"; echo $?)

  out="$("$CTL" parse --lang ru КТ315Б 2>&1)"
  check "parse: автодетект без базы" $(ok_if contains "$out" "транзистор" && contains "$out" "gost"; echo $?)

  out="$("$CTL" parse 2N2222 2>&1)"
  check "parse: JEDEC" $(ok_if contains "$out" "jedec"; echo $?)

  rc=0
  out="$("$CTL" parse 2222 2>&1)" || rc=$?
  check "parse: неверное обозначение — «Ошибка: », код 1" \
    $([ $rc -eq 1 ] && contains "$out" "Ошибка: "; echo $?)

  "$CTL" export --kind capacitor --format ndjson "${db[@]}" > "$TMP/caps.ndjson" 2>/dev/null
  out="$("$CTL" import "$TMP/caps.ndjson" "${db[@]}" 2>&1)"
  check "round-trip: экспорт ndjson → импорт — без изменений" \
    $(ok_if contains "$out" "без изменений: $n_caps" && ! contains "$out" "отвергнуто"; echo $?)

  "$CTL" export --format yaml "${db[@]}" > "$TMP/all.yaml" 2>/dev/null
  out="$("$CTL" import "$TMP/all.yaml" "${db[@]}" 2>&1)"
  check "round-trip: полный экспорт yaml → импорт — без изменений" \
    $(ok_if contains "$out" "без изменений: $total" && ! contains "$out" "отвергнуто"; echo $?)

  out="$("$CTL" import data/diodes.jsonc --dry-run "${db[@]}" 2>&1)"
  check "import --dry-run: контрольный прогон без записи" \
    $(ok_if contains "$out" "контрольный прогон" && contains "$out" "без изменений: $n_diodes"; echo $?)

  out="$("$CTL" delete КД522А "${db[@]}" 2>&1)"
  check "delete: удаление с каскадом" $(ok_if contains "$out" "удалено"; echo $?)

  rc=0
  out="$("$CTL" info КД522А "${db[@]}" 2>&1)" || rc=$?
  check "delete: запись отсутствует, код 1" \
    $([ $rc -eq 1 ] && contains "$out" "не найдена"; echo $?)

  out="$("$CTL" count "${db[@]}" 2>&1)"
  check "count: $((total - 1)) после удаления" $([ "$out" = "$((total - 1))" ]; echo $?)

  printf '{ broken json' > "$TMP/broken.jsonc"
  rc=0
  out="$("$CTL" import "$TMP/broken.jsonc" "${db[@]}" 2>&1)" || rc=$?
  check "import: битый файл — «Ошибка: », код 1" \
    $([ $rc -eq 1 ] && contains "$out" "Ошибка: "; echo $?)

  out="$("$CTL" catalog list "${db[@]}" 2>&1)"
  check "catalog list: справка из каталога" \
    $(ok_if contains "$out" "Параметры:" && contains "$out" "h21e"; echo $?)
}

# ---------------------------------------------------------------------------
# Сценарии REST (qa/restsrv — минимальный хост /api/v1)
# ---------------------------------------------------------------------------
# rest_suite <метка> <флаги подключения...>
rest_suite() {
  local label="$1"
  shift
  local db=("$@")

  echo "== REST: $label =="

  local port=$((17000 + RANDOM % 2000))
  local base="http://127.0.0.1:$port/api/v1"
  "$TMP/restsrv" --addr "127.0.0.1:$port" "${db[@]}" 2>"$TMP/restsrv.log" &
  local srv_pid=$!

  local ready=1 i
  for i in $(seq 1 100); do
    if curl -sf "$base/kinds" >/dev/null 2>&1; then ready=0; break; fi
    sleep 0.1
  done
  check "restsrv: готовность" $ready

  # hcode — статус ответа; тело — в $TMP/body
  hcode() { curl -s -o "$TMP/body" -w '%{http_code}' "$@"; }
  body() { cat "$TMP/body"; }

  local sc bodytext etag
  sc="$(hcode "$base/kinds")"
  bodytext="$(body)"
  check "GET /kinds: 200, классы" $([ "$sc" = "200" ] && contains "$bodytext" "transistor"; echo $?)

  sc="$(hcode "$base/catalog")"
  bodytext="$(body)"
  check "GET /catalog: 200, снимок каталога" \
    $([ "$sc" = "200" ] && contains "$bodytext" '"parameters"'; echo $?)

  etag="$(curl -s -D - -o /dev/null "$base/catalog" | tr -d '\r' | awk 'tolower($1)=="etag:"{print $2}')"
  sc="$(hcode -H "If-None-Match: $etag" "$base/catalog")"
  check "GET /catalog: ETag → 304" $([ "$sc" = "304" ]; echo $?)

  sc="$(hcode "$base/stats")"
  bodytext="$(body)"
  check "GET /stats: 200, ревизии" \
    $([ "$sc" = "200" ] && contains "$bodytext" "data_revision"; echo $?)

  sc="$(hcode "$base/components?kind=capacitor&q=%D0%9A50")"
  bodytext="$(body)"
  check "GET /components: поиск с фильтрами" \
    $([ "$sc" = "200" ] && contains "$bodytext" "К50-35"; echo $?)

  etag="$(curl -s -D - -o /dev/null "$base/components?kind=capacitor" | tr -d '\r' | awk 'tolower($1)=="etag:"{print $2}')"
  sc="$(hcode -H "If-None-Match: $etag" "$base/components?kind=capacitor")"
  check "GET /components: ETag → 304" $([ "$sc" = "304" ]; echo $?)

  sc="$(hcode "$base/components?kind=transistor&par.h21e.min=50")"
  check "GET /components: параметрический фильтр" $([ "$sc" = "200" ]; echo $?)

  sc="$(hcode "$base/components?kind=transistor&par.bogus.min=1")"
  bodytext="$(body)"
  check "GET /components: неизвестный параметр — 400, код unknown_parameter" \
    $([ "$sc" = "400" ] && contains "$bodytext" "unknown_parameter"; echo $?)

  sc="$(hcode "$base/components?kind=transistor&bogus=1")"
  bodytext="$(body)"
  check "GET /components: неизвестный ключ запроса — 400, код validation_failed" \
    $([ "$sc" = "400" ] && contains "$bodytext" "validation_failed"; echo $?)

  sc="$(hcode "$base/components/capacitor/%D0%9A50-35")"
  bodytext="$(body)"
  check "GET /components/{kind}/{designation}: карточка с исполнениями" \
    $([ "$sc" = "200" ] && contains "$bodytext" '"variants"'; echo $?)

  local id
  id="$(curl -s "$base/components?kind=transistor&limit=1" | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1)"
  sc="$(hcode "$base/components/id/$id")"
  check "GET /components/id/{id}: карточка по стабильному id" $([ "$sc" = "200" ]; echo $?)

  curl -s -o /dev/null -X DELETE "$base/components/transistor/2N9013" || true
  sc="$(hcode -X POST -H 'Content-Type: application/json' -d '{"name":"2N9013"}' "$base/components")"
  bodytext="$(body)"
  check "POST /components: создание — 201 Added" \
    $([ "$sc" = "201" ] && contains "$bodytext" '"Added"'; echo $?)

  sc="$(hcode -X POST -H 'Content-Type: application/json' -d '{"name":"2N9013"}' "$base/components")"
  bodytext="$(body)"
  check "POST /components: повторное создание — 409 already_exists" \
    $([ "$sc" = "409" ] && contains "$bodytext" "already_exists"; echo $?)

  sc="$(hcode -X PUT -H 'Content-Type: application/json' \
    -d '{"name":"2N9013","attributes":{"package":"TO-18"}}' "$base/components/transistor/2N9013")"
  bodytext="$(body)"
  check "PUT /components: семантика секций — 200, секция применена" \
    $([ "$sc" = "200" ] && contains "$bodytext" "TO-18"; echo $?)

  sc="$(hcode -X PUT -H 'Content-Type: application/json' \
    -d '{"name":"2N9014"}' "$base/components/transistor/2N9013")"
  bodytext="$(body)"
  check "PUT /components: несовпадение обозначения — 422 designation_mismatch" \
    $([ "$sc" = "422" ] && contains "$bodytext" "designation_mismatch"; echo $?)

  sc="$(hcode -X DELETE "$base/components/transistor/2N9013")"
  check "DELETE /components: удаление — 204" $([ "$sc" = "204" ]; echo $?)

  sc="$(hcode "$base/components/transistor/2N9013")"
  check "GET после DELETE — 404" $([ "$sc" = "404" ]; echo $?)

  sc="$(hcode "$base/suggest?q=%D0%9A%D0%A23&kind=transistor")"
  bodytext="$(body)"
  check "GET /suggest: автодополнение по префиксу" \
    $([ "$sc" = "200" ] && contains "$bodytext" "designation"; echo $?)

  sc="$(hcode "$base/bogus")"
  check "неизвестный маршрут — 404" $([ "$sc" = "404" ]; echo $?)

  kill "$srv_pid" 2>/dev/null || true
  wait "$srv_pid" 2>/dev/null || true
}

set +e # отказ проверки не прерывает прогон: итог — число отказов

cli_suite sqlite --db "$TMP/qa.db"
rest_suite sqlite --db "$TMP/qa.db"

if [ -n "${CATALOG_TEST_POSTGRES_DSN:-}" ]; then
  cli_suite postgres --dialect postgres --dsn "$CATALOG_TEST_POSTGRES_DSN"
  rest_suite postgres --dialect postgres --dsn "$CATALOG_TEST_POSTGRES_DSN"
else
  echo "== PostgreSQL: CATALOG_TEST_POSTGRES_DSN не задан — нога опущена =="
fi

echo
echo "итог: ok=$PASS fail=$FAIL"
[ "$FAIL" -eq 0 ]
