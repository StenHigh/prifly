## Context

Мотивация и границы — [proposal.md](proposal.md). Baseline плана:
Pri-Fly `1ced124dea87631d0098fee390bf843dc472de30`, Go 1.27.3.
Установленный `aif-classic` объявляет package 1.40.0; origin в project profile
указывает commit `c2aa705fd31991f19aa79a90d44f396b6d498371` внешнего repository.
Это не доказательство отсутствия локальных правок: при реализации нужно
отдельно сверить folder digest, upstream и местный `extend.yaml`.

Нормативные ограничения уже заданы `workflow-and-context`: известные каталоги,
single-document YAML, явные routes и limits, exact bytes контекста и неизменная
история Runs. Product AI Factory package принадлежит внешнему repository.
Нормативная ownership и предметная модель не меняются; delta specs отсутствуют
по `skip_specs: true`.

`compileProjectComponent` в `cmd/prifly/project_compile.go` читает `source`
в raw bytes. Это основание для кандидата, но не заменяет его будущую CLI-пробу.
Политика файла относительно `.prifly/` и отказ на escape остаются прежними.

## Goals / Non-Goals

**Goals:**

- Показать проверяемое изменение `aif-classic` до массового переноса.
- Проверить простую форму на одном переходе существующего CSV-примера.
- Отделить читаемость YAML от читаемости длинного текста инструкций.
- Сохранить поведение, явные отказы и доступность прежней формы.
- Получить решение владельца по наблюдениям, а не по числу строк.

**Non-Goals:**

- Новый язык, Markdown step с frontmatter, anchors, шаблонизатор или автограф.
- Переименование IDs, ports, stage names, aliases либо физическая перегруппировка YAML.
- Перенос всех bridges, новых функций движка или изменение логики gates.
- Переоформление CSV package, его README, отдельные CSV UX-замеры или новые CSV tests.
- Вызовы платных моделей ради детерминированной регрессии, выпуск или автоперенос пакетов.

## Decisions

### 1. Один основной кандидат и короткая контрольная проба

| Сценарий | Файлы реализации | Изменение |
|---|---|---|
| AI Factory | `aif-classic/workflow.yaml` | Блочные contracts, bindings, routes, limits и profile values; тот же граф |
| AI Factory | `aif-classic/steps/verify.yaml` | Читаемые ports, executor и effects; прежний read-only capture |
| AI Factory | `aif-classic/workflows/{verify-once,verify-batch}.yaml` | Явные predicates, bindings, routes и пределы repeat |
| AI Factory | `aif-classic/contexts/aif-verify-bridge.yaml` | Четыре поля: id, version, media_type, source |
| AI Factory | `aif-classic/instructions/aif-verify-bridge.md` | Полные decoded bytes прежнего `text`, без редактирования инструкций |
| AI Factory | `aif-classic/README.md` | Обзор root и отдельная карта verify; ссылки на существующие компоненты |

AI Factory paths в таблице принадлежат внешнему repository или disposable копии,
а не `examples/` Pri-Fly. Списки `[pass, needs_revision]`, argv и простые значения
можно оставить короткими. Не разворачивать всё механически ради единообразия.
`schemas`, `decisions`, skills contexts и остальные steps не переписывать.
`extend.yaml` проекта сохранить: это настройки потребителя, а не новая версия
канонического package.

Альтернатива — группировка YAML по шагам — требует проверки discovery и alias
contracts. Она не нужна для первого измеримого результата.

### 2. Обзор показывает путь, файлы остаются исполняемой правдой

В README AI Factory показать root с явными ссылками на gates. Участок verify
объяснить отдельно: disabled пропускает implementation; enabled запускает
bounded repeat; owner-only не направляется в автоматический fix.
Root summary не выдавать за полный graph security/review/improve.
После каждой правки routes сравнивать карту с YAML. Графогенератор не вводить.

### 3. Markdown сохраняет текст, а не пересказывает его

Контекст после изменения приведён в [preview AI Factory](previews/aif-classic.md).
Значение `source` относительно корня Project остаётся явным:
`.prifly/workflows/aif-classic/instructions/aif-verify-bridge.md`.
`instructions/` не становится новым каталогом discoverable components.
Compiler читает файл только по `source`.

Файл получить из decoded YAML scalar, сохранив UTF-8, LF и финальный перевод
строки. Сравнить длину и SHA-256 bytes до/после, затем sealed context resource.
Не извлекать Markdown копированием визуального отступа; chomping YAML влияет
на bytes. Не сокращать инструкции и не переводить их в этой работе.
Проверить missing source и выход за `.prifly/` на нейтральном fixture;
существующее покрытие переиспользовать. Дополнить только обнаруженный пробел.

### 4. Сравнение definitions учитывает compiled identities

До оформления зафиксировать author IDs/versions и settings, host skills bytes,
external refs, schemas и executable files. Для YAML-only правок parsed values
должны совпасть. Для context сравнить три metadata-поля и decoded content bytes.

Обе формы скомпилировать CLI в отдельных disposable проектах и output folders.
Сравнить все owned definitions по mapping author identity → compiled reference.
Разрешённая нормализация: только compiler-owned versions/refs по этому mapping
и состав source provenance. Не удалять все `digest` либо строки по шаблону:
schema refs, external identities, data и context digests остаются проверяемыми.
После нормализации сравнить definitions целиком, а не только happy path.

Если identities изменились, отчёт объясняет причину и показывает обе стороны.
Не обещать их изменения заранее: конкретный build key определяется compiler.
Отдельный bump author version для upstream поставки не входит в сравнение
формы; это следующий этап внешнего repository.

