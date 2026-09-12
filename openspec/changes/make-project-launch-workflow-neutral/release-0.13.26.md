# Выпуск Pri-Fly 0.13.26

Три наблюдения пилота с первого сухого прогона `tests`-программы на 0.13.25
(MR !1140): один запрос и две правки текста.

## `references:` в `extend.yaml`

Было: `refs:` шага требуют digest, а взять его негде, кроме `prifly inventory
--json` (два ряда на id, один с `null`); логическая форма `id@version`, как в
`references:` пакета, в `refs:` шага не принимается; у шага проекта в
`project/` своих `references:` не было — отпечаток встроенного адаптера
вписывался руками.

Стало: `references: {process: core:adapter/local-process@2.0.0}` в
`extend.yaml`; разрешается из инвентаря на компиляции и на старте ровно как
пакетные, доступно как `{{process}}` в `project/steps` и соседях; имя,
которое пакет уже объявляет — отказ `project_extension_invalid:
references.<name> is already a reference of the package`, не тень. Разрез:
без слияния — `project_extension_unknown_step` (шаг с неразрешённым `{{…}}`
не читается).

## `prifly-step/2` для программы называет `prifly-step/1`

Было: `schema_invalid at /executor/adapter_ref/id … the declared value is
"core:adapter/assisted-session"` — читается как опечатка, а не как «другая
версия authoring»; час пилота ушёл сюда.

Стало: `prifly-step/2 describes an assisted session step; a program step
(operation: process) is written as authoring: prifly-step/1`, указатель
`/executor/operation`. Шапка `step-authoring-reference.yaml` говорит то же.

## Пути портов относительно scratch попытки

Программа, которая делает `cd "$PRIFLY_REPOSITORY_WORKSPACE"`, теряет
`outputs/test_run`. Так и задумано (cwd программы — scratch попытки), теперь
названо в csv-report README и `examples/README.md`: разрешайте пути портов от
каталога `PRIFLY_CONTEXT_FILE`.

## Не сделано, решение владельца

`merge-request` как программа — наружное действие (push + MR) из шага; пилот
спросит владельца после первого боевого прогона `tests`.

## Ворота

`make check` (race), `make e2e`, CI `verify`.
