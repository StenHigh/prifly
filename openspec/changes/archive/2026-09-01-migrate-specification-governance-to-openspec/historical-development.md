# Разработка и проверка

## Изменение спецификации

OpenSpec ведёт только изменения спецификации и development process Pri-Fly. Он
не нужен готовому binary, не является runtime component и не заменяет
пользовательский YAML contract. Перед изменением нормативного поведения
прочитай [карту источников](../openspec/SOURCE-OF-TRUTH.md): до поэтапного
cutover указанный там source set остаётся единственной правдой.

Для local authoring установи закреплённый development CLI:

```sh
npm install -g @fission-ai/openspec@1.11.0
```

Создавай изменения `openspec new change <kebab-case-name>` и проверяй их
`openspec validate <name> --strict`. Historical evidence и manifests не
переписывай ради новой структуры.

## Термины и именование

[Словарь Pri-Fly](glossary.md) закрепляет канонические понятия, определения и соответствия нынешним Go/JSON-именам. Перед добавлением сущности проверь, не описана ли она уже. Изменение имени или смысла должно обновлять словарь, затронутый исходный раздел ТЗ, код, schemas, примеры и тесты в одном изменении; wire-совместимость рассматривается отдельно. Краткое Go-имя из карты не считается новой сущностью.

`TestGlossaryBindings` в существующем `internal/runtime/contracts_test.go` сверяет указанную в словаре карту с Go AST и JSON struct tags; входит в `make test` и `make check`. Тест обнаруживает устаревшие перечисленные bindings, но не доказывает полноту словаря или семантическое соответствие всего текста. Новые F2-типы не создаются только ради заполнения карты.

## Команды

```sh
make build
make check
make e2e
make release
```

`make check` выполняет обычные Go tests, race detector, vet и проверку generated public schemas. Integration tests запускают настоящие локальные процессы, используют Unix sockets, process groups и SIGKILL **только принадлежащих тесту helpers**. Sandbox, запрещающий эти операции, не является квалифицированной средой: разрешите тестам нужные операции; не отключайте assertions ради sandbox-pass. Нужен C compiler для CGO и race detector. На первой проверяемой машине использован Apple clang.

Полный native race-набор после P2-04 превышает стандартный десятиминутный timeout Go. `TEST_TIMEOUT=20m` ограничивает каждый test package; `make check TEST_TIMEOUT=30m` может явно изменить только этот предел. Лимиты исполнения Attempts и subprocess helpers от него не меняются.

Тесты работают в временных каталогах, не используют production credentials, SMSPlace, AI Factory, LLM или платные API. Не запускайте их с рабочей authority в качестве fixture. `test/e2e/verify-cli.py` оставляет отдельный тестовый проект и JSON evidence с digest бинарника. Встроенные helper tests не являются публичными операциями продукта.

`make e2e` также выполняет `test/e2e/verify-core.py`: самостоятельный проект P2-01 с конфигурацией, projections и известной ошибкой процесса. Проверка настоящего обновления запускается отдельно: `python3 -B scripts/verify-upgrade.py --old-binary PATH_TO_F1 --new-binary bin/prifly --evidence NEW_FILE.json`. Она создаёт собственные authorities, вызывает оба бинарника и не клонирует SQLite. Нужен сохранённый прежний F1 executable; сравнение одной сборки с самой собой запрещено.

`make release` перепроверяет идентичность native rebuild, выполняет empty-install/doctor, собирает binary/source/docs/examples/notices, build manifest и SHA256SUMS в `dist/`. Само создание архива **не меняет** статус F1 или критериев приёмки. Сборка с dirty tree явно записывает это обстоятельство; source manifest сохраняет точные hashes. Скрипт не публикует GitLab release и не подписывает бинарник.

## Структура ядра

