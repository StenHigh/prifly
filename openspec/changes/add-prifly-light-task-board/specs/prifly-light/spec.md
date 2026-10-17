## Purpose

Определяет отдельный компактный read-only интерфейс Pri-Fly Light для локальных задач, связанных запусков и записанных результатов ревью, с достоверным происхождением данных и ограниченной стоимостью чтения.

## ADDED Requirements

### Requirement: Light имеет самостоятельный первый экран

Pri-Fly Light SHALL предоставлять отдельную компактную доску задач и приёмки, явно включаемую как отдельный mode/entrypoint. Default full monitor, его frontend/API, auto-start и обслуживание SHALL сохранять прежнее поведение. Другой domain/listener, page title, тема либо отсутствие filesystem scan MUST NOT сами считаться Light. Graph, timing, traces, storage diagnostics и raw payloads SHALL открываться по необходимости и MUST NOT доминировать или загружаться на первом экране.

Источник нормы новой capability — `openspec/specs/prifly-light/spec.md` после sync этого change; current source ownership остальных capabilities по `openspec/SOURCE-OF-TRUTH.md` сохраняется. До sync действуют delta/design этого change вместе с текущими source sets. Existing local monitor/queue и historical saved Run contracts не мигрируют ради Light.

#### Scenario: Два интерфейса одного набора данных
- **WHEN** full monitor и явно включённый Light открыты на одинаковых sources
- **THEN** Light показывает отдельный первый экран задач/ревью, full показывает прежний интерфейс; отсутствие scan-root не используется как доказательство отличия UI

### Requirement: Native task identity определяет одну карточку

Light SHALL читать native QueueTask, включая ready task до первого Run, и группировать associated Runs/continuations в одну карточку по exact authority/project/task identity. Канонические immutable Run bindings и durable launch intents SHALL учитываться при отставшем queue index; несогласованность SHALL быть видима как integrity error, без mutation/reconcile при чтении. Task revision и привязанная revision каждого Run SHALL различаться. Связанные Runs SHALL сохранять собственные identities, outcomes и доступную постраничную историю.

Runs без task binding SHALL получать явно помеченные standalone fallback-карточки. Одинаковые title, время, workflow namespace или похожий текст MUST NOT объединять работу. Recorded forks без task binding SHALL давать только ссылки. Missing source/association SHALL обозначать неполноту. Несколько frontier Runs MUST NOT заменяться единственным latest по timestamp.

#### Scenario: Очередь и цепочка continue
- **WHEN** task видна без Run, затем start заканчивается partial, continue отменён и создаётся ещё одно продолжение
- **THEN** остаётся одна task-карточка с recorded associations и неизменными partial/cancelled outcomes в истории

#### Scenario: Совпавшие названия и identities
- **WHEN** два независимых Runs имеют одинаковый title либо разные authorities содержат одинаковые task IDs
- **THEN** карточки сохраняют разные source/authority identities и не объединяются

#### Scenario: Crash до обновления queue index
- **WHEN** admitted Run содержит canonical QueueTaskBinding, а queue index ещё не обновлён
- **THEN** Light показывает связанный Run по binding/intended identity, не теряет его и не выполняет reconcile

### Requirement: Этапы и причины не меняют сохранённую семантику

Карточка SHALL показывать recorded название задачи/проекта с явным fallback, queue state, Run status/outcome, причину ожидания/остановки, current round/candidate при наличии достоверного источника, все активные branches/phases и scope результата. Phase SHALL происходить из exact declared mapping закреплённых workflow/stage refs либо достоверной структурированной metadata. Generic workflow без mapping SHALL показывать «В работе / этап не сопоставлен» и actual stages; stage name, prose и лог MUST NOT определять phase или outcome.

Ready/pending, фактически начатая работа, ожидание decision/resource/signal, partial/failed, cancelled и uncertain SHALL различаться. Owner blockers и unresolved effects SHALL оставаться заметны при параллельной работе. QueueTask done SHALL означать только явное локальное complete; succeeded Run при claimed task SHALL показывать «Результат получен» с незавершённой task acceptance. Standalone succeeded Run SHALL называть Run scope. Ни одна колонка MUST NOT утверждать merge/deploy/external issue close без такого recorded evidence.

