## Why

Обзор для владельца — [review.html](review.html). Это плановый снимок;
актуальные решения и прогресс хранятся в design и tasks этого change.

Автору трудно найти маршрут, инструкции и правила остановки среди плотных YAML
mappings и переходов между файлами. Первый срез улучшает `aif-classic`;
существующий `csv-report` служит короткой контрольной проверкой простой формы.

## What Changes

- Добавить к README `aif-classic` краткую карту маршрута и таблицу
  «что хочу изменить → какой файл открыть». Карта не становится вторым графом.
- Проверить на копии `aif-classic` ту же блочную запись и вынос одного bridge
  из `text: |` в Markdown через существующий `context.source`.
  Конкретный первый участок: verify, verify-once и verify-batch.
- Сохранить exact bytes инструкций, маршруты отказа, порядок `first_match`,
  передачу исправленной implementation, capture и настройки повторов.
- Сравнить исходную и предложенную формы `aif-classic` по четырём заданиям читателя.
  Зафиксировать время, ошибки, подсказки и итог отдельным текущим evidence.
- Проверить на временной копии `csv-report` один переход до/после и прежний
  sample output. Исходные CSV-файлы и README не менять; отдельный UX-пилот
  и новая матрица CSV-тестов не требуются.
- Подготовить перенос принятого варианта `aif-classic` в его собственный
  repository. Не править установленную product folder как источник пакета.

Здесь меняется оформление исходников и документация существующих возможностей.
Runtime, compiler contract, versioned markers, CLI API и схемы не меняются.
Новый Markdown-формат шага с frontmatter в этот срез не входит.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

Нет. `skip_specs: true`: проверяем существующую форму, а не вводим новое
поведение. Нормативная ownership ни одной capability не меняется.
Источники ограничений: `workflow-and-context`, `aif-classic-workflow`,
`release-documentation-layout` и `delivery-roadmap` из `openspec/specs/`.
Карта источников — `openspec/SOURCE-OF-TRUTH.md`.

## Impact

В Pri-Fly затрагиваются плановые артефакты, отчёт приёмки и нейтральная проверка
`context.source` только при обнаруженном пробеле покрытия.
`examples/workflows/csv-report/` используется как неизменяемый материал проверки.
Внешний product package принадлежит `StenHigh/prifly-aif-workflows`.
Здесь находятся только план, выбранные фрагменты для review и результаты
сравнения; полного дубликата пакета в `examples/` не будет.

`refresh-author-guides-and-examples` сохраняет обновление общего индекса,
справочников, progress/live output и resumable-примера. Этот change владеет
читаемостью `aif-classic` и не повторяет те задачи.

Существующие Runs, releases, public bundles и archives защищены от правок.
Изменение source closure может дать новые compiled identities и digests;
равенство поведения не означает равенство сборок. Прежние исходники остаются
поддержанными. Обнаруженный дефект compiler или потребность в новом синтаксисе
получит отдельный capability change.

Основное предложение — [AI Factory](previews/aif-classic.md).
[CSV](previews/csv-report.md) описывает только контрольную пробу.
Порядок работ — [tasks.md](tasks.md).