| Путь | Ответственность |
|---|---|
| `internal/flow` | Bounded [YAML authoring](workflow-yaml.md) и pre-lock workflow aliases, exact refs, schema/graph/port validation, F1 sequence и core DAG/calls/repeat, projections/configuration declarations, hook contracts |
| `internal/local` | SQLite transactions/receipts/history/cuts, immutable blobs, native process ownership/accounting |
| `internal/runtime` | Admission и состояния Run/Invocation/Attempt, local process binding, acceptance/control, publications, projections/telemetry |
| `cmd/prifly` | CLI/формат ответа и transport без бизнес-решений в shell |
| `cmd/schema-gen` | Development generator закрытых public DTO schemas |
| `examples` | Необязательные Python wrappers, проверки и независимые пользовательские executables; они не являются source graph project workflow |

Авторская JSON Schema проверяется отдельным вычислительным helper того же executable с hard deadline. Поэтому `flow.Compile` включает OS adapter; чистые plan/routing функции не читают мир. Main и TestMain поддерживают закрытый helper entrypoint до обработки обычных флагов. Это не отдельная устанавливаемая программа, worker шага, plugin или AI-вызов. Подробные пределы — в [ADR-0001](decisions/0001-foundation.md).

P2-02 добавляет pure Predicate evaluator и проверку graph paths в `internal/flow`; `internal/runtime` разрешает только закреплённые JSON inputs/results и сохраняет ChoiceDecision. AST limits проверяются до рекурсивной schema validation и только в позициях условий контракта. Не добавляйте template engine, live sources, worker или общий plugin interface ради choice. Порядок вычисления, ошибки, data availability и версия decision определены в [ADR-0003](decisions/0003-choice.md); изменение этих правил требует синхронизации WF-009/010, API-010, словаря и fixtures.

P2-03 добавляет Call и настоящий WorkflowInvocation tree. `runtime.Invocation` — краткое Go-имя существующего понятия, не новый Run. Local stage lookup, inputs, outputs, hooks и executor bindings разрешаются через owning invocation; общий Run CAS, control epoch, budgets и unknown barrier сохраняются. Compiler кеширует exact child definitions, но считает стоимость каждого call site. Нельзя использовать map по одному Stage ID для всего дерева или «возобновить» cancelled child новым worker.

Принятые границы — [ADR-0004](decisions/0004-call.md): root depth 0 и relative ancestor limits; call entry/return; child configuration pinning при Start; scoped pause/cancel; сохранённые entry/finish/return. Минимальный author-local alias resolver разрешён владельцем до lock, а package install/trust остаются P2-07. Поиск владельца invocation пока использует bounded Store scan до 1000 Runs / 64 MiB; не вводите отдельный индекс/журнал до повышения квалифицированной capacity. P2-04 добавляет свежие body invocations итераций отдельно от Call и переиспользует локальный selector для author `repeat.body_workflow_ref`; machine refs остаются exact, mixed call/repeat cycles запрещены. Каркасы ещё не реализуемых operators не вводятся.

Пределы WorkflowRevision не расширяют exact PolicyBundle: новая `core:policy/local@2.0.0` разрешает depth 8, прежняя v1 сохраняет depth 0 и прежние bytes. F1 default остаётся v1, новый Core default — v2; выбор v1 в Core не получает новых полномочий. При call default/project configuration подставляется только при отсутствии ключа binding. Optional binding с отсутствующим источником должен оставить input отсутствующим, а не незаметно выбрать default.

Интерфейс с единственной реализацией, plugin framework, server, workflow queue и интеграция с model SDK намеренно не добавлены. `flow.Profile` — явный предел F1, `flow.CoreProfile` — отдельная семантика расширений. `Compile` остаётся F1; `CompileProfile` требует явного выбора. Возможности описывает `prifly capabilities`; отсутствие capability нельзя обходить сменой поля в Run. F2 добавляется по этапам [roadmap](roadmap/roadmap.md), без изменения смысла принятых F1 Runs. Текущая разработка и незавершённая приёмка F1 описаны в [f2-progress](f2-progress.md).