#### Scenario: Generic и declared feature
- **WHEN** доступны fixture feature с mapping и generic workflow без него
- **THEN** первый показывает declared фазы, второй — actual stages и unmapped fallback без AI Factory assumptions

#### Scenario: Parallel и owner blocker
- **WHEN** один Run имеет работающую ветвь, ожидающий ресурс и pending owner decision в другой ветви
- **THEN** одна task-карточка показывает все ветви и причины, а группа «Нужно участие» не выдаёт работу за безопасно завершённую

#### Scenario: Run успешен до task complete
- **WHEN** native task claimed и её frontier Run succeeded
- **THEN** карточка сообщает успешный результат Run и отсутствие owner complete, не обозначая завершённой всю task или deployment

### Requirement: Review summary имеет typed provenance и консервативный итог

Каждый review summary SHALL сохранять declared lens/applicability, source/Run/invocation/iteration identity, candidate/round/request basis, exact report/gate refs и schema versions, gate scope, freshness и mapping digest. Summary SHALL раздельно показывать check result, gate acceptance и краткие findings с truncation; подробности SHALL открывать источники и raw preview по explicit action. Недостаточные/неподдерживаемые источники SHALL оставаться unknown/unavailable с причиной.

`StepResult pass` MUST NOT доказывать passed review. Passed SHALL требовать valid fresh report, совпавшие candidate/round/request, выполненные required checks и доказанную принимающую gate связь без blockers. Defect, unavailable/blocked, waiting, disabled, not-applicable, unknown/invalid/stale, warnings и passed SHALL различаться по фактическому контракту. Missing report MUST NOT означать ни PASS, ни enabled review; optional skip MUST NOT означать выполненную проверку. Aggregate gate MUST NOT создавать ложные дефекты отдельных lens либо общий PASS при mixed/blocking outcomes. Model/session independence MUST NOT синтезироваться из labels или заявления хоста.

#### Scenario: Usable report и blocked gate
- **WHEN** producer вернул `StepResult pass`, а его typed gate имеет `status=blocked`
- **THEN** summary показывает блокировку приёмки и источник причины без общего PASS

#### Scenario: Набор состояний и backend
- **WHEN** typed fixtures содержат passed, defect, unavailable required check, optional excluded, disabled, pending, missing и invalid report
- **THEN** соответствующие состояния различимы, backend без UI/UX contract не получает фиктивные UI/UX проверки, а source refs доступны из details

### Requirement: Repair и parallel сохраняют границы candidate и round

Recorded repair/new candidate SHALL инвалидировать прежнюю приёмку до fresh acceptance. Previous reports SHALL оставаться историческими/stale; отсутствие нового candidate MUST NOT продлевать прежний PASS. Candidate freshness SHALL опираться на recorded basis и declaration, без чтения текущих Git/filesystem bytes. Несопоставимые current Runs, iterations и parallel branches SHALL сохраняться раздельно, unknown round SHALL оставаться unknown. Gate без candidate/round SHALL связываться через точные recorded producer inputs/outputs либо оставаться с неизвестной свежестью.

#### Scenario: Repair после PASS
- **WHEN** после fresh accepted review начат recorded repair и затем записан другой candidate
- **THEN** прежний PASS сразу становится historical/stale, новый candidate ожидает собственных reports/gates и не наследует приёмку

#### Scenario: Mixed parallel review
- **WHEN** security lens unavailable, code lens passed, а UI/UX не применимы
- **THEN** каждый lens сохраняет свой scope/status, aggregate acceptance не становится passed и не требует новых reviewers

### Requirement: Optional mappings не меняют workflow producers

Light SHALL поддерживать явный bounded versioned declarative mapping закреплённых definitions и typed ports/fields в phase/review projection. Mapping SHALL фиксироваться digest и разрешать только declared selectors/correlation, без скриптов, discovery случайных файлов, inference по prose или runtime hardcode consumer namespaces. Unsupported/no mapping SHALL сохранять generic обзор и явно показывать отсутствующие review данные. W1/W2/W5 SHALL квалифицироваться на exact typed fixtures через optional data mappings Core; дополнительные readers, изменения classic/profiled/consumer packages и package versions ради Light MUST NOT требоваться.

