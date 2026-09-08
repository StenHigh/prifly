# Указатель по симптомам

[Справочники `authoring/`](README.md#справочники-yaml-для-авторов-сценариев)
проиндексированы **по полю**: чтобы найти ответ, нужно заранее знать имя поля.
Здесь тот же материал проиндексирован **по тому, что видно** — по отказу,
диагностике или поведению прогона. Каждая запись называет причину и точное поле
или флаг, а затем ведёт туда, где оно описано полностью.

Это не список всех отказов и не вторая спецификация: нормативные правила
остаются в [OpenSpec](../openspec/specs/). Сюда попадает только то, на чём
действительно спотыкались.

Строки отказов приведены дословно, чтобы их можно было искать по выводу CLI.

## Прогон

### `ambiguous_branch`, StageActivation failed, переход в `on_error`, ни одной Attempt

**Причина.** `choice` с `selection: exclusive` вычисляет **все** ветви и
отказывает, если истинны две — даже когда обе ведут в один и тот же следующий
stage.

**Поле.** `selection: first_match` в этом же stage. Оно берёт первую истинную
ветвь в закреплённом порядке и остальные не вычисляет; в trace они остаются
`not_evaluated`.

`selection` обязателен, и допустимых значений ровно два. Отрицать предикат
предыдущей ветви в каждой следующей не нужно: перекрытие ветвей — это то, что
`first_match` и разрешает. Полная форма stage —
[`authoring/workflow-authoring-reference.yaml`](authoring/workflow-authoring-reference.yaml),
раздел `choose`.

Отдельно: `unknown` **раньше** первой истинной ветви ведёт в `on_unknown`, а не
в неё. Ветвь после первой истинной не вычисляется и на решение не влияет.

### Assisted шаг останавливается примерно через час

**Причина.** `session_limits.active_timeout_ms` — конечное время разрешённой
работы host, по умолчанию `3600000` (один час). У шага с маркером
`authoring: prifly-step/2` умолчание вписывает компилятор, даже если блока
`session_limits` в YAML нет вовсе; оно запечатывается в определении. Ответ на
вопрос и restart не дают новый полный час, и это не измерение CPU чужого
процесса.

**Поле.** `active_timeout_ms` в шаге, целое `1..9223372036854`. Формы «без
ограничения» у него нет — она есть только у соседнего
`decision_wait_timeout_ms`, где `null` означает неограниченное ожидание одного
ответа. `0` не означает бесконечность и отвергается.

Каждый шаг задаёт свои значения; это не лимит на весь прогон. Правка YAML
создаёт новую редакцию, а не новое время уже идущему Run. Полная форма —
[`authoring/step-authoring-reference.yaml`](authoring/step-authoring-reference.yaml),
блок `session_limits`.

### Исполнитель сообщил объявленный output port и получил отказ

**Причина.** Порт, связанный в `workspace_trees`, — исключение из общего
правила. Обычный порт заполняет тот, кто назван в `outputs`: он пишет байты в
слот из своего context manifest и сообщает порт с `artifact_id`, revision и
digest. Дерево вместо этого захватывает и запечатывает сам runtime, и
исполнитель, сообщивший такой порт, отвергается.

**Поле.** `workspace_trees[].output_port` и `capture.kind` (`exact_file`,
`direct_child_file`, `direct_child_tree`).

Runtime захватывает объявленные output trees при каждой отправке шага с такими
bindings — независимо от того, есть ли `workspace_trees` в самой отправке.
Для policy с единственным допустимым значением (`exact_file`) не называть
location — **не** отказ: runtime берёт объявленный path. Отказ вызывает путь,
отличный от объявленного input location. Шаг с деревом обязан оставить
`result_check_refs` пустым.

Форма binding — в том же
[`authoring/step-authoring-reference.yaml`](authoring/step-authoring-reference.yaml);
обязанности стороны host — в
[cli-protocol](../openspec/specs/cli-protocol/spec.md).

### Заход ушёл в `uncertain`, и ни одно предложенное действие его не двигает

**Причина.** Неразрешённое исполнение удерживает слот, и слепой повтор
запрещён: `run drive` отвечает `recovery_required`, а `session disconnect`,
который этот отказ раньше называл, для срочной доставки сам отвергается.
Заход двигает только владелец, сказавший, что произошло.

**Команда.** `run resolve --run RUN --attempt ATTEMPT --outcome applied|not_applied
--reason TEXT`, затем `run drive`.

Подсказки в `run next` и в отказе `recovery_required` называют `run.resolve` с
версии 0.12.9. Если видите там только `doctor` — сборка старше.

### Отменил заход, а он завершился `failed`, не `cancelled`

**Причина.** Это не сбой отмены. Разрешение `not_applied` — свидетельство
«неизвестно, применилось ли», и оно не есть отмена: слияние двух исходов
спрятало бы худшую находку под лучшей. Статус захода берётся из корневого
вызова, поэтому исход зависит от того, на каком вызове висела разрешаемая
попытка.

**Поле.** `run.root_workflow_invocation_id` в выводе `run status` — по нему
видно, какой вызов определил исход. Диагностика `resolved_not_applied` с
версии 0.13.0 называет эту разницу прямо.

### `claim_conflict`, хотя рабочая копия своя

**Причина.** До 0.13.0 конфликт определялся репозиторием, и линкованные
worktree делят `--git-common-dir`, поэтому отказ приходил на **другое** дерево.
С 0.13.0 конфликт определяется занимаемым рабочим деревом: два worktree одного
репозитория идут рядом, два `checkout` — нет.

**Команда.** `claim list` называет держателя; `claim release --id CLAIM
--generation N` заканчивает его. Проверить, одно ли дерево у двух путей:
`git rev-parse --git-common-dir` в обоих.

### `capacity_conflict`: второй заход не стартует, хотя рабочие копии разные

**Причина.** Заходы одного репозитория идут рядом с 0.13.0, но авторитет по
умолчанию допускает **одну** попытку за раз. То есть параллельная работа
открыта правилом клеймов и закрыта числом допусков — это «не настроено», а не
«не поддерживается».

**Команда.** `capacity show` называет занятое; `capacity set --capacity N
--reason TEXT` поднимает предел.

**Одна попытка — не один прогон.** Предел считает попытки, а не заходы: стадия
с `kind: parallel` занимает слот на ветку. Поставив `--capacity 2` ради «двух
задач», на пакете с параллельной стадией вы упрётесь снова. Считайте по худшей
одновременности маршрута, а не по числу задач.

### `claim_identity_conflict` после перезагрузки машины

**Причина.** До 0.13.0 идентичность каталога сверялась по номеру тома, а он
перенумеровывается при загрузке — защита осуждала тот самый каталог, который
сама создала, и репозиторий запирался: `claim release` шёл через ту же
проверку. С 0.13.0 решает инод.

**Что делать на старой сборке.** Обновиться. Отдельного обходного пути нет:
именно документированный выход и отвергался.

## `project extend`

### `project_extension_missing_step_ref` или `project_extension_unknown_step: X (known: Y)`

**Причина.** `--step-ref NAME=JSON` и `--step-source NAME=FILE` — **пара на
каждый вставляемый шаг**, а не два способа назвать одно и то же. Ссылка
вписывается в workflow; файл проверяется на отсутствие входов. Список известных
шагов строится **только из `--step-ref`**, поэтому одна половина пары говорит о
другой:

| Что передано | Отказ |
|---|---|
| только `--step-source` | `project_extension_missing_step_ref` — называет оба флага |
| `--step-ref` с другим `NAME` | `project_extension_unknown_step: qa (known: lint)` |
| только `--step-ref` | `project_extension_missing_step_source: qa` |

Времени двум командам стоила именно средняя строка: `known:` перечисляет
ссылки, поэтому шаг из правильно написанного `extend.yaml` выглядит в нём
неизвестным, и поиск уходит в поиск опечатки, которой нет.

**Флаги.** Оба, с одинаковым `NAME`. `NAME` — короткое имя из `step:` в
`extend.yaml`, а не полный `id:` внутри файла шага. То же правило у `workflow:`:
это последний сегмент `--workflow-id` (`example:workflow/cycle` → `cycle`),
иначе `project_extension_unknown_workflow: build (known: cycle)`.

`--step-ref` принимает `ImmutableRef` — объект `{id, version, digest}`. Digest
записывается в workflow как есть и **не** сверяется с байтами
`--step-source`: источник проверяется только на то, что это `StepDefinition`
без `inputs`. Проверенный пример целиком:

```sh
cat > workflow.yaml <<'YAML'
id: example:workflow/cycle
version: 1.0.0
definition:
  stages:
    build: {kind: repeat, on_complete: {succeeded: commit}}
    commit: {kind: finish, outcome: succeeded}
YAML

cat > extend.yaml <<'YAML'
extensions:
  - id: qa-after-build
    workflow: cycle
    between: {from: build, to: commit}
    step: qa
    on: {pass: commit}
    impossible_verdicts: [fail, needs_revision, no_work]
YAML

printf 'inputs: {}\n' > qa.yaml

prifly project extend \
  --workflow workflow.yaml --workflow-id example:workflow/cycle \
  --extensions extend.yaml --output compiled.json \
  --step-ref 'qa={"id":"example:step/qa","version":"1.0.0","digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}' \
  --step-source qa=qa.yaml --json
```

```json
{"extensions":1,"schema_version":"project-extension/1","workflow_id":"example:workflow/cycle"}
```

В `compiled.json` маршрут `build.on_complete.succeeded` теперь ведёт в `qa`, а
сам `qa` получает `kind: step`, `step_ref` из `--step-ref` и объявленный
`on`. Ни ссылка, ни источник при этом не запечатываются и не исполняются.

Полная форма `extend.yaml` —
[`authoring/extension-authoring-reference.yaml`](authoring/extension-authoring-reference.yaml).

### `project_extension_requires_full_yaml: a simple extension step cannot declare inputs`

**Причина.** Файл из `--step-source` объявляет `inputs`. Простая вставка их не
принимает: шагу, которому нужны входы, нужен собственный граф workflow, а не
вставка в чужой.

**Поле.** `inputs` в файле шага — должно отсутствовать или быть пустым.

### `project_extension_route_missing: A → B` или `project_extension_route_ambiguous: A → B`

**Причина.** `between: {from: A, to: B}` называет **маршрут**, а не точку после
stage. Вставка заменяет ровно один существующий прямой маршрут из `A` в `B`
(среди `on`, `on_complete`, `on_limit`, `on_error`, `on_unknown`, `default`);
`missing` — такого маршрута нет, `ambiguous` — их несколько.

**Поле.** `between` во вставке. Именно поэтому хвост графа не может молча
отвязаться: `from` сохраняет маршрут в новый шаг, новый шаг ведёт в `to`.

### После вставки скомпилированный workflow объявляет `schema_version: "4"`

**Причина.** `impossible_verdicts` на вставке поднимает до v4 **весь**
скомпилированный workflow, включая узлы исходного пакета. Измерено на примере
выше: с этим полем результат содержит `"schema_version": "4"`, без него — не
содержит поля вовсе.

Закрыть недостающие вердикты у узлов пакета может только его автор — проект,
расширяющий чужой пакет уровня v1–v3, сам этого сделать не может. Если вставке
объявлять невозможные вердикты не нужно, уберите `impossible_verdicts`: тогда
версия результата остаётся прежней. Если нужно — вердикты закрывает автор
пакета.