P2-04 реализуется по [ADR-0005](decisions/0005-repeat.md). `Plan.Repeats` хранит body definitions отдельно от Calls; общий compiler cache не объединяет executions. `SelectRepeat` — чистый расчёт post-body route, не admission: runtime записывает RepeatDecision и создаёт next body атомарно. Не разворачивайте max_iterations копий графа; bounds считаются по outcomes/prefixes с безопасной арифметикой до предела.

Core cap — 100 iterations, 256 StepInstances, 1024 control transitions, depth 8. Repeat entry charge=1; каждый post-body decision=1, включая next body creation без второго charge. Initial/next maps не наследуют значения друг друга; max1 исключает обязательность отсутствующего next binding, но не validation объявленных полей. `limit_configuration` ссылается только на project-scoped configuration input своего WorkflowRevision; Start закрепляет positive integer не выше author max_iterations, а compiler всегда считает по author ceiling. Каждый new body/continue commit проходит прежний 8 MiB state admission reserve. Failed/on_error repeat не экспортирует stage_output последнего body, даже если тот completed.

## Версии и generated files

Первый checkpoint P2-05 использует [ADR-0006](decisions/0006-context-checks.md): `core-configuration/2` явно выбирает `CompileCore`, typed resources и state/read/next/preview4. Default configuration1 не меняется. Ресурсы хранятся отдельно от JSON definitions; profiles и encoding/media покрыты immutable lock. `local-context/2` предоставляет полный ContextManifest, rendering и source files, inputs переиспользуют эти же файлы. Byte cap учитывает rendering и каждую source copy; silent truncation, tokenizer и fresh/AI qualification не поддерживаются.

`source import --file FILE --type json|blob` создаёт SourceSnapshot descriptor с реальной acquisition provenance; external metadata не объявляются проверенными. `ref FILE --id ID --version VERSION --raw-text` вычисляет exact UTF-8 resource ref без нормализации. JSON/YAML default сохранён. Перед использованием SourceSnapshot общий port validator проверяет acquisition producer, content и provenance, не только JSON shape.

Первый checkpoint `445553f` подготовил CheckDefinition/Request/Result и компонент CheckExecution отдельно. Следующий checkpoint включает `automatic_checks` только для configuration2: PendingAcceptance удерживает точные bindings/candidate, checker использует общий slot/control limits, cancellation/unknown и отдельную наблюдаемость. Полный этап P2-05 пока не квалифицирован; наличие capability в dev build не является release evidence. Check executors задаются по ID определения в configuration, закрепляются по exact ref и требуют operation `check` у local adapter v2.

Не объединяйте process settlement с result acceptance. У завершённого producer Attempt может быть `Accepted=nil`, а Step остаётся `verifying`. Время Attempt.Settled не переносится на конец checker; Step.Settled фиксирует позднейшую приёмку/отказ. Проверенные входы и projections после resume потребляются по прежним refs. Подготовленные outputs не получают public metadata до обязательных checks; после durable `acceptance.passed` допустима metadata без committed StepResult, её повторная публикация проверяет immutable identity. Soft quota не должна отклонять settlement ранее допущенного процесса; новый checker admission quota обязан соблюдать.

Baseline [protocol.schema.json](spec/contracts/protocol.schema.json) сохранён. Новые contracts находятся в [schemas/foundation](../schemas/foundation), отдельно от v1 RunSnapshot. `schema NAME` отдаёт явно выбранный self-contained контракт. Shape pass не заменяет ownership, policy, cross-field, budget и execution checks.

P2-01 добавляет отдельный [core bundle](../schemas/core/public.schema.json) и `WorkflowRevisionV2`; foundation bundle защищён тестом неизменного SHA-256. При расширении общего Go DTO generator исключает новые поля **до** обхода F1 types. Изменение Go struct само по себе не разрешает расширить старый публичный контракт. Подробности совместимости, scoped config и projection provenance — [ADR-0002](decisions/0002-core-workflow.md).

