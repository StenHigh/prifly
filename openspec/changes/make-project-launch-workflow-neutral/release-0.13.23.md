# Выпуск Pri-Fly 0.13.23

Решение владельца через пилота: ответы на объявленные решения, которые он
принял для проекта, живут в дереве, а не в памяти оператора.

## Что было

Пять решений владельца — `--decision-policy autonomous`, `--preflight-answer
plan_tests=true`, `gate_warnings`, `gate_checks=<файл>`, `--runtime-answer
improve_apply=all` — жили в памяти оператора и в лаунчере фиксированными
флагами. Проектное место было только для одного из шести: `profile:` в
`.prifly/workflows/<package>/extend.yaml`. Мост improve «when the owner sealed
an answer for it before the Run started» говорил о `--runtime-answer` в
командной строке, не о профиле.

## Что стало

В том же `extend.yaml`, рядом с `profile`:

```yaml
answers:
  decision_policy: autonomous
  preflight:
    plan_tests: true
    gate_checks: |
      composer test -- --group=product
  runtime:
    improve_apply: all
```

- `project questionnaire` и `project start` читают их, когда флаг не назван;
  в листе решений источник `project_default` (уже в опубликованном enum,
  схемы состояния не менялись).
- Проверка — та же, что у флага: неизвестное решение, чужая фаза, неверное
  значение — отказ, и он называет место правки: `project_start_unknown_decision:
  nobody (from extend.yaml answers.preflight) is not declared…`, `plain is a
  runtime decision; pass it with extend.yaml answers.runtime, not extend.yaml
  answers.preflight`, `project_start_invalid_decision_answer: retry (from
  extend.yaml answers.runtime): …`.
- Флаг перебивает только тот ответ, который называет; остальные постоянные
  ответы остаются; `extend.yaml` не переписывается. `--decision-policy` без
  флага — `answers.decision_policy`, иначе `attended`, как прежде.
- Авторская схема `extension-v1` получила необязательное `answers` (аддитивно,
  версия та же); справочник `examples/authoring/extension-authoring-reference.yaml`
  показывает блок; сценарий в `workflow-and-context`.

Текст раннера не менялся: анкета показывает постоянные ответы отвеченными с
источником, хост спрашивает только недостающее — как и было велено.

Разрезы: без слияния — «the standing preflight answer was not sealed as the
project's: "true" "autonomous_policy"»; без чтения политики — `attended` при
объявленном `autonomous`.

`origin.extend_digest` в `project.yaml` — отпечаток upstream-файла на момент
установки, не локального: после правки `answers` он не меняется, и
`--prepare` на это не жалуется; «чинить» его не нужно.

## Для пилота

Лаунчер ужимается до двух проверок базы; пять флагов переезжают в
`extend.yaml` пакета `aif-classic` в вашем репозитории, `gate_checks` — блочным
скаляром. Первый заход после переезда: анкета должна показать их источником
`project_default`.

## Ворота

`make check` (race), `make e2e`, CI `verify`.
