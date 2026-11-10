# Параметры сборки: предел описания task

Параметр сборки задаёт разрешённый пакетом предел до проверки входного task.
Минимальная версия Core — `0.13.90`.
Проверьте `version --json`, capability `project_compile_parameters` и Project profile `/3`–`/5`.
Project profile `/2` получает явный отказ.

[Полный пример](../compile-parameters/README.md) содержит root, child workflow и project-owned worker.
[Release evidence](../../openspec/changes/archive/2026-11-10-add-project-owned-compile-parameters/evidence/released-v0.13.90/README.md) связывает опубликованный binary с проверенным SHA.

```text
объявление пакета + значение проекта
  -> проверка integer/диапазона/binding -> общая схема root/child/worker
  -> prepare -> проверенный summary -> start -> закреплённый Run
```

## Файлы автора пакета

Пакет объявляет тип, диапазон, default и привязку в `compile-parameters.yaml`.
Привязка называет собственную схему через локальный alias и JSON Pointer её строкового `maxLength`.
В примере пакет объявляет default `12000` и диапазон `1..1000000`.

```yaml
authoring: prifly-compile-parameters/1
parameters:
  task_description_limit:
    type: integer
    minimum: 1
    maximum: 1000000
    default: 12000
    bindings:
      - schema: schema_task
        pointer: /properties/description/maxLength
```

Файл `schemas/task.yaml` содержит подстановку целого значения:

```yaml
id: example:schema/task
version: 1.0.0
type: object
required: [description]
additionalProperties: false
properties:
  description:
    type: string
    maxLength: "{{task_description_limit}}"
```

Root, child и worker ссылаются на эту схему через `schema_ref: "{{schema_task}}"`.
Compiler закрепляет согласованные refs всей цепочки.
Одинаковые имена параметров в разных source packages имеют независимую область действия.

## Файл проекта

Проект сохраняет свой выбор в `.prifly/workflows/NAME/project/compile-values.yaml`:

```yaml
authoring: prifly-compile-values/1
values:
  task_description_limit: 32000
```

Значение `32000` — число без кавычек.
Без имени в `values` или без owner file compiler использует default пакета.
Нужные поля показаны в справочниках [объявления](compile-parameters-authoring-reference.yaml) и [значения проекта](compile-values-authoring-reference.yaml).

## Prepare и start

1. Проверьте декларацию установленного пакета и допустимый диапазон.
2. Сохраните значение проекта в owner file.
3. Выполните `project questionnaire --prepare` с выбранным task и host.
4. Проверьте summary, package ref, `build_key` и `review_digest`.
5. Выполните `project start` с теми же inputs и `--expected-launch-digest` из summary.

Prepare читает значение и проверяет task без создания Run.
Изменение owner value меняет compiled identity и делает прежний review недействительным.
Start со старым digest получает `project_start_stale_launch` до создания Run.
Готовые команды находятся в [полном примере](../compile-parameters/README.md).

## Проверка длины и отказы

JSON Schema считает Unicode code points.
Кириллический символ и emoji дают по одному code point; combining marks считаются отдельно.
При пределе `32000` описание длиной `16557` или `32000` принимается.
Описание длиной `32001` получает `schema_invalid` с `/description`, пределом `32000` и фактической длиной `32001`.

| Код | Что проверить |
|---|---|
| `project_compile_parameter_invalid` | Integer, положительный диапазон, default и значение проекта. |
| `project_compile_profile_required` | Выберите поддержанный Project profile `/3`–`/5` для compiled variants. |
| `project_compile_parameter_unknown` | Имя параметра в owner file совпадает с объявленным именем. |
| `project_compile_parameter_binding` | Собственный schema alias, существующий строковый `maxLength` и совпадающая подстановка. |
| `project_start_stale_launch` | Повторите prepare после изменения значения или sources. |

Подстановка разрешена только для объявленного schema keyword.
Binding в `default`, `examples`, чужую схему или другое поле получает отказ.
Project references и package profiles не должны объявлять alias с именем этого параметра.

## Update и продолжение

Workflow update сохраняет `project/compile-values.yaml` вместе с каталогом `project/`.
После сужения диапазона следующая compilation отказывает несовместимому сохранённому значению.
Существующий Run сохраняет свои definitions и полный task.
Continue проверяет task по объявленному контракту продолжения и отказывает до эффектов при несовместимости.

`settings` меняет default configuration input.
Объявленный параметр сборки выбирается через owner file.
Флаг standalone `compile --value` сохраняет отдельный legacy contract; prepare/start его не принимают.

Consumer должен выпустить пакет с декларацией и согласованными task-портами.
Проверки classic/profiled в [workflows #3](https://github.com/StenHigh/prifly-aif-workflows/issues/3) имеют отдельную приёмку A1–A10.