ChoiceDecision P2-02 публикуется отдельно: `choice-decision/1`, schema `urn:prifly:choice-decision:1`, CLI `schema ChoiceDecision`. Событие `stage.choice_decided` не требует добавления полей в сохранённые Run/state/read DTO или изменений foundation/core P2-01 bundles. Существующие histories и fixed-cut telemetry не пересчитываются по новым правилам.

P2-03 публикует отдельный [invocation bundle](../schemas/core/invocations.schema.json) `urn:prifly:core-invocation-public:2`: state/read/next/preview v2, CoreWorkflowInvocation, LocalRegistryV2 и CoreCapabilitiesV2. Старые bundles сохраняют прежние bytes; новые поля общего Go DTO исключаются из старого generator path до рекурсивного обхода. Обычный flat Core workflow остаётся state1, workflow с calls или declared partial — state2. В state2 ready frontier существует только в Invocations; Run.ready_stages запрещён при decode. Capabilities/2 перечисляет все поддерживаемые state/read versions, singular поля обозначают основную новую версию.

Исправление отражения `time.Time` в новой schema не разрешает менять опубликованный Core bundle с прежней ошибочной object shape. Версионная граница должна быть явной и проверяться сохранёнными hashes. Неподдерживаемые state/events блокируют всю authority для старого reader; downgrade нельзя проверять только одним старым Run, игнорируя новые записи.

Nested timing использует `core-timing/1`, telemetry — `core-telemetry/2`, timing descriptors revision 2. Выбор версии определяется выбранными Runs на cut; не проверяйте наличие state2 только в latest snapshot всей authority. Mixed groups разных revisions не объединяются. Новые точные clock fixtures проверяют арифметику, а native Call tests — реальное создание и settlement; ни то ни другое не заменяет аппаратный suspend/resume gate.

Repeat-containing closure получает отдельный `urn:prifly:core-repeat-public:3`, `schemas/core/repeats.schema.json`: state/read/next/preview v3, RepeatProgress, RepeatDecision и CoreWorkflowInvocationV3. Calls/partial без repeat сохраняют v2, старые flat Runs — v1; новые поля общего DTO исключаются из generator paths всех прежних bundles до обхода types. Capabilities/2 и Registry2 сохраняют shapes. Не меняйте опубликованный v2 bundle ради новых iteration fields.

Timing tree для repeat использует реальные body invocations и уникальные Attempts. State v3 не требует механической смены core-timing/1/core-telemetry/2, если arithmetic/labels/order прежние; exact state-version guards при этом должны поддерживать v3, а не отправлять его в legacy flat fallback. Новые измерения/labels требуют своего согласованного versioned contract и fixed-cut regression.

Для state4 используются `core-timing/2`, `core-telemetry/3`, timing descriptors revision 3. CheckExecution имеет собственные узлы, длительности, counts и report outcome; прежние executor totals и Step/Attempt population не включают фиктивную работу. Pending acceptance/settlement не выдаются за законченные latency samples; отрицательный check не становится принятым result. Проверяйте context/config pins каждого выбранного Run, даже когда compiled workflow plan берётся из общего кэша. Прежние report DTO, catalogs на старых cuts и пять опубликованных schema bundles сохраняются.

Для assisted session state используются `core-state/5`/`core-read/5` и седьмой bundle [sessions](../schemas/core/sessions.schema.json) `urn:prifly:core-session-public:5`. Поля AIF-04 исключаются из обхода всех шести прежних bundles тем же механизмом, что и поля P2-05; их bytes проверяются неизменными. Assisted step выбирает исполнение своим StepDefinition, а не конфигурацией, поэтому `core-configuration/2` не менялся и не должен меняться ради нового способа исполнения.

