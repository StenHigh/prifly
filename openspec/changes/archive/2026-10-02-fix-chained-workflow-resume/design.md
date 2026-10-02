## Context

Мотивация и scope — в `proposal.md`; требования — в `specs/`. На HEAD `ac8d42b` `recoveryTrace` читает только `source.Activations`. Первый resume сохраняет префикс в `Recovery.Reused`, копии root outputs — в `Recovery.RootOutputs`, а исполнение нового Run начинает с frontier. `bindingRefsForBody` уже читает эти outputs; повторный planner их теряет.

Run B SMSPlace имеет 13 перенесённых записей и outputs warmup/plan/improve/implement; его собственные root activations — verify и finish. Installed 0.13.74 отказывает `resume_prefix_empty`. Независимый диагностический CLI fixture на временном проекте, без ИИ, воспроизвёл A(partial) → B(partial) → второй prepare с тем же отказом. Диагностический overlay находился в `/private/tmp`; постоянный regression test должен появиться в репозитории.

## Goals / Non-Goals

**Goals:** использовать существующий общий recovery planner и публичные prepare/start; доказать композицию prefix и provenance на реальных Runs; сохранить существующие purity, compatibility и ownership guards.

**Non-Goals:** новый workflow engine, новые authoring поля, автоматическое решение baseline findings, обновление SMSPlace packages, миграция SQLite и автоматическое выполнение бизнес-пилота при тестировании фикса.

## Decisions

### 1. Восстановить доказанный путь, не создавать фиктивные activations

Planner разрешает `Recovery.SourceRunID/SourceRunVersion` и унаследованные identities в immutable evidence предков. Обход ancestry конечный, с проверкой циклов, missing/unsupported evidence и происхождения каждой записи; кэшировать загруженный ancestor внутри одного prepare, не добавлять постоянный cache/service.

Составить логический путь: валидный унаследованный префикс, затем собственные принятые стадии source до cutoff. Порядок стадий и nested ancestry доказываются в исходных Runs. `source_event_sequence` сохраняет прежний локальный смысл; не сортировать объединённый путь по sequences разных Runs. Повторно сравнить effective contracts и остальные основания с target. Простое append `Recovery.Reused` без такой проверки и снятие `len(reused)==0` отвергнуты: они скрывают несовместимость и отсутствие inputs.

Root outputs нового плана должны брать актуальные carried refs непосредственного source и его собственные принятые outputs, сверенные с исходными bytes/provenance. Это сохраняет доступные bindings после нескольких переносов; нельзя заменить refs только первыми refs предка или потерять ранние outputs, когда source прошёл дальше frontier. Стадии на/после выбранной точки, включая её nested subtree, исключаются; вновь исполненная стадия заменяет прежнее перенесённое основание в логическом пути.

### 2. Единая граница для default frontier и `--from-stage`

Default frontier определяется существующим routing source. Более ранняя стадия выбирается из доказанного root пути, включая inherited root stages. Определять root принадлежность по исходной Invocation и связи переноса, не по сравнению ID с новым RootInvocationID и не по имени стадии. Nested fix не становится доступной root точкой. Выбор entry оставляет действительно пустой префикс и прежний отказ.

### 3. Передавать дерево только от непосредственного source

Существующие prepare/start и transfer claim переиспользуются. Evidence читается у предков; claim берётся только у B, затем C. При отсутствии claim не искать дерево у A. Никаких reset/checkout по SubjectCommit артефакта, очисток или пересоздания дерева. Dirty tracked/untracked bytes не меняются от передачи. Start снова проверяет reviewed plan и source/claim generation; конкурентные запуски не получают один claim.

Все чтения evidence выполняются до write transform; в транзакцию передаются вычисленные данные и необходимые pins. Сохраняются atomic transfer/create и command dedup. Новая branch/worktree для фикса создаётся стандартным `prifly-run` → `aif-classic`, отдельно от SMSPlace claim.

### 4. Сначала использовать имеющуюся provenance, затем проверить необходимость edition

Source ancestry и глобальные source identities уже записаны. Сначала использовать их для проверки/публичной навигации без новых wire-полей. Прежние RecoveryReuse fields не переинтерпретируются; source event sequence остаётся локальным. Если публичный план не позволяет однозначно показать origin при смешанном prefix, добавить явную origin reference в новой edition и покрыть старые readers/bundles тестами. Не вписывать поле в замороженные схемы. `recovery/1` и `recovery/2` не переписываются; неполное historical evidence отказывает.

### 5. Исправить объяснение, сохранив индекс объявлений

`run next.continuations` остаётся индексом declarations, а не полным planner/admission. Guidance объясняет эту границу и требует конкретный prepare с явным repository. Ошибка на inherited evidence сообщает его недостаток/несовместимость, а не «ничего не было принято». В справочнике существующих `resumable`/`--from-stage` показать повторный перенос, минимальную исправленную release revision и ограничения. Новую capability identity ради bug fix не вводить; таблицу `examples/README.md` сверить с `prifly capabilities`.

## Risks / Trade-offs

- Недоступный ancestor или artifact → точный отказ до мутаций; не доверять одной сохранённой карте outputs вместо исходной приёмки.
- Несравнимые EventSequence и повторные имена nested stages → порядок в каждом Run, проверка root/nested ancestry и regression с call/repeat.
- Source прошёл дальше либо оператор выбрал раннюю inherited stage → проверка отсечения, отсутствия дублей и наличия bindings после исполнения frontier.
- Рост ancestry → конечный обход без искусственного лимита «два resume», один read каждого ancestor за prepare; operational bounds authority сохраняются.
- Изменение общей recovery логики → проверить и `project recover` от ранее восстановленного technically failed Run; не закрывать соседний change этим тестом.
- Потеря dirty work на transfer → fixture с tracked/untracked bytes и сохранением hashes до prepare и сразу после create/transfer, до первой workspace-write Attempt.

## Migration Plan

1. Добавить постоянное воспроизведение, исправить planner, пройти targeted checks и compatibility. Старые Runs не мигрировать.
2. Выпустить бинарник стандартными verify/qualify gates; номер фиксирующего релиза записать по факту, не заранее.
3. Владелец обновляет установленный бинарник. Агент SMSPlace выполняет инструкции в дополненном отчёте: read-only prepare от B, review, затем start при проверенном разрешении.
4. Сохранить инженерное и пилотное evidence отдельно. При любом новом отказе не менять authority/tree и не создавать обходной Run. Откат бинарника не откатывает Runs; старый reader, не поддерживающий новую edition, обязан отказать явно.
