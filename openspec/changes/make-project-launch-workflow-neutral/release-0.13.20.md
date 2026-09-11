# Выпуск Pri-Fly 0.13.20

Три починки после 0.13.19: две родились из одного красного CI перед его тегом,
третья — из первого отчёта пилота на нём. Ни одна не меняет формат Run'а.

## Занятое хранилище — это ожидание, не потерянная запись

`ProblemFor` относил ошибку SQLite «database is locked» к
`persistence_unavailable`: exit 6, `retryable: false`, «не считай, что операция
закоммичена». Так отвечала любая команда, кроме `observe` (у неё свой
`retryOnBusy`), когда другой процесс держал authority дольше busy bound (3 с)
— то есть за запись, которая **не начиналась**. Механизм, изготавливающий
ложное убеждение из ожидания: тот же класс, что очередь на #111.

Стало: `storage_busy`, `retryable: true`, exit 5, свои слова:

```
storage_busy | Another process held the authority for longer than the busy bound;
  nothing was written. Retry.
```

Тест держит транзакцию одним хранилищем и ждёт другим с 1 мс bound; разрез
возвращает ровно старый ответ с «Do not assume the operation committed».

## `project runners update` в clone с одним живым host'ом

Профиль пилота объявляет `codex-cli` и `claude-code`, в репозитории лежит
только `.claude/skills/prifly-run`. `update` отказывал `project_runner_missing`
за отсутствующий codex-файл и не обновлял присутствующий; `add --host` для
объявленного host'а отвечал `conflict`. Обновить runner до текста 0.13.19 было
нечем.

Стало: отсутствующий runner объявленного host'а **перечисляется**, не
блокирует:

```json
{"schema_version":"project-runners-update/1","updated_hosts":["claude-code"],"missing_hosts":["codex-cli"]}
```

Для него ничего не создаётся. `project_runner_missing` остаётся у профиля, в
котором нет ни одного runner. `--host` у `update` не появился: общий профиль
объявляет host'ы один раз, повторять это руками в каждом clone — не выход.

Не закрыто рядом: положить runner для уже объявленного host'а по-прежнему
нечем (`add` — `missing`/`conflict`, `update` не создаёт). Выход руками:
убрать строку host'а из `.prifly/project.yaml` и `runners add --host NAME`.

## Тест гонки за слот ждёт busy, как драйвер

`TestConcurrentAdmissionsNeverExceedTheSlot` покраснел на CI перед тегом
0.13.19: 5 из 8 гонщиков — «database is locked» за 0.76 с. Восемь
сериализованных коммитов с fsync не уложились в 500 мс busy bound
**фикстуры**, пока `internal/runtime` (126 с) молотил тот же диск. Допущение
«< 70 мс на Apply» — не свойство продукта. Гонщик теперь ждёт busy и считает
только решение; разрез при 1 мс bound: с ожиданием 20/20 зелёных, без —
сообщение CI.

## Ворота

`make check` (race), `make e2e`, CI `verify`; каждая починка вырезана и
показала свой старый ответ.