Сообщённая стоимость использует `core-state/11`/`core-read/11`/`core-next/11`,
`assisted-session/2` и отдельный bundle
[reported-cost](../schemas/core/reported-cost.schema.json)
`urn:prifly:core-reported-cost-public:11`. Новые assisted Runs получают эту
версию; прежний Run продолжает выдавать и принимать `assisted-session/1` без
поля стоимости. `ReportedCost.amount` — decimal string, не JSON number;
`source` является заявлением хоста, а не доказанной provider identity. Поля
исключаются из обхода всех двенадцати прежних bundles до рекурсии, и их bytes
должны оставаться неизменными. Не добавляйте rate card или вычисление цены в
runtime: ядро принимает готовое число и сохраняет его на Attempt.

Ранняя публикация артефакта использует StepDefinition v3/v4, команду
`PublishStepPublicationCommandV2`, `artifact-publication/1` и
`core-state/read/next/preview/step-read/12`. Отдельные schemas —
[StepDefinition v3](../schemas/core/step-definition-v3.schema.json) и
[StepDefinition v4](../schemas/core/step-definition-v4.schema.json),
[artifact publication](../schemas/core/artifact-publication.schema.json)
`urn:prifly:core-artifact-publication:12`; четырнадцатый public bundle не меняет
байты тринадцати прежних. SQLite storage и EventEnvelope остаются v1. Большое
чтение, hashing, schema/type check и sealing выполняются до SQL transaction;
writer затем повторно проверяет publisher generation/stop и только durable
ArtifactPublication объявляет item готовым. Artifact metadata, оставшаяся без
этой записи после отказа, является orphan, не публикацией. Ранняя запись не
подставляется в `stage_output`. Узкий `publication_subscription_once` использует отдельный
[publication-source/1](../schemas/core/publication-source-v1.schema.json):
StepDefinition v4 должен разрешить `declared_subscribers`, source связывает
direct sibling producer с JSON wait, а `event_ref` становится exact consumer
input. Для `keyed_many` отдельная команда `PublishStepPublicationCommandV3`
принимает `kind=close` только с exact ordered `item_keys`; authority запечатывает
полный `artifact-manifest/1`, затем фиксирует единственный
`artifact-closure/1`. Новые Runs получают
`core-state/read/next/preview/step-read/13`, а пятнадцатый
[artifact closure bundle](../schemas/core/artifact-closure.schema.json)
`urn:prifly:core-artifact-closure:13` оставляет прежние четырнадцать bundles
байт-идентичными. После commit поздний item отклоняется до чтения candidate;
close не завершает Attempt и не меняет RunVersion. Не расширяйте одноразовый
wait полями cursor/queue: stream требует отдельного исполняемого lowering.
Полные границы — [ADR-0010](decisions/0010-early-artifact-publication.md) и
[ADR-0011](decisions/0011-once-publication-subscription.md), exact close —
[ADR-0012](decisions/0012-artifact-close-manifest.md).

Непустой `content_check_refs` artifact hook теперь требует `core-configuration/2`
и даёт state/read/next/preview/step-read v15 в отдельном
[checked-publication bundle](../schemas/core/publication-checks.schema.json)
`urn:prifly:core-publication-checks:15`. Authority сначала сохраняет собственную
sealed copy как один pending candidate, исполняет на ней каждый exact
CheckDefinition и делает ArtifactPublication/delivery только после всех
`pass` evidence. `fail`/`inconclusive` удаляет pending candidate без публикации
и не меняет verdict producer; явная следующая публикация остаётся новым intake.
Для этой формы объявляется capability `artifact_publication_checks`; generic
`artifact_checks`, waivers per-item checks, async managed-process overlap и
retention/GC не выводятся из неё. Шестнадцать ранних bundles остаются
байт-идентичными. Граница — [ADR-0014](decisions/0014-checked-artifact-publication.md).

