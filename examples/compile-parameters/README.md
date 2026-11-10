# Проектный предел описания до prepare/start

Пример требует Core `0.13.90+`, capability `project_compile_parameters` и Project profile `/3`.
Он использует синтетический task и assisted worker с `effects.class: none`.
Worker проверяет передачу task; пример не требует модели или внешних программ workflow.
Подробности полей находятся в [руководстве](../authoring/compile-parameters-guide.md).

```text
project.yaml -> package/compile-parameters.yaml + package/project/compile-values.yaml
  -> schemas/task.yaml -> root -> child -> project/steps/worker.yaml
```

Пакет объявляет default `12000`; проект выбирает `32000`.
Все три task-порта используют alias `schema_task`.
Файлы `project/steps/worker.yaml` и `project/compile-values.yaml` принадлежат проекту.

## Подготовить отдельный проект

Команды выполняются из корня checkout Pri-Fly и пишут только в новый временный каталог.
Укажите абсолютный путь установленного binary.

```sh
PRIFLY_BIN=/absolute/path/to/prifly
TASK_LIMIT_EXAMPLE=$(mktemp -d)
mkdir "$TASK_LIMIT_EXAMPLE/repository"
"$PRIFLY_BIN" version --json
"$PRIFLY_BIN" capabilities --json
"$PRIFLY_BIN" project init --repository "$TASK_LIMIT_EXAMPLE/repository" \
  --state-root "$TASK_LIMIT_EXAMPLE/authority" --host codex-cli --json
mkdir -p "$TASK_LIMIT_EXAMPLE/repository/.prifly/workflows"
cp examples/compile-parameters/project.yaml "$TASK_LIMIT_EXAMPLE/repository/.prifly/project.yaml"
cp -R examples/compile-parameters/package "$TASK_LIMIT_EXAMPLE/repository/.prifly/workflows/limits"
python3 - "$TASK_LIMIT_EXAMPLE/task.json" <<'PY'
import json, sys
from pathlib import Path
description = "я😀" * 8278 + "я"
assert len(description) == 16557
Path(sys.argv[1]).write_text(json.dumps({"description": description}, ensure_ascii=False))
PY
```

## Проверить и запустить

1. Выполните prepare и прочитайте summary.
2. Проверьте выбранный launch `limits`, host, task, package ref и `build_key`.
3. Возьмите `review_digest` из этого summary и передайте его start.

```sh
"$PRIFLY_BIN" project questionnaire --prepare \
  --repository "$TASK_LIMIT_EXAMPLE/repository" --launch limits --host codex-cli \
  --input "task=$TASK_LIMIT_EXAMPLE/task.json" --json > "$TASK_LIMIT_EXAMPLE/review.json"
cat "$TASK_LIMIT_EXAMPLE/review.json"
```

После проверки summary выполните start с теми же inputs:

```sh
TASK_LIMIT_DIGEST=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["review_digest"])' "$TASK_LIMIT_EXAMPLE/review.json")
"$PRIFLY_BIN" project start \
  --repository "$TASK_LIMIT_EXAMPLE/repository" --launch limits --host codex-cli \
  --input "task=$TASK_LIMIT_EXAMPLE/task.json" \
  --expected-launch-digest "$TASK_LIMIT_DIGEST" --json
```

Start возвращает `run.run.id`.
Для передачи task worker выполните `run drive RUN_ID`, затем `session task --run RUN_ID` с `--project "$TASK_LIMIT_EXAMPLE/authority"`.
Полный task находится по пути `workspace` плюс `context.inputs.task.path` из задания сессии.
Шаг принимает отчёт `pass` без outputs.
Форму отчёта выдаёт `session submit --template --run RUN_ID --attempt ATTEMPT_ID`; attempt ID берётся из задания сессии.
После принятого отчёта `run drive RUN_ID` завершает Run с outcome `succeeded`.
[Протокол отчёта](../operations/hosting.md) объясняет обязательные поля и `answered_questions`.

## Проверить границы

В копии примера можно менять owner value и длину синтетического task.

| Значение проекта | Длина description | Результат prepare/start |
|---|---|---|
| Owner file отсутствует: default `12000` | `12000` / `12001` | Принимается / `schema_invalid`. |
| `32000` | `16557` / `32000` | Принимается с полным task. |
| `32000` | `32001` | `schema_invalid`: `/description`, предел `32000`, фактическая длина `32001`. |
| `1000001` | Любая | `project_compile_parameter_invalid` до Run. |
| Значение изменено после prepare | Допустимая | Старый digest получает `project_start_stale_launch`. |

После изменения owner file повторите prepare и review перед новым start.
Workflow update сохраняет owner file; прежний Run хранит свой контракт.

[Release evidence и повторяемая проверка](../../openspec/changes/archive/2026-11-10-add-project-owned-compile-parameters/evidence/released-v0.13.90/README.md)
содержат полный synthetic lifecycle, проверенный на опубликованном Linux binary.
