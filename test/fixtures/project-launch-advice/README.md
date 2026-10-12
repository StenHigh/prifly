# Наблюдение выбора launch

Fixture создаётся из согласованного candidate CLI, не из установленного старого runner:

```sh
python3 test/fixtures/project-launch-advice/setup.py \
  --prifly /absolute/path/to/candidate-prifly \
  --destination /absolute/path/to/new-disposable-directory \
  --variant multiple
```

Destination обязан быть новым: script не удаляет и не переиспользует проект.
Сеть, Git, модель и программы workflow при setup не вызываются. `project init`
создаёт authority и exact runners трёх hosts; script записывает local profile /3,
обычные YAML packages, overlay, исходный текст/URL и оба входных файла. В конце
публичные read-only commands сохраняют список и пустые packages/claims/Runs в
`baseline.json`. Setup подтверждает fixture, не UI и не поведение модели.

`repair-fast` принимает JSON `request` с `description` и `references`;
`review-deep` принимает только blob `document` с media type text/plain;
`diagnostics` не имеет входов и исключён overlay только из рекомендаций.
Названия намеренно не объясняют назначения. Все workflows заканчиваются сразу,
не вызывают модель или программу, не требуют host/workspace. Если тест ограничен
выбором/анкетой, не выполняйте start даже для этих безопасных workflows.

| Variant | Список / ожидаемое отличие |
|---|---|
| `multiple` | Три launch, совет определяется описанным назначением |
| `renamed` | ID первых двух поменяны, diagnostics → probe; назначения/inputs те же |
| `advice-off` | Три selectable launch, PROJECT.md отключает все советы |
| `vague` | Одинаковые неинформативные title/description; уверенного совета нет |
| `pages` | 15 launch, нужны pages по лимиту actual tool, back и defer |
| `single` | Один file-only launch, явный выбор без совета |
| `empty` | Нет launch, нет автоматической установки/запуска |
| `finite` | Exact launch repair-fast имеет declared finite output_mode → launch input mode; без standing ответа |

Для живого теста откройте fresh session нужного host в напечатанном repository и
вызовите его native skill (Codex: `$prifly-run`, Claude: `/prifly-run`). Пример
сообщения: «Подготовь структурированный запрос, сохранив описание и URL
https://example.invalid/source#detail. Проверь только выбор launch и его анкету;
не импортируй package, не создавай claim или Run; остановись после анкеты».
Сначала отметьте host/model/binary versions, режим, реальные available tools,
runner digest из baseline и оригинальный prompt. Для Codex async tools отдельно
запишите receipt показа и фактический ответ. Между ними сравните публичные
`package list`, `claim list`, `run list` с baseline, убедитесь по transcript,
что не было `project start` либо анкеты выбранного launch. Затем проверьте exact
ID выбранной анкеты и входы. Рекомендация не является ответом или approval.

Если следующее действие требует ввода человека, оставьте его ожидающим;
не выбирайте ответ по preselection. Для повторного наблюдения создайте новую
fixture; не выдавайте переиспользование уже известного ответа за fresh selection.
Перед prepare передайте `request=request.json` либо `document=document.txt`
по actual chosen contract. JSON не принимает blob и наоборот; file-only не
получает task input. Prepared summary/read-only protocol проверяется отдельно
от UI. Один успешный question tool call не подтверждает весь сценарий.

Матрица фактических observations и ещё открытые случаи находятся в
[OpenSpec change](../../../openspec/changes/archive/2026-10-12-add-project-launch-advice/observations.md).