Ограниченный stream lowering использует additive
[WorkflowRevision v3](../schemas/core/workflow-revision-v3.schema.json),
[publication-source/2](../schemas/core/publication-source-v2.schema.json) и
шестнадцатый bundle
[publication subscription](../schemas/core/publication-subscription.schema.json)
`urn:prifly:core-publication-subscription:14`. Каждый direct subscriber repeat
в sibling-ветви получает собственные handle/cursor и assignment ledger; body
исполняет `wait → choice → call`, передаёт exact item через
`from=publication`, а cursor — через `iteration_output`. Compiler разрешает
item binding только после точного доказательства `delivery.kind == Item`.
Pending assignment двигается лишь после settlement body; close и deadline
доставляются отдельно как `Closed` и `Interrupted`. Старые пятнадцать bundles,
WorkflowRevision v1/v2, `publication-source/1`, storage и EventEnvelope
неизменны. Границы —
[ADR-0013](decisions/0013-each-publication-subscription.md).

Initial mode является частью immutable source contract, не настройкой cursor:
`publication-source/3` и `/4` задают `new_only` для once и stream. При
регистрации Authority записывает current event sequence в отдельный state/read
family v16; delivery допускает только publication или closure строго после
этого cut. Старые state contracts явно отвергают новое поле, retained `/1` и
`/2` остаются без cut. Это не reservation и не immediate producer-failure
wakeup; граница — [ADR-0015](decisions/0015-new-only-publication-source.md).

Для branch fan-out используются `core-state/7`/`core-read/7` и восьмой bundle [parallel](../schemas/core/parallel.schema.json) `urn:prifly:core-parallel-public:7`: ParallelProgress, JoinDecision, CoreWorkflowInvocationV7 и `branch_id`. Поля P2-10 исключаются из обхода всех семи прежних bundles тем же механизмом, что и поля AIF-04; их bytes проверяются неизменными.

Номер версии состояния не объявляет класс запуска. Веер доходит до `core-state/7`, не объявляя ресурсов контекста, поэтому проверки вида «версия ≥ 4 ⇒ есть context pins» неверны. `contextPinnedInvariant` требует совпадения ровно с одной из двух закреплённых форм целиком; для класса без контекста проверяется именно отсутствие соответствующих закреплений, а не отсутствие проверки. Класс не определяйте по непустоте полей состояния: пустая карта или срез не переживают сериализацию и вернутся как отсутствующие. Добавляя версию поверх контекстной, проверьте, какие инварианты опираются на номер как на признак класса.

Замыкание плана обязано включать ветви. `workflowPlans` обходит `Calls`, `Repeats` и `Branches`; пропуск последних скрыл бы ресурсы, бюджеты и требуемую версию состояния вложенных определений. У параллельного этапа определение адресуется идентичностью ветви: `BodyPlan` для него возвращает nil, используется `BranchPlan(stage, branch)`.

Не расширяйте gates вида `SchemaVersion == CoreContextStateVersion` перечислением версий. Используйте предикаты `isContextState`/`isSessionState`: новая версия, несущая прежние возможности, иначе молча теряет их и даёт отказ на декодировании, а не понятную ошибку.

## Факты, выявленные в работе

Записаны потому, что каждый из них однажды стоил времени или едва не превратился в неверное утверждение журнала.

**Проверка целостности выбирается по версии хранилища, которую надо знать до проверки.** `Store.open` вызывал `verify` до присвоения `info.StorageVersion`, поэтому база v2 проверялась правилами v1: `authority_commands` не входили в общий cut и любое повторное открытие после первой authority-команды падало с `ErrIntegrity`. Ни тесты storage, ни тесты control plane не переоткрывали store после authority-записи, поэтому зелёный `make check` этого не показывал. Регрессия закрыта `TestStoreReopensAfterAnAuthorityCommand`. Правило: контракт, зависящий от версии, проверяйте тестом, который проходит полный цикл записи и повторного открытия.

**Опубликованный контракт может быть невыполнимым.** `SafeRelativePath` был записан через lookahead-конструкции, которых нет в RE2, поэтому валидатор продукта не мог скомпилировать `PackageManifest` и отвергал любой вход. Дефект был не в смысле правила, а в его записи. При замене паттерна обе одинаковые копии (`prifly/contracts` и `internal/flow`) двигаются вместе, а проверяющие артефакты ТЗ перезапускаются: `verify-contracts.mjs` требует Ajv2020 и не устанавливает зависимости — путь `PRIFLY_AJV_MODULE` предусмотрен для внешнего модуля.

