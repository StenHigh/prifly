# Handoff для repository workflows

Кандидат технически проверен и принят владельцем по review ответом «Принимай».
Ручные UX-замеры не проводились; выбранный способ и согласованное изменение критериев — в [evidence](../evidence/owner-review.json).
Product patch принадлежит `StenHigh/prifly-aif-workflows`, а не установленному package Pri-Fly.
Author versions сохранены для сравнения формы. Версии и release нужно менять отдельно после приёмки.

## Входы

- Source: `https://github.com/StenHigh/prifly-aif-workflows.git`.
- Ref: `v1.53.0`; commit: `c85306050df060dc4b381ba7512173806250b3a6`.
- Package: `aif-classic` 1.47.0.
- Binary: Pri-Fly 0.13.81; SHA-256 указан в [acceptance](../acceptance.md).
- Skills и project settings: hashes в [baseline manifest](../evidence/baseline-manifest.json).
- Product patch: [aif-classic-readability.patch](aif-classic-readability.patch), ровно семь authoring/documentation файлов.
- Дополнительные probes передаются отдельными patches в `tests/probes/` внешнего repository.
  Они не являются tests или runtime Pri-Fly.

## Подготовка

`HANDOFF` должен указывать на каталог этого файла. `SKILLS` и `LOCAL_EXTEND`
должны указывать на сохранённые входы из manifest.
В выполненном эксперименте использовались `.agents/skills` и project `extend.yaml` Pri-Fly.
Temp authority должна находиться вне любого Git repository.

```sh
export PRIFLY_BIN=/Users/sh/.local/bin/prifly
export HANDOFF=/absolute/path/to/improve-workflow-authoring-readability/handoff
export SKILLS=/absolute/path/to/Pri-Fly/.agents/skills
export LOCAL_EXTEND=/absolute/path/to/Pri-Fly/.prifly/workflows/aif-classic/extend.yaml
export STAND=$(mktemp -d /private/tmp/readability-handoff.XXXXXX)
git clone --branch v1.53.0 --depth 1 https://github.com/StenHigh/prifly-aif-workflows.git "$STAND/baseline"
git -C "$STAND/baseline" rev-parse HEAD
cp -R "$STAND/baseline" "$STAND/candidate"
git -C "$STAND/candidate" apply --check "$HANDOFF/aif-classic-readability.patch"
git -C "$STAND/candidate" apply "$HANDOFF/aif-classic-readability.patch"
git -C "$STAND/baseline" apply "$HANDOFF/upstream-build-probe.patch"
git -C "$STAND/baseline" apply "$HANDOFF/upstream-route-probe.patch"
```

Сверьте HEAD с закреплённым commit до проверок. Не заменяйте ref на текущий `main`.
Для этих команд нужны Python с PyYAML и исходные host skills; платные модели не нужны.
При отличающихся skills нельзя ожидать прежние build keys из evidence.

## Compile и compatibility

```sh
(cd "$STAND/baseline" && TMPDIR=/private/tmp python3 tests/verify.py --binary "$PRIFLY_BIN")
(cd "$STAND/candidate" && TMPDIR=/private/tmp python3 tests/verify.py --binary "$PRIFLY_BIN")
(cd "$STAND/baseline" && TMPDIR=/private/tmp python3 tests/compatibility.py --binary "$PRIFLY_BIN")
(cd "$STAND/candidate" && TMPDIR=/private/tmp python3 tests/compatibility.py --binary "$PRIFLY_BIN")
TMPDIR=/private/tmp python3 "$STAND/baseline/tests/probes/readability_builds.py" \
  --binary "$PRIFLY_BIN" --baseline "$STAND/baseline" --candidate "$STAND/candidate" \
  --skills "$SKILLS" --local-extend "$LOCAL_EXTEND" --output "$STAND/builds.json"
```