CSV исполняется без Git и host. AI Factory compile требует выбранный declared
host и доступные skills; отсутствие host не превращается в successful compile.
Детерминированные ответы session executor проверяют маршруты без живого ИИ.
Такая симуляция не доказывает улучшение работы модели или живую host UI-приёмку.

### 5. Регрессия проверяет опасные различия

| Сценарий | Наблюдаемое совпадение baseline и candidate |
|---|---|
| Verify выключен | Implementation проходит без verify/fix |
| Verify чистый | Gate даёт `blocking: false`; выполняется следующий root route |
| Verify owner-only | При обоих blocking flags true первым выбирается owner; fix не вызывается |
| Verify исправим | Fix выдаёт новую implementation; следующий round и downstream читают её |
| Verify предел | При `verify_round_limit=1` исправление требует повторной проверки; путь завершается `partial` |
| Verify ошибки шага | `fail` и `no_work` сохраняют исходные terminal routes |
| Profiles/settings | fast/full/ultra и действующий project `extend.yaml` сохраняют выбранные values, capture и limits |

Повторить параметры принятого baseline; не изменять граф ради более удобного
теста. Не принимать заранее, что root после `no_work` завершится ожидаемо:
сравнить фактический baseline и раскрыть прежний дефект отдельно.
Owner-only и repeat проверяются прежде успешного сквозного маршрута.
Package-specific cases живут в тестах внешнего repository; нейтральный
`context.source` fixture — в Pri-Fly.

### 6. Читаемость имеет отдельный протокол приёмки

См. [readability-protocol.md](readability-protocol.md). Исполнитель записывает
время, правильность и число подсказок по четырём заданиям `aif-classic` для каждой формы.
Результат на одном владельце называется локальным пилотом, не общей UX-оценкой.

Приёмка: функциональная матрица `aif-classic` проходит; автор решает 4/4 задания
candidate; ошибок/подсказок не больше baseline; суммарное время меньше.
CSV проходит короткую контрольную пробу из следующего пункта.
Это предложенный критерий, не достигнутая метрика.
При равном времени или ухудшении карта/структура дорабатывается либо кандидат
отклоняется. Оценку с ошибкой таймера повторяют и помечают, а не скрывают.

### 7. CSV проверяет один переход без отдельного проекта

Взять существующий `csv-report` из зафиксированного Pri-Fly baseline.
В disposable копии раскрыть только `stages.parse.input_bindings` и `on`,
как показано в [контрольном preview](previews/csv-report.md).
Parsed value всего `workflow.yaml` должен остаться прежним.

Попросить читателя найти следующий шаг и отказ по этому фрагменту; записать
короткое наблюдение. Отдельные четыре задания, таблица времени и UX-gate
для CSV не нужны. Скомпилировать обе формы и проверить sample Run: прежние
`succeeded` и bytes `Pri-Fly CSV report\nRows: 3\nTotal: 24\n`.

Не менять tracked CSV YAML, steps, README, worker или schema. Новую матрицу
отказов и новые CSV tests не создавать. Существующее покрытие остаётся в общих
воротах; инженерная проверка кандидата выполняется в disposable копии.

### 8. Владение и завершение разделены

`refresh-author-guides-and-examples` владеет общей навигацией `examples/README.md`,
новыми руководствами, program progress/live output и resumable example.
Эта работа может идти без его завершения на зафиксированном CSV baseline.
CSV diff в Pri-Fly отсутствует: обновление этого примера остаётся у соседнего change.

Для AI Factory сначала сравнить upstream commit и установленный материал;
зафиксировать расхождения, не переносить local overrides в canonical folder.
Кандидат создаётся в disposable копии. Этот change выдаёт проверенный diff
для владельца repository workflows и точную команду проверки latest stable.
Коммит, version bump, release и установка upstream edition выполняются
отдельно в том repository. Текущий `.prifly/` Pri-Fly не переписывается.
Недоступность upstream проверки означает «handoff не проверен»; её нельзя
заменить compile только на development binary или закрытой задачей.

## Risks / Trade-offs

- [Markdown добавляет файл и переход] → проверить поиск и редактирование; при
  ухудшении оставить inline text и отклонить только этот кандидат.
- [Форматирование скрывает прежний дефект] → сравнить полный graph и baseline
  отказов; исправление поведения вывести в отдельный capability change.
- [Повторные задания дают эффект обучения] → поменять порядок A/B и формы;
  хранить результаты первого знакомства отдельно от повторов.
- [Нормализация скрывает реальную перемену] → разрешить только явный mapping
  owned refs; bytes, external refs, настройки и маршруты сравнивать полностью.
- [Старый roadmap status принят за факт] → фиксировать текущие commits,
  tasks и реально исполненные проверки; не закрывать P1/P2 этим отчётом.

## Migration Plan

1. Зафиксировать baseline и наблюдения читателя.
2. Подготовить AI Factory candidate, сравнить bytes и опасные маршруты.
3. Выполнить короткую CSV-пробу во временной копии.
4. Провести reader pilot и принять или отклонить каждое изменение отдельно.
5. Передать принятый AI Factory diff в repository workflows.

```text
Baseline → aif-classic candidate → проверка поведения → reader pilot → handoff
                                  + короткая CSV-проба  └─ хуже → доработка
```

Rollback возвращает authoring файлы; прежние package builds и Runs не
редактируются. Внешняя новая edition устанавливается только обычным update
с защитой локальных правок и сохранением origin. Релиз Pri-Fly этим change
не назначается.