**Допустимость отчёта — не бюджет исполнения.** `remainingBudget` отказывает на неквалифицированных часах, потому что выдаёт время работы. Ассистируемый хост этим authority не ограничен, а его отчёт всегда приходит из другого процесса, поэтому общий monotonic domain не существует никогда. Смешивать эти два вопроса нельзя: получится либо неработающий приём отчётов, либо подделка «доверенных часов». Срок handoff решает только допустимость отчёта, сравнивается по записанному UTC, а `deadline_trust` называет часы. Смена локальных часов сдвигает предел — это заявленное ограничение.

**Ассистируемое закрытие не имеет фактов процесса.** Код возврата, время выхода и identity процесса у него отсутствуют. Синтетический `ProcessOutcome` записал бы в журнал наблюдение процесса, которого не было. Закрытие разделено по опоре: report-derived половина общая, process-derived выполняется только при наличии фактов.

**Операционные факты установки.** Свежая установка `init --profile core-workflow/1` выбирает `core-configuration/1`; полный контекст, pinned resources и assisted-исполнение требуют `core-configuration/2` и `core:adapter/local-process@2.0.0`, а CLI-команды для переключения нет — `prifly.json` правится явно. Это runtime-конфигурация, не authoring source современного project workflow: для него действует [YAML guide](workflow-yaml.md). `prifly inventory` возвращает и builtin refs, поэтому автор шага берёт exact digest адаптера оттуда, а не из исходников. Claim рабочей копии берётся до старта Run и в этой поставке допускается ровно один активный; принадлежность claim'у Run не приписывается. Идентичность команды отчёта выводится из его содержимого, поэтому точный повтор идемпотентен, а исправленный отчёт — новая команда, а не занятая идентичность.

Storage version 3 заменяет единственный admission slot ограниченным набором с записанной ёмкостью. Свежая установка начинает с ёмкости 1, а миграция с версии 2 переносит занятый слот и сохраняет ёмкость 1: обновлённая установка допускает ровно то же, что допускала. `verify` теперь проверяет, что каждый занятый слот принадлежит существующему прогону и что набор не превышает ёмкость — раньше «два владельца» были невыразимы в одном поле и потому не проверялись.

Фикстуры, подделывающие старую версию хранилища, обязаны убирать структуры новых версий, а не только откатывать `PRAGMA user_version`. Иначе тест проверяет миграцию из состояния, которого в поле не бывает, и настоящее расхождение остаётся незамеченным.

После изменения public DTO:

```sh
make schemas
make schemas-check
```

Generator публикует одинаковые bytes в runtime embed и schema distribution; запись атомарна. Это developer tool, не runtime reflection model. Изменения публичной семантики после первой поставки потребуют новой contract/profile/storage версии и regression fixtures; нельзя только обновить существующий `$id` и прочитать старое состояние по новым правилам.

## Документы и evidence

[SPECIFICATION](../SPECIFICATION.md) и HTML собираются из нормативных исходников. Правила сборки/проверки документации — в [verification](../tools/docs/README.md). `file-manifest.json` относится к исходной поставке документов; новая реализация закономерно меняет список/байты и не должна переписывать этот manifest для сокрытия изменений.

Нормативные IDs сохраняются. Milestone помечается `done` только после проверки назначенных cases и общих stage gates. Статическое ревью, generated schemas, compilation, unit test и real process/crash test — разные виды evidence. Команда без выполненного результата и подпись агента не являются PASS. Новые функциональные проверки расширяют имеющиеся fixtures; общий framework тестирования не нужен.

Пока нет выделенного CI runner и опубликованной квалификации Linux. Не выдавайте cross-compile или зелёный Linux container job за native macOS process-control evidence. Добавление CI возможно без изменения продукта, когда будет выбран доверенный runner.