Последняя команда проверяет 12 пар: `codex-cli`/`codex-app`, canonical/local settings,
`fast`/`full`/`ultra`. Она сравнивает полные identities и SHA-256 каждого sealed component.
Успех не зависит от нормализации refs или удаления полей.
Provenance source inventory может отличаться из-за нового `.md`; component bytes должны совпасть.

## Маршруты verify

```sh
TMPDIR=/private/tmp python3 "$STAND/baseline/tests/probes/readability_compare.py" \
  --binary "$PRIFLY_BIN" --baseline "$STAND/baseline" --candidate "$STAND/candidate" \
  --output "$STAND/routes.json"
```

Probe сначала проверяет owner-only и exhausted, затем остальные шесть случаев.
Он переиспользует upstream `verify`, `compatibility` и `run_check`.
Он создаёт реальные Runs и отвечает synthetic artifacts вместо вызова моделей.
Для `fail`/`no_work` он убирает ненужный gate output из template.
Шаблон upstream `run_check.submit` сам этот случай не обслуживает.

После каждой пробы код отменяет оставшиеся Runs, освобождает созданные claims
и удаляет monitor registration своей authority. Временный каталог удаляет `TemporaryDirectory`.
Обнаруженное прежнее поведение — [existing-verify-outcome-gap.md](existing-verify-outcome-gap.md).
Readability patch его не исправляет.

## Остальные выполненные команды

В Pri-Fly claim workspace выполнены:

```sh
TMPDIR=/private/tmp GOMAXPROCS=2 make test-changed GO=/opt/homebrew/opt/go/libexec/bin/go
TMPDIR=/private/tmp GOMAXPROCS=2 go test ./cmd/prifly -run '^TestCLIProjectContextSource' -count=1
openspec validate improve-workflow-authoring-readability --strict
git diff --check
```

CSV проверен одноразовой CLI-пробой по `examples/workflows/csv-report/README.md`.
Во второй копии изменены только два mappings `stages.parse` из [preview](../previews/csv-report.md).
Команды: `project init`, `project local set --allow-executable`, `project compile`,
`project start --allow-execution`, `run drive`, `artifact export`.
Обе формы дали sample bytes из [CSV evidence](../evidence/csv-control.json).
Отдельный CSV harness или новые CSV tests не передаются.

Перед checkpoint-коммитом нужен staged gate:

```sh
TMPDIR=/private/tmp GOMAXPROCS=2 make ci-check-once \
  GO=/opt/homebrew/opt/go/libexec/bin/go CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1
```

Полный race/e2e, version-bump audit и upstream release gates относятся к последующей поставке.
Handoff не разрешает upstream commit, release или установку кандидата в Pri-Fly.


## Первоначальный блокер локального коммита

Полные ворота упали в существующем concurrent worktree test.
Тот же тест упал 2/10 раз на чистом baseline и 1/10 на текущем claim.
[Evidence](../evidence/worktree-gate-failure.json) содержит точные параметры.
Readability-specific проверки выше прошли. Это не делает весь `ci-check` зелёным.
Checkpoint-коммит не создан; исправление core/CLI не входит в этот product patch.

Последующий package fix устранил rename race. Полные локальные ворота прошли; [результаты](../../fix-concurrent-named-worktree-start/acceptance.md).
На том этапе человеческая приёмка handoff оставалась открытой. Теперь владелец принял его по review.


## Проверка после исправлений concurrency

Текущий исходный CLI повторил 12 пар сборок и восемь пар маршрутов: PASS.
Семь файлов product patch сохранили прежние hashes. [Evidence](../evidence/current-runtime.json).
Worktree fix завершил локальный пункт 3.1 и полный staged gate. Человеческая приёмка завершена по review; измеренного ускорения не заявлено.

Изменения находятся в ветке `prifly/92f0f7c1e833a0f97c8fd2afb4163989b787ae36acb2eb5d3f072f5f42af915b`.
Владелец выполняет перенос в main и push после принятия результата. В этой сессии они не выполнены.
