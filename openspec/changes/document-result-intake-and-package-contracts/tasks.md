## 1. Исход работы читается сводкой

- [x] 1.1 `run status` в текстовом виде печатает строку на шаг с известным вердиктом: id, статус, вердикт и число запечатанных выходов. Тесты: после принятого результата вердикт есть в тексте без `--json`, для Run без шагов лишних строк нет (`go test ./cmd/prifly ./internal/runtime -run 'Status|Render'`). (сделано: строка `step <id> status=… verdict=… outputs=…` печатается для шагов с известным вердиктом, в отсортированном порядке; `TestRunSummaryPrintsStepVerdicts` и `TestAcceptedVerdictIsVisibleInTheRunState`)

## 2. Контракты читаются из инструмента

- [x] 2.1 `package inspect --component ID` отдаёт bytes компонента с этим declared ID из установленного trusted package; `--ref` работает как прежде. Отказ команды схем на неизвестное имя называет эту команду, чтобы поиск не заканчивался тупиком. Тесты: объявленная schema отдаётся байт-в-байт, неизвестный ID отказывает именованно, `schema` остаётся функцией binary (`go test ./cmd/prifly -run 'Package|Schema'`). (сделано: `PackageComponent` ищет компонент по declared ID среди установленных trusted packages, читает его из sealed manifest и сверяет digest; отказ `package_component_not_found` называет предмет, а `schema` на имя с двоеточием указывает на эту команду; проверено на живом пакете — `aif:schema/implementation` отдался с digest)
- [x] 2.2 `prifly schema NAME --def DEF` отдаёт одно определение вместе с его закрытием по `$ref`. Тесты: результат валиден как JSON Schema и содержит только достижимые определения, неизвестное `--def` отказывает именованно, без флага поведение прежнее (`go test ./cmd/prifly -run Schema`). (сделано: одно определение с замыканием по `$ref` — 4008 байт вместо 222694 для `SessionSubmissionV5`; неизвестное имя отказывает с pointer `/$defs/<name>`)

## 3. Форма публикации выхода объявлена

- [x] 3.1 Записать в published contract, как host заполняет слот, который движок не закрывает захватом: bytes в объявленный путь слота, `artifact_id` и `revision` из `context.json`, digest записанных bytes. Отразить это в authoring reference шага. Проверить `make examples` и что описание совпадает с тем, что проверяет `readResultOutputs` (сделано: форма записана в инструкции сгенерированного runner-навыка и в authoring reference шага; правка в baseline `StepResult` отменена — её digest закреплён встроенным определением `core:schema/step-result`, на которое запечатанные пакеты ссылаются по отпечатку, и аннотация разорвала бы уже подписанные identity; формулировка требования исправлена под это; `make examples` зелёный).

## 4. Каталог не обещает лишнего

- [x] 4.1 `ValidateDecisionDefinition` отказывает решению фазы `runtime` с `required: true`, называя, что обязательность доступна только фазе `preflight`. Тесты: такой каталог не компилируется, preflight-решение с флагом принимается как прежде, поставляемые пакеты компилируются (`go test ./internal/runtime -run Decision`, `python3 -B tests/verify.py` в репозитории сценариев по возможности) (сделано: `decision_required_unenforceable` при `phase: runtime` и `required: true`, preflight не затронут; пять тестовых фикстур bridge несли этот флаг без смысла и очищены; поставляемый `aif-classic` его не объявляет, поэтому пакеты не затронуты).

## 5. Гейты

- [x] 5.1 `make ci-check`, `make e2e`, `make examples`, `make race` (или GitHub `race.yml`), `openspec validate document-result-intake-and-package-contracts --strict`, `git diff --check`; убедиться, что `openspec/changes/archive/` и опубликованные bundles не изменились. (сделано: `ci-check`, `e2e`, `examples` и полный локальный `race` зелёные — `internal/runtime` 1013 s без гонок; `openspec validate --strict` проходит, `git diff --check` чист, архив не изменён; `internal/flow/protocol.schema.json` и generated bundles байт-в-байт прежние, изменена только копия authoring-схемы решений вместе с распространяемой)
- [x] 5.2 Пройти на dev-сборке сценарий пилота: принять результат шага и прочитать его вердикт обычным `run status`; запросить schema обязательного выхода по её declared ID; прочитать одну форму через `--def`. Записать exact ответы в этот файл. (сделано на живом прогоне `aif-classic`):
  - текстовый `run status` печатает `step "…" status=completed verdict=pass outputs=1` для каждого принятого шага, рядом с `run_outputs=0 step_outputs=2`;
  - `package inspect --component aif:schema/implementation` отдал схему обязательного выхода с её `required: [base_commit, head_commit, changed_files]`;
  - `schema SessionSubmissionV5 --def runtime_SessionSubmission` — 4008 байт против 222694 у полного bundle;
  - `schema aif:schema/implementation` отказывает и называет команду чтения package;
  - каталог с `required: true` у runtime-решения не компилируется, поставляемый `aif-classic` компилируется как прежде.