#### Scenario: Unsupported producer
- **WHEN** workflow не публикует достаточных typed refs или не соответствует exact mapping version
- **THEN** task и Run остаются видимы с «нет данных» для review и причиной; Light не читает случайный Markdown и не запускает reviewer

### Requirement: Read projection ограничена и согласована

Light SHALL предоставлять bounded pagination task/Run/review summaries, totals/counters с явной областью и полнотой, conditional polling и per-source freshness. Ни общий предел 200, ни размер одной backend page MUST NOT усекать backlog; ≥1000 tasks/Runs SHALL оставаться доступны поиском и страницами. Board/poll MUST NOT загружать полные graphs/traces, все artifact bodies или всю историю при каждом запросе и MUST NOT требовать filesystem scan.

Один опубликованный source generation SHALL читать queue, bindings, selected Runs и admission facts одного согласованного authority cut. Между authorities SHALL показываться отдельные cuts/freshness, без выдуманного глобального cut. Пагинация SHALL сохранять source generation vector до expiry; late responses другого filter/generation SHALL отбрасываться. Expired page SHALL требовать явного обновления с сохранением filters. Inaccessible source SHALL отдельно показывать ошибку и stale last-good данные, сохраняя здоровые проекты. Индексация SHALL показывать coverage, неизвестный total MUST NOT заменяться нулём.

Квалификация SHALL измерять выбранные response-size/request-count/backend-work/latency budgets на большой fixture и actual browser; название light либо только mock HTTP MUST NOT заменять измерения.

#### Scenario: Большой backlog и unchanged polling
- **WHEN** fixture содержит ≥1000 tasks/Runs и пользователь открывает первую страницу, ищет позднюю task и оставляет unchanged poll
- **THEN** страницы и поиск охватывают backlog, polling ограничен и conditional, response не содержит all-history/graph/trace payload, а измерения budgets записаны

#### Scenario: Concurrent writes и потеря source
- **WHEN** queue/Runs/admission меняются во время read, затем один authority становится недоступен
- **THEN** response не смешивает cuts одного source, недоступный source помечен stale/error, а здоровые карточки продолжают обновляться

### Requirement: Повседневный UX сохраняет контекст

Light SHALL иметь search/project/source filters, counters, empty/loading/error/stale states, desktop/mobile layout, theme и keyboard navigation. Данные SHALL отображаться текстом без выполнения записанного HTML/scripts. Poll SHALL сохранять focus/scroll/filter/selection. Каждая Run-ссылка SHALL вести к exact source/Run полного monitor по настроенному base URL, с encoded parameters и поддержкой proxy prefix; возврат SHALL сохранять контекст Light. Несколько current Runs SHALL иметь отдельные ссылки.

#### Scenario: Filter, deep link и возврат
- **WHEN** пользователь с клавиатуры открывает Run из filtered поздней страницы Light через full monitor и возвращается
- **THEN** выбран exact authority/Run, filters/page/позиция сохраняются, encoded IDs и base URL prefix не ломают route

#### Scenario: Mobile и ошибка обновления
- **WHEN** экран узкий и очередной poll завершился ошибкой
- **THEN** карточки и filters остаются доступны, last-good данные помечены свежестью/ошибкой, focus не сбрасывается и пустой успех не рисуется

### Requirement: Light listener остаётся строго read-only

Light SHALL слушать loopback, сохранять Host/same-origin/no-permissive-CORS и owner/source access guards, не открывать arbitrary paths и не предоставлять writable endpoints полного monitor. На public deployment SHALL сохраняться существующий authenticated reverse proxy. Read-only mode MUST NOT давать queue start/continue/approve/complete, maintenance/delete, shell commands либо auto-actions. Rollback SHALL отключать только Light routing/service без изменения authority и full monitor.

#### Scenario: Попытка mutation через Light
- **WHEN** запрос обращается к maintenance, queue mutation или использует write method на Light listener
- **THEN** операция отказывается без side effects; соответствующие full-monitor routes продолжают прежнюю защищённую работу на своём listener

#### Scenario: Released поставка до server rollout
- **WHEN** локальная и released qualification Light завершена, но deployment ещё не принят владельцем
- **THEN** E6 production gate остаётся открытым, domains/auth не переключаются на непроверенный frontend
