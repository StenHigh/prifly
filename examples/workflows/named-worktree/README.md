# Читаемое имя рабочей копии

Минимальная версия Project profile — `/4`. Runtime сохраняет служебный claim ID,
а каталог и ветка получают основу `smsplace-trucker-default-123`, затем `-02`,
`-03` при занятых именах. Проект сам объявляет JSON input `task`: Core не ищет
tracker и не выводит идентификатор из текста задачи.

В чистом Git repository после `prifly project init --host codex-cli` скопируйте
эту папку в `.prifly/workflows/named`, а её `project.yaml` — в `.prifly/project.yaml`.
Игнорируемый `local.yaml` от init сохраняется. Закоммитьте объявления перед запуском.

```sh
prifly project questionnaire --prepare --repository . --launch named --host codex-cli \
  --input task=.prifly/workflows/named/task.json
# Прочитайте summary; используйте его review_digest в следующей команде.
prifly project start --repository . --launch named --host codex-cli \
  --input task=.prifly/workflows/named/task.json \
  --expected-launch-digest DIGEST --command-id task:default-123:first
```

Для уже импортированного artifact вместо `--input` применяется
`--input-ref task=ARTIFACT_REF.json` с теми же JSON bytes и declared schema.
`pointer: ""` выбирает целое JSON string value; `/source` и `/id` выбирают поля.
Константа задаётся `{value: smsplace}`, без input. Значения — строки до 64 ASCII
символов, lowercase letters/digits с одиночными hyphen separators. Без silent
slugification: uppercase, underscore, Unicode, traversal и Git syntax отвергаются.
Template до 256 символов содержит literals `[a-z0-9-]` и `{name}`, до 16 bindings;
получившаяся основа до 96 bytes. Все placeholders связаны, extra bindings запрещены.

`project-launch-summary/5` показывает policy `first-free/1`, основу, значения,
authority root и `worktree_name_preview` с `reserved: false`. Preview не резервирует
имя: start может выбрать следующий свободный suffix до `-9999`. Digest закрепляет
policy/входы, а не наблюдаемую доступность. `project-start/5` сообщает фактические
`workspace.path`, `workspace.branch` и абсолютный `workspace_path`.

Повтор **того же** command ID возвращает сохранённый результат; новый независимый
запуск использует другой ID. Продолжение/recovery с передачей существующего claim
сохраняет каталог/ветку даже после удаления naming policy. Checkout и workflows
без Workspace не требуют naming inputs. Shared profile `/2`/`/3` не мигрирует сам;
без блока действует прежний hash naming. Claims используют `authority-claims/4`,
`claim list` — `foundation-claims/3`; старый binary не открывает новый claim contract.

Host получает обычное session задание и фактический `repository_workspace`.
Передавать Run настоящему AI host для проверки naming не требуется: CLI fixtures
проверяют контракт синтетическими session reports.
