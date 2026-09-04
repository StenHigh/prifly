# Словарь Pri-Fly

Редакция 1.34. Канонические названия, определения и соответствия текущему коду. Словарь охватывает основу F1 и расширения F2; наличие термина **не означает, что возможность уже реализована или принята**. Фактический статус — в [delivery roadmap](../delivery-roadmap/spec.md).

Используем привычный отраслевой термин, когда его смысл подходит. Собственные понятия Pri-Fly определяем явно; сходство названий не означает совместимости с Temporal, Argo, OpenTelemetry или другим продуктом. Английское название закрепляет понятие, русский текст объясняет его. Краткие имена существующего Go-кода перечислены как соответствия, а не как новые сущности.

Словарь задаёт единое именование и краткое значение. Подробные правила остаются в OpenSpec capabilities [исполнения](../domain-execution/spec.md), [runtime](../runtime-resources/spec.md), [управления](../control-security-ux/spec.md), [наблюдаемости](../observability-publication-reactions/spec.md) и [архитектурных решений](../architecture-decisions/spec.md). При противоречии нельзя выбирать удобную формулировку: сначала согласовать и исправить оба места. Словарь сам по себе не меняет wire-схему, сохранённые данные или семантику профиля.

## Навигация

- [Сценарий и исполнение](#execution)
- [Данные и контракты](#data)
- [Управление и права](#control)
- [Состояния, hooks и реакции](#state)
- [История и телеметрия](#history)
- [Расширения F2](#extensions)
- [Соответствия Go и JSON](#bindings)
- [Как менять словарь и предотвращать расхождения](#maintenance)

<a id="execution"></a>
## Сценарий и исполнение

<a id="workflow"></a>
### Workflow — Сценарий

Описание связанной работы: стадии, входы, выходы, переходы и ограничения. Слово без уточнения обозначает сценарий как понятие. Для точной версии используем **WorkflowRevision**, для конкретного запуска — **Run**, для включения сценария внутри запуска — **WorkflowInvocation**. Не используем слово «задача» как общий синоним всех трёх.

<a id="workflow-revision"></a>
### WorkflowRevision — Версия сценария

Конкретное неизменяемое определение графа и его контрактов, закреплённое ссылкой с ID, версией и digest. Обновление сценария не заменяет определение уже принятого Run. В F1 поддержан конечный последовательный профиль `foundation-sequence/1`, а не весь набор операторов F2.

<a id="workflow-authoring-source"></a>
### Workflow authoring source — Авторский исходник сценария

Изменяемое читаемое представление будущей WorkflowRevision. Реализованный
`prifly-workflow/1` — ограниченный YAML без исполняемых tags, anchors и скрытой
подстановки environment: он даёт локальные имена точным Definition references,
сокращает обычные ports/bindings/predicates и опускает только безопасные
структурные defaults. До schema validation, compiler, digest, lock и Run он
однозначно превращается в обычную машинную WorkflowRevision; runtime не
получает второй язык. Это не [WorkflowAlias](#workflow-alias), package template
или новая версия wire-контракта. Полная форма любого машинного поля остаётся
доступна. В Project execution profile YAML — единственный источник graph,
step contracts и настроек authoring package; compiler может разрешить refs и
seal-ить revision, но не создаёт stages, routes, schemas, questions или
limits. Точный контракт — в [workflow and context](../workflow-and-context/spec.md).

<a id="step-authoring-source"></a>
### Step authoring source — Авторский исходник шага

Изменяемый YAML `prifly-step/1`, который перед sealing однозначно превращается
в машинный StepDefinition v2. Он опускает только безопасные пустые collections,
title, JSON port format и обычные port defaults; effects, retry, executor,
result schema и повышенные capabilities остаются явными. Marker не является
полем StepDefinition, новой runtime-семантикой или способом изменить уже
закреплённый Run. Полный справочник полей — в
[`step-authoring-reference.yaml`](../../../examples/authoring/step-authoring-reference.yaml).
Полная форма сценария с комментариями —
[`workflow-authoring-reference.yaml`](../../../examples/authoring/workflow-authoring-reference.yaml).

<a id="workflow-extension"></a>
### Workflow extension — Простое расширение сценария

Версионируемая YAML-запись проекта, которая вставляет один уже объявленный
Project step между двумя exact соседними stages выбранного Workflow authoring
source. `between.from` и `between.to` должны находить ровно один прямой route;
иначе compiler отказывает до package sealing. Extension не изменяет прошлую
WorkflowRevision и не предназначен для repeat, parallel, map, join или
нескольких маршрутов: такие изменения остаются явной правкой полного YAML
graph. Это не runtime operator, plugin и не скрытая автоматика маршрутизации.

<a id="project-workflow-setting"></a>
### Project workflow setting — Настройка сценария проекта

Tracked JSON value в `extend.yaml.settings`, который compiler применяет только
к exact project-scoped configuration input named workflow того же Project
workflow folder. Значение проверяется исходной schema input и становится
default новой WorkflowRevision до sealing; это не environment variable,
runtime override или неограниченный patch definition.

<a id="optional-feature"></a>
### Optional feature — Необязательная часть сценария

Явно названная часть Project workflow authoring source, связанная с одним
boolean project-scoped configuration input через `features.NAME.input`.
`extend.yaml.exclude` задаёт этому input false для новой compilation. Author
сам описывает обычный Choice и safe bypass route; Pri-Fly не удаляет stage и не
выводит маршрут по feature name. Feature metadata не попадает в sealed
WorkflowRevision и не меняет уже начатый Run.

<a id="run-brief"></a>
### RunBrief — Паспорт запуска

Предмет работы, ожидаемый результат, границы, критерии завершения и исходные материалы, сформулированные владельцем. Это собственное понятие Pri-Fly; не prompt модели, не workflow и не разрешение на любые действия. Go-имя сейчас `runtime.Brief`; подтверждение локального владельца в F1 не объявляется независимым многосторонним Approval.

<a id="task-intake"></a>
### Task intake — Приём задачи

Граница **до** RunStart, которая приводит выбранный текст из чата, issue
tracker или файла к одному `TaskInput/1`, а затем к RunBrief и immutable
SourceSnapshot. Реализованный `prifly task prepare` принимает закрытый
TaskInput/1, сохраняет его exact bytes и пишет производный RunBrief в local
authority. Task intake не является Workflow, новой Stage или status Run; это
не автоматический запуск модели. Связь с milestone AI Factory roadmap
необязательна и выбирается владельцем. Границы определяет [исполнение и вход задачи](../domain-execution/spec.md).

<a id="task-input"></a>
### TaskInput — Вход задачи

Versioned provider-neutral документ `task-input/1`: exact исходный текст,
заголовок, результат, границы, критерии, owner confirmation, declared source
provenance и ранее sealed SourceSnapshot refs. Он преобразуется в RunBrief без
изобретения scope, criteria или confirmation. `raw_text` остаётся evidence, а
не становится prompt. Точная форма и CLI — в [исполнении и входе задачи](../domain-execution/spec.md).

<a id="task-source-adapter"></a>
### Task source adapter — Адаптер источника задачи

Будущий versioned adapter, обязательная операция которого — read-only
`read_task(reference) -> TaskInput/1`. Он сохраняет исходный текст и
provenance `{type, external_id, url, fetched_at}`, но не делает GitLab, GitHub,
Jira или иной provider частью ядра Pri-Fly. Право прочитать задачу не даёт
права изменить её: публикация комментария, статуса или merge request —
отдельный внешний ActionIntent с явным решением владельца. Это не общий plugin
framework и не текущая реализованная capability.

<a id="project-execution-profile"></a>
### Project execution profile — Версионируемый профиль исполнения проекта

Tracked часть `<repository>/.prifly/`: явные Project launches, YAML workflow
folders и пример локальной настройки. Шаги, schemas и contexts лежат
только внутри folder того сценария, которому принадлежат.
`prifly project init` создаёт нейтральный пустой layout, отдельную authority,
способную закреплять context resources, и три host-specific entry point
`prifly-run`; он не добавляет какой-либо product recipe. Ignored `local.yaml`
закрепляет для конкретной машины пути к authority и исполняемому Pri-Fly, чтобы
host не угадывал системный `PATH`. Профиль описывает общий для команды
процесс и передаётся через Git. После clone та же команда проверяет shared
profile и exact runners, создавая только отсутствующий ignored `local.yaml`;
она не переписывает общий YAML или reviewed runner. Профиль не является authority state, workspace,
artifact store или местом для credentials. Profile v2 фиксирует стандартные
roots `<repository>/.codex/skills/`, `<repository>/.agents/skills/` и
`<repository>/.claude/skills/` для Codex CLI, Codex Desktop и Claude Code.
Launcher механически передаёт свой host compiler; он не выбирается по наличию
папки. Точный layout и границы — в [сценариях, пакетах, контексте и YAML authoring](../workflow-and-context/spec.md).

Profile `/3` вводит отдельную identity compiled package; переход с `/2` —
явная правка `schema_version` shared `project.yaml`, не результат init/start
или обновления сценария. Первый срез `/3` сохраняет layout `/2`: Git,
обязательные три host roots, explicit host при compile/start и RunBrief при
start. Optional hosts и запуск без Git/brief — следующий срез, не свойство
одного нового marker. `/2` сохраняет прежнюю compilation и exact refs.

<a id="project-workflow-folder"></a>
### Project workflow folder — Папка сценария проекта

Единственная authoring форма Project package source: tracked directory
`.prifly/workflows/NAME/` с обязательным `workflow.yaml` marker
`prifly-project-workflow/1` и необязательным `extend.yaml`. Root хранит package
identity, exact external refs и graph; compiler рекурсивно читает YAML only из
`steps/`, `schemas/`, `contexts/` и child `workflows/` на произвольной глубине.
Каждый такой файл содержит ровно один YAML document; `---` не разрешён.
Пути организуют чтение человеком и не выводят refs или control flow. Context
может назвать raw text только из `.prifly/` либо selected versioned host skills
root этого repository; compiler seal-ит exact bytes. Результат — тот же immutable
package, не второй runtime language и не automatic authority import.

Авторская identity — `id` и `version` исходного package/компонента; она описывает
редактируемый источник, но не alias последней сборки. Для `/3` compiler
сохраняет ID и назначает детерминированную compiled version package и каждому
owned component. Build key — SHA-256 canonical effective authoring closure:
profile, values/settings/extensions, exact external refs, hashes ресурсов,
manifest metadata и catalog с версией алгоритма b1. Все данные generated
provenance определяются этим входом; время и absolute machine paths исключены.
Compiled version имеет форму `0.0.0-b1.` и lower-case base32 полного SHA-256
без padding: для package используется build key, для component — hash tuple
`[build key, kind, author ID, author version]`. Внутренние refs связываются с
compiled components; внешние exact refs не меняются. Сортировка compiled
versions не определяет «последний» авторский release.

`build-provenance.json` (`prifly-build-provenance/1`) — schema-validated inert
file в manifest пакета, не новая runtime сущность. Он связывает author refs
с compiled refs/root; CLI проверяет mapping по фактическим exports перед
выбором root. Final package digest не включается в собственный provenance.
Внешний consumer новой сборки использует её exact compiled ref, а не author
`ID@version`. Legacy package без provenance читается по прежнему exact root
contract. История, collision checks и отдельный trust для каждого exact
manifest сохраняются; сведения о сборке не являются подписью.

<a id="project-launch"></a>
### Project launch — Точка запуска проекта

Явно объявленный элемент `launches` в versioned Project execution profile.
У него есть стабильный ID, title, description, kind и ровно один source внутри
`.prifly/`. Единственный `kind` — `workflow`, ссылающийся на root
`workflow.yaml` Project workflow folder и объявляющий её реальные input ports.
Launcher
показывает все точки через `prifly project workflows`, не выбирает default по
тексту пользователя и спрашивает только требуемые входы выбранной точки. Это
каталог процесса проекта, не adapter внешней системы, не пакетный plugin и не
разрешение начать незакреплённый workflow.

<a id="workflow-repository"></a>
### Workflow repository — Репозиторий сценариев

Любой доступный пользователю Git-репозиторий с одной или несколькими Project
workflow folders. `prifly project workflows add` находит папки только по root
`workflow.yaml` с marker `prifly-project-workflow/1`, копирует выбранную папку
в `.prifly/workflows/NAME/` и объявляет её в `project.yaml`; ничего из
репозитория не исполняется, package не seal-ится и не становится trusted.
Это не Registry, не SealedPackage и не Task source adapter: репозиторий даёт
authoring source, а доверие остаётся решением владельца при `project start`.
Go-представление разобранного SOURCE — `projectWorkflowSource` в
`cmd/prifly/project_workflows.go`.

<a id="workflow-catalog"></a>
### Workflow catalog — Каталог сценариев

Git-репозиторий с одним root `catalog.yaml` `prifly-workflow-catalog/1`:
карта категорий и записи `repository + path + ref [+ commit]`, указывающие на
Workflow repositories. Каталог служит только discovery для `prifly project
workflows search` и `add NAME`; он не переносит bytes, ключи или trust, а его
необязательный `commit` только проверяет identity полученного commit. Это не
DecisionCatalog, не Registry и не source adapter задач. Официальный каталог —
`https://github.com/StenHigh/prifly-workflows.git`. Go-имена —
`projectWorkflowCatalog` и `projectCatalogEntry` в
`cmd/prifly/project_workflows.go`.

<a id="workflow-folder-origin"></a>
### Workflow folder origin — Происхождение папки сценария

Tracked запись `packages.NAME.origin` в `project.yaml`: `repository` без
credentials, `path`, `ref`, exact `commit`, `digest` дерева папки без корневого
`extend.yaml`, необязательные `extend_digest` и `catalog`. Её пишут `add` и
`update`, а проверяет локальный digest, чтобы `update` отличал
upstream-изменения от правок команды. Это заявление установившей команды, а не
TrustDecision, sealed `PackageOrigin` или SourceSnapshot. Go-имя —
`projectWorkflowOrigin` в `cmd/prifly/project_workflows.go`.

<a id="local-authority-state"></a>
### Local authority state — Локальные данные authority

Непередаваемые machine-local state, history, receipts, sealed artifacts,
inventory, session workspace, locks и claimed Git worktree. `prifly project
init` создаёт такой root вне repository. Данные остаются вне
целевого repository, потому что writing step не должен иметь доступ к
доказательствам и управлению своей работы. Текущий raw CLI использует
`.prifly/` именно под выбранным authority project root; это не то же самое,
что Project execution profile внутри repository. Host skill `prifly-run`
читает ignored `local.yaml`, чтобы получить этот root и exact local executable,
и передаёт свой declared host ID; он остаётся host, а не
встроенным AI executor.

<a id="run"></a>
### Run — Запуск

Один управляемый запуск с закреплёнными определениями и входами, общими ограничениями, историей и итогом. Владеет корневой WorkflowInvocation и её дочерними включениями. Run не равен одному процессу ОС: процесс driver может исчезнуть, а сохранённый Run и неопределённость его исполнения остаются.

<a id="decision-catalog"></a>
### DecisionCatalog / DecisionDefinition — Каталог решений Run и его запись

**DecisionCatalog** — закреплённый package контракт известных выборов одного
Run. **DecisionDefinition** задаёт stable ID, typed values или schema,
назначение входа, sensitivity, recommendation и допустимость automatic choice.
Условие видимости может читать только выбранный profile и exact ранее
зафиксированный preflight answer; оно не создаёт Stage, route, effect,
capability или permission. Каталог не извлекается из prose skill и не является
Approval, Grant, ActionIntent либо DecisionArtifact. Точные правила — в
[каталоге решений Run](../run-decisions/spec.md).

<a id="decision-sheet"></a>
### DecisionSheet — Лист предзапусковых решений

Неизменяемый набор profile и applicable preflight values для одного запуска:
catalog digest, источник profile, policy и записи ответов. Он передаётся
совместимому host как контекст уже принятого выбора, а не как permission
переспрашивать или заменить этот выбор. Изменение листа требует нового Run;
он не меняет tracked `extend.yaml` или предыдущий Run.

<a id="decision-request"></a>
### DecisionRequest / DecisionAnswer — Запрос и ответ runtime-решения

**DecisionRequest** — versioned сообщение совместимого executor о ровно одном
объявленном runtime-выборе текущей Attempt; оно несёт Run/Attempt identity,
definition и request digest, поэтому не является произвольным native chat
question. **DecisionAnswer** — typed ответ на этот exact pending request с
ожидаемой generation Run. Просроченный, конфликтный или неизвестный ответ не
передаётся другому step и не делает successor ready. Ни один из этих DTO не
является Approval, Grant, ActionIntent либо DecisionArtifact.

<a id="decision-record"></a>
### DecisionRecord — Запись журнала решений

Неизменяемая запись того, какой definition был pending, answered или defaulted,
какое exact typed value было принято и кем либо какой policy оно выбрано.
Журнал объясняет происхождение выбора в Run report; automatic recommendation
никогда не выдаётся за ответ человека. Запись не разрешает внешний effect и не
заменяет ActionIntent или Approval.

<a id="interaction-policy"></a>
### Attended / autonomous / unattended Run — Режим участия владельца

**Attended** Run ждёт владельца для каждого обязательного невыбранного
решения. **Autonomous** Run может применить только явно разрешённую ordinary
recommendation из sealed catalog и записывает policy source. **Unattended** —
не синоним autonomous: это только такой sealed profile, в котором ни один
известный либо будущий restricted вопрос не требует ожидания. Отсутствие
человека не создаёт Approval, Grant или право расширить scope.

<a id="workflow-invocation"></a>
### WorkflowInvocation — Включение сценария

Один экземпляр выполнения WorkflowRevision внутри Run: корневой либо дочерний. Дочернее включение сохраняет общий Run, журнал, бюджет и управляющие ограничения; это не новый независимый запуск. В F1 и прежнем Core state v1 сохраняется корневая identity без отдельного invocation tree. P2-03 представляет реальный узел деревом `Run.Invocations` в `core-state/2`; Go-тип — `runtime.Invocation`.

Узел хранит exact WorkflowRef, свои input/output refs, status/outcome, Created/Settled и локальный ready frontier. Child связан с parent invocation и caller StageActivation. Счётчики ограничивают всё его поддерево, а не создают независимый бюджет. Root имеет depth 0; `max_child_depth` проверяется относительно каждого ancestor. Завершение child не завершает Run; неизвестный значимый исход сохраняет общий barrier Run. Границы реализации и совместимости задаёт [исполнение и вход задачи](../domain-execution/spec.md).

<a id="call"></a>
### Call — Вызов дочернего сценария

Управляющая Stage вида `call`, создающая одну дочернюю WorkflowInvocation внутри текущего Run и ожидающая её исхода. Публичные inputs/outputs child и `on.<outcome>` задают контракт вызова; `on_error` может обработать известный технический failure. Call не имеет StepResult verdict, собственного worker или отдельного execution slot. Его entry и return расходуют управляющие переходы, работа child — общий бюджет Run и ancestors. Alias разрешается до закрепления определения; исполнение использует exact ref.

`cancelled` child не возвращает outcome и не запускает `on_error`. Caller остаётся waiting с причиной `blocked_child`; resume не создаёт новый child вместо отменённого. Повторное использование того же WorkflowRevision другим Call создаёт другие identities, а не повторно использует прежний результат.

<a id="repeat"></a>
### Repeat — Ограниченный цикл

Управляющая Stage с exact body WorkflowRevision, независимыми initial/next bindings, continue_on, Predicate until и конечным max_iterations. `limit_configuration` может назвать project-scoped JSON input того же WorkflowRevision: его закреплённое положительное целое только сужает author max_iterations, но никогда не расширяет посчитанный compiler budget. Первая body invocation обязательна; проверка выполняется после её принятого outcome. Non-continue outcome и true выбирают on_complete, unknown — on_unknown либо durable condition_unknown, false после последней разрешённой итерации — on_limit. Missing on_complete даёт unhandled_outcome; эти отказы не переходят молча в on_error.

Одна StageActivation создаёт последовательность отдельных body invocations, а не возвращает completed StepInstance в running. Обычные exports принадлежат последнему завершённому body; на on_error failed repeat их не экспортирует. Cancelled body оставляет caller waiting с blocked_child. В P2-04 Core limit — 100 итераций; state/read v3 и точный подсчёт control transitions определены в [исполнении и входе задачи](../domain-execution/spec.md). Отдельный while и автоматический retry не добавлены.

<a id="iteration"></a>
### Iteration — Семантическая итерация

Одна реально созданная body WorkflowInvocation конкретной Repeat StageActivation. Имеет собственную identity и номер от 1; следующие iterations являются siblings, а не цепочкой потомков предыдущего body. Новые StepInstances/Attempts исполняют новую предметную работу на явно переданных artifacts. Retry/resend, Predicate evaluation и polling не увеличивают iteration count. `needs_revision` остаётся verdict внутреннего шага, не новым workflow outcome.

<a id="repeat-progress"></a>
### RepeatProgress — Текущий прогресс цикла

Сохраняемая часть repeat StageActivation: число созданных bodies, exact current body identity и последний RepeatDecision. До first entry count равен 0; создание body и изменение count фиксируются одной transaction. Progress не дублирует status/outcome, definition limits или body outputs и не хранит массив всех traces. История прежних decisions остаётся в journal.

<a id="stage"></a>
### Stage — Стадия графа

Именованный узел определения workflow. Может обозначать выполнение шага или управляющий оператор. В F1 доступны `step` и `finish`; условия, вызовы, циклы и другие операторы относятся к F2. Stage ID принадлежит определению, а не конкретной попытке исполнения.

<a id="stage-activation"></a>
### StageActivation — Активация стадии

Конкретное исполнение стадии в определённой WorkflowInvocation. Отделяет узел определения от его включения в запуск, ветвь или итерацию. Активация вида `step` связана со StepInstance; управляющий узел не требует фиктивного worker. Текущее Go-имя — `runtime.Activation`.

<a id="choice"></a>
### Choice — Выбор ветви

Управляющая Stage вида `choice`, которая один раз выбирает переход по закреплённым данным и объявленным Predicates. `selection=exclusive` требует единственной true branch без мешающего unknown; `first_match` учитывает закреплённый порядок до первой true либо более ранней unknown. Default применяется только при всех false, `on_unknown` — при мешающей выбору неопределённости. Отсутствие этих handlers не подменяется `on_error`. Ошибка вычисления отличается от false и unknown. Точные правила — в [исполнении и входе задачи](../domain-execution/spec.md).

Choice не создаёт StepInstance, Attempt или выдуманный output. Это snapshot decision, а не [Guard](#guard), непрерывно наблюдающий за состоянием. Не выбранная ветвь не исполняется как фиктивный skipped worker. P2-02 относится только к `core-workflow/1`; F1 продолжает отвергать этот оператор.

<a id="choice-branch"></a>
### ChoiceBranch — Ветвь выбора

Именованный элемент массива `choice.branches`: Predicate и target `next`. ID уникален внутри Choice, порядок хранится в WorkflowRevision. Это описание пути графа, не отдельный Run, WorkflowInvocation или процесс. Ветвь, до которой вычисление не дошло, не считается false или unknown.

<a id="step-definition"></a>
### StepDefinition — Определение шага

Переиспользуемая декларация работы: вид, входные и выходные порты, исполнитель, проверки, необходимые возможности и объявленные hooks. Это определение, а не работающий процесс и не текущий статус шага. Его версия закрепляется до использования в Run.

<a id="workspace-tree-binding"></a>
### Workspace tree binding — Объявленная привязка дерева рабочей области

Конечная декларация StepDefinition v5, связывающая совместимый входной и/или
выходной `WorkspaceTreeManifest` с одной ограниченной частью claimed
RepositoryWorkspace. Capture policy разрешает только exact file, один прямой
file-child или одну прямую directory-child с прямыми regular files. Это не glob,
рекурсивная синхронизация, право host читать ArtifactRevision или экспорт всей
рабочей копии. Runtime materialize-ит вход и seal-ит выход; host сообщает лишь
разрешённую location output-only дерева.

<a id="step-instance"></a>
### StepInstance — Экземпляр шага

Конкретное включение StepDefinition в запуск через StageActivation. Имеет собственную identity, состояние, принятый verdict, выходы и попытки исполнения. Два обращения к одному определению не являются одним экземпляром. Go-имя сейчас `runtime.Step`; поля `StepID` в ряде структур означают именно `step_instance_id`.

<a id="attempt"></a>
### Attempt — Попытка исполнения

Одна техническая попытка исполнить StepInstance с определённым допуском, контекстом и исполнителем. Новая Attempt — повтор работы шага, а не повтор доставки сообщения о прежнем результате. В F1 автоматических retries нет; неоднозначность прежней попытки не даёт права запустить следующую.

<a id="dispatch"></a>
### Dispatch — Передача работы на исполнение

Граница между принятым решением запустить работу и фактическим обращением к исполнителю. В F1 dispatch record сохраняется до OS spawn: наличие этой записи ещё не доказывает, что процесс действительно запустился. Потеря driver в этой щели требует учёта неопределённости, а не автоматического повторного exec.

<a id="settlement"></a>
### Settlement — Учёт завершения попытки

Фиксация подтверждённого исхода принадлежащей попытке работы и её обязательств. Принятие кандидата StepResult само по себе не является settlement живого процесса. Execution slot нельзя освобождать лишь по отсутствию ответа или исчезновению старого PID; нужны предусмотренные профилем проверки завершения и владения.

В state4 завершённая producer Attempt может ещё иметь непринятый StepResult: process settlement освобождает slot для обязательных CheckExecutions. Attempt.Settled сохраняет границу процесса; позднейшая приёмка либо отказ фиксируется в Step.Settled. Завершённый процесс не означает принятие его verdict/outputs. CheckExecution имеет собственный settlement и отдельный содержательный исход отчёта.

<a id="uncertain"></a>
### Uncertain — Неопределённый исход

Явное состояние, когда нельзя надёжно подтвердить значимый исход исполнения или эффекта. Это не success, не обычный failed и не разрешение повторить операцию. Продолжение ограничено до предусмотренного восстановления или выяснения фактов.

<a id="step-result"></a>
### StepResult — Результат попытки шага

Типизированное сообщение с identity попытки, verdict, ссылками на outputs и свидетельства. Полученное сообщение сначала является кандидатом. Только проверка и принятие ядром связывают его с результатом шага; раннее сообщение не доказывает завершение процесса. Go-имя — `runtime.Result`; wire-контракт — `StepResult`.

<a id="engine"></a>
### Engine / Runtime — Движок исполнения

Код допуска, переходов состояния, контроля, принятия результатов и проекций. Текущий Go-пакет — `internal/runtime`, основной объект — `Engine`. Здесь runtime означает подсистему Pri-Fly, не стандартный Go-пакет `runtime` и не ИИ-контроллер.

<a id="driver"></a>
### Driver — Процесс, продвигающий исполнение

Компонент, который выполняет разрешённые движком действия и наблюдает за принадлежащими ему процессами. В F1 `Engine.Drive` работает на переднем плане; фонового сервиса или scheduler между запусками нет. Driver не является Authority и не сохраняет право владения процессом лишь по старому PID после перезапуска.

<a id="executor"></a>
### Executor / Worker / Adapter — Исполнитель, рабочая программа, адаптер

**Executor** — привязка к способу исполнения конкретной работы; в F1 это закреплённые executable, arguments, environment и лимиты. **Worker** — выполняющая работу программа; ИИ может использоваться внутри неё, но не обязателен. **Adapter** — интеграция с явным контрактом операций и проверенными свойствами. Эти слова связаны, но не взаимозаменяемы: наличие программы ещё не доказывает свойства адаптера, изоляцию или безопасность повтора.

Пример: одно StepDefinition «проверить документ» используется двумя стадиями — получаются два StepInstance с разными входами. В F1 каждый может получить свою Attempt. Будущий разрешённый retry создаст новую Attempt прежнего StepInstance, а повтор отправки прежнего StepResult новой Attempt не создаст.

<a id="data"></a>
## Данные и контракты

<a id="ports"></a>
### InputPort / OutputPort — Входной / выходной порт

Именованное место данных в определении шага или сценария: формат, schema/media type, обязательность и проверки. Port не равен пути к файлу; наличие файла не удовлетворяет контракт порта само по себе.

<a id="binding"></a>
### Binding — Привязка данных

Явное указание, откуда берётся значение порта: вход workflow, конкретный выход производителя или объявленный literal. Во время исполнения разрешается до точного источника. Поиск «последнего подходящего файла» привязкой не считается.

F1 допускает целый порт. В `core-workflow/1` JSON Pointer выбирает часть JSON с явной `projected_schema_ref`; пустой pointer выбирает весь JSON value, но сохраняет отдельный контракт проекции. Missing отличается от присутствующего `null`. Конверсия другой формы остаётся обычным версионированным шагом, а не скрытой функцией binding.

<a id="field-ref"></a>
### FieldRef — Ссылка на значение поля

Адрес значения для Predicate: источник, port, producer stage при необходимости и JSON Pointer. В P2-02 разрешены только JSON ports закреплённого `workflow_input` и принятого `stage_output`; output разрешается до конкретной StageActivation. Blob port отвергается при compile. Отсутствующий optional источник или поле отличается от существующего null. FieldRef не является ArtifactRef, привязкой нового input port или разрешением читать произвольный файл. Live hooks/state, environment и часы не добавляются в этот контракт; `iteration_output` требует реализации repeat.

P2-04 дополнительно разрешает `iteration_output` только в until своего Repeat: источник — конкретная последняя completed body invocation, не произвольный предыдущий artifact. Parent stage_output здесь ограничен pre-entry facts. Это не расширяет Choice live источниками и не меняет прежнюю choice schema.

<a id="operand"></a>
### Operand — Операнд условия

Одна сторона сравнения: literal допустимого scalar type либо FieldRef. Строковый literal остаётся данными и не исполняется как expression, shell или SQL. Present `null` является значением, а не отсутствующим Operand; несовместимый тип не преобразуется автоматически. Go — `flow.Operand`, JSON `kind` определяет форму.

<a id="input-configuration"></a>
### InputConfiguration — Настройка входа

Декларация изменяемого JSON-входа в WorkflowRevision v2: `configuration.scope` и необязательный `default`; schema и обязательность берутся из того же InputPort. `scope=project` разрешает project override и запрещает run override, `scope=run` разрешает оба. Default принадлежит закреплённой версии сценария; установка пакета для его использования не требуется. Это не настройка executable, permissions, grants или limits. Go — `flow.InputConfiguration`; в StepDefinition такая декларация не добавлена.

<a id="effective-configuration"></a>
### EffectiveConfiguration / ConfigurationValue — Итоговая конфигурация / выбранное значение

**EffectiveConfiguration** фиксирует WorkflowRevision и разрешённые значения настроек при создании Run. **ConfigurationValue** хранит JSON value и источник: `package_default`, `project`, `run` либо `absent`. При `absent` поля value нет; JSON `null` — присутствующее значение. Порядок для объявленных параметров: default → project → допустимый run input. Неизвестные параметры и override вне scope отвергаются. Конфигурация закреплена в lock; смена defaults не меняет принятый Run.

Для calls P2-03 Start сохраняет `workflow_configurations[digest]` по всему closure. Child call binding — override уровня Run и запрещён для `scope=project`. Закреплённый default/project value используется только при отсутствии ключа input в `call.input_bindings`; объявленный optional binding с отсутствующим источником остаётся отсутствующим без fallback. Если required configuration input не связан, Start проверяет наличие валидного закреплённого значения и при отсутствии отказывает, даже если author definition допускает пропуск binding. Предварительный набор конфигурации revision не объединяет input artifacts разных invocations и не переписывается значением последнего call.

Project overrides находятся в `runtime.Configuration.InputValues`, JSON `input_values[workflow_id][port]`, в `core-configuration/1` и `core-configuration/2`. Это обычные входные данные, не хранилище секретов и не механизм расширения прав. Core read/preview показывают итоговую конфигурацию; F1 DTO такого поля не имеет.

Для Repeat initial/next bindings независимы. Каждая map отдельно применяет закреплённый default/project value лишь при отсутствии binding key; declared optional absence не получает fallback и не копирует previous inputs. При max_iterations=1 omitted required next bindings/configuration не нужны, но все объявленные next bindings проходят source/port/type/scope validation. Initial required inputs остаются строгими.

<a id="projection-manifest"></a>
### ProjectionManifest — Манифест проекции

Неизменяемый JSON artifact контракта `json-projection/1`: исходная ArtifactRef, точный pointer, projected schema ref и WorkflowRevision. Производная ArtifactRevision ссылается в provenance на исходный artifact и этот manifest. Так одинаковые bytes, выбранные разными проекциями, не теряют способ получения. Manifest — данные, не исполняемый converter и не доказательство доверия произвольному импортированному файлу.

<a id="artifact-revision"></a>
### ArtifactRevision — Ревизия артефакта

Принятые неизменяемые данные и их описание: identity, revision, digest, формат, происхождение и результаты проверок. Рабочий файл производителя ещё не является принятой ревизией. Текущее описание хранится в `runtime.Artifact`, bytes — отдельно в blob store. Раннее потребление артефактов до завершения producer относится к F2.

<a id="workspace-tree-manifest"></a>
### WorkspaceTreeManifest — Запечатанный манифест дерева рабочей области

Неизменяемый JSON ArtifactRevision `workspace-tree-manifest/1`: relative root,
entrypoint и отсортированный конечный список прямых relative files с точными
ArtifactRef их raw bytes. Он переносит нативный документ или shallow bundle
между declared Workspace tree bindings, не дублируя file contents и не выдавая
ссылке на artifact filesystem-права. Fast/Full документ имеет один file entry;
Ultra bundle — `index.md` и его direct phase files. Path сам по себе не является
ArtifactRef и не заменяет проверку provenance или bytes.

<a id="artifact-ref"></a>
### ArtifactRef — Ссылка на ревизию артефакта

Точная тройка `artifact_id`, `revision`, `digest`. Не является путём к изменяемому файлу и не выдаёт права чтения. При недоступности или повреждении закреплённых данных нельзя молча подставить другую ревизию.

<a id="definition-ref"></a>
### Definition reference — Ссылка на определение

Точная тройка `id`, `version`, `digest`, представленная в Go как `flow.Ref`. Имя и версия без совпадения digest не подтверждают нужные bytes. Не путать с ArtifactRef: у ревизии артефакта другой контракт идентичности.

<a id="workflow-alias"></a>
### WorkflowAlias — Локальный псевдоним сценария

Авторское имя, которое локальный resolver связывает с явно указанным workflow file до создания lock. P2-03 ввёл selector `workflow_ref: {alias: name}` у Call; P2-04 переиспользует его для `body_workflow_ref: {alias: name}` у Repeat. `RegistryFile` версии 2 хранит `aliases[name]=relative_path`; разрешённый subset не включает другие позиции ссылок или сетевое resolution. После resolution в machine definition и Run остаётся exact Definition reference, а общий dependency graph проверяется также на смешанные call/repeat cycles. Это не YAML alias, immutable identity, package trust или разрешение выбирать новую revision во время исполнения. Package lifecycle остаётся отдельной работой P2-07.

<a id="digest"></a>
### Digest / Canonicalization / Pin — Хеш, канонизация, закрепление

**Digest** связывает ссылку с конкретными bytes. **Canonicalization** приводит данные к объявленной однозначной форме перед вычислением соответствующего digest; raw byte digest исходника может отличаться от canonical digest определения. **Pin** закрепляет точную зависимость для запуска. Ни один из этих механизмов сам по себе не доказывает правильность результата или доверие к автору.

<a id="context-manifest"></a>
### ContextManifest — Манифест контекста

Явное описание передаваемого исполнителю контекста и точных источников данных. В F1 локальный `runtime.ContextManifest` связывает inputs, выделенные outputs и зависимости с workspace paths; его wire-схема — `LocalContextManifest`, версия `local-context/1`. Это ограниченная транспортная форма, не обещание реализации всего контекста из полного ТЗ.

Для P2-05 полная baseline-форма имеет Go-имя `FullContextManifest`; это то же каноническое понятие, не другой вид контекста. `local-context/2` описывает отдельно размещение закреплённых источников, полного manifest и rendering. Их exact refs и роли не дают новых прав. Правила реализации — в [сценариях, пакетах и контексте](../workflow-and-context/spec.md); наличие определения не означает завершения P2-05.

<a id="context-resource"></a>
### Context resource — Ресурс контекста

Явно зарегистрированные immutable bytes определения/пакета, выбранные через instructions_ref или context_refs. JSON encoding означает canonical JSON; utf8_text сохраняет исходные UTF-8 bytes. Encoding и media type закрепляются вместе с ref, даже когда digests bytes разных представлений совпадают. Это источник ContextManifest, не ResourceClaim, workspace directory или исполняемое определение. Go `flow.ContextResource` / `flow.ContextResources` — typed compiler records этих источников.

<a id="context-profile"></a>
### Context profile — Профиль контекста

Exact immutable конфигурация assembly/renderer, способов чтения, byte/reference/token limits, isolation и overflow policy конкретного Executor. Профиль не является workflow, prompt или разрешением на retrieval. Отсутствующий token cap допустим только как явно неприменимый к нетокенизируемому исполнению; fresh требует квалификации, а не нового process id.

<a id="source-snapshot"></a>
### SourceSnapshot — Снимок источника

Необязательный immutable descriptor полученных данных: source adapter, точный content ref, external identity/version при наличии, фактическая область получения, observation и provenance. Область описывает выполненное чтение, не выдаёт Grant; external metadata не становятся независимо подтверждёнными фактами. Снимок — данные, не обязательная tracker task и не изменение графа. Импорт не создаёт фиктивные StepInstance/Attempt; actor/operation/область получения проверяются отдельно от пользовательского JSON.

<a id="context-request"></a>
### ContextRequest — Запрос дополнительных данных

Типизированное описание необходимых источников и ограничений, которое шаг может вернуть в объявленном output. Это данные для предусмотренного workflow пути, не Grant, немедленный lookup или право изменить текущий manifest. В P2-05 используется обычный verdict needs_revision; live retrieval остаётся отдельной операцией последующего этапа.

<a id="execution-envelope"></a>
### ExecutionEnvelope — Задание допущенной попытке

Закреплённое сообщение одной Attempt: identities, определения, контекст, ограничения и основания допуска. Передаётся исполнителю, в F1 через stdin. Не равно RunBrief или prompt; не выдаёт права на произвольные будущие tool arguments. В текущем Go хранится как JSON, отдельный одноимённый struct не введён.

<a id="evidence"></a>
### Evidence — Свидетельство проверки

Запись, поддерживающая конкретное утверждение о конкретном предмете, с методом, происхождением, результатом и ограничениями. «Процесс завершился с кодом 0», «schema валидна» и «внешнее действие действительно произошло» — разные утверждения. Любой artifact не становится evidence автоматически; prose исполнителя не является независимым доказательством.

<a id="check-execution"></a>
### CheckDefinition / CheckExecution — Определение проверки / исполнение проверки

CheckDefinition задаёт kind, exact method/executor, ожидаемый claim и закрытый request/result contract автоматической проверки порта либо результата. Конкретные subjects определяет owning boundary и закрепляет CheckRequest. CheckExecution — отдельная ограниченная sub-operation того же Run с собственными identity, admission, процессом и settlement. Она не повторная Attempt производителя и не Stage вне WorkflowRevision. Обычный StepDefinition.kind=check остаётся полноценным workflow шагом; его название не включает автоматическое выполнение из content_check_refs.

Каждый CheckExecution расходует один control transition во всех owning scopes; StepInstances не увеличиваются фиктивно. Pending acceptance удерживает непринятый candidate и sealed subjects до обязательных checks. Отрицательное либо inconclusive свидетельство не меняет прежний producer verdict и не разрешает скрытый repair. Для этих новых records требуется отдельная версия состояния, а не расширение прежних snapshots.

CheckRequest/CheckResult — собственные закрытые сообщения проверки. Это не ExecutionEnvelope/StepResult: checker не становится StepInstance и не получает публикационный интерфейс производителя. Request digest связывает результат с точными переданными bytes, а не с повторно отформатированным JSON. Process settlement и `pass|fail|inconclusive` отчёта — разные величины.

<a id="pending-acceptance"></a>
### PendingAcceptance — Ожидание обязательной проверки

Сохранённая граница приёмки точных входов, выходов либо candidate StepResult внутри одной WorkflowInvocation. Содержит неизменяемые bindings/subjects, список обязательных проверок из закреплённого определения и состояние `pending|passed`. Подготовленные выходные bytes сохраняются, но до всех положительных проверок не публикуются как принятые ArtifactRevision metadata. `passed` не подменяет отдельное принятие StepResult/экспорта и не разрешает продолжить отменённый Run.

Четыре kinds — workflow_input, step_input, step_result, workflow_output; step_result объединяет output content checks и result checks. Workflow-input boundary не создаёт StepInstance/StageActivation. Go `PendingCheck` — элемент списка этой границы: будущая CheckExecution identity, exact definition, boundary/port и subjects; не самостоятельный scheduler или новый вид исполнения. Проверенные input/projection refs сохраняются после restart, а не вычисляются заново. `EvidenceRef` — соответствие прежней baseline ссылки `id,digest`; оно не заменяет ArtifactRef и не означает положительный исход само по себе.

<a id="registry"></a>
### Registry / Package / Manifest — Реестр, пакет, манифест

**Registry** — каталог доступных закреплённых определений; в F1 локальный `flow.Registry` не скачивает отсутствующие зависимости. **Package** — версионированный набор определений и ресурсов; полный lifecycle пакетов и доверия входит в F2. **Manifest** — структурированная опись состава или связей; нужно уточнять, какой именно: context, dependency lock, build manifest. Исторический `file-manifest.json` не является manifest нынешнего binary release.

<a id="release-manifest"></a>
### ReleaseManifest — Манифест публичной поставки

**ReleaseManifest** — отдельный `prifly-release/1` JSON public release: version,
поддержанные OS/architecture и digest одного archive на platform. Его detached
Ed25519 signature проверяется public key конкретного binary. Это не
PackageManifest, не authority record и не доказательство качества workflow;
archive digest, manifest bytes и signature остаются тремя разными величинами.

<a id="sealed-package"></a>
### SealedPackage / PackageComponent / TrustDecision — Закреплённый пакет, его компонент и решение доверия

**SealedPackage** — проверенная копия локального пакета под неизменяемым каталогом установки, названным по его identity. Импорт разделяет получение, проверку и регистрацию: ни один байт пакета не исполняется, lifecycle scripts и инструкции README не выполняются. Необъявленный в манифесте файл отклоняет импорт целиком, а не игнорируется.

**PackageComponent** — объявленный экспорт пакета, разрешаемый как обычная запись реестра по exact ref. Его bytes проверяются по digest при каждом разрешении, поэтому изменённый после импорта файл перестаёт разрешаться, а не подменяет закреплённое содержимое. Компоненты, расширяющие policy, trust или исполнение (`policy`, `tool`, `adapter`, `redaction_profile`), в этой поставке отклоняются.

**TrustDecision** — записанное authority решение принять exact manifest digest, связанное с ControlIntent операции `package.trust` и committed ControlAdmission. Та же пара id/version с другими bytes является конфликтом, а не обновлением: прежнее решение не наследуется. Локальное решение владельца не является внешней подписью или provenance-доказательством; `origin.location` — заявление импортирующего, а не проверенный факт.

<a id="worktree-claim"></a>
### WorktreeClaim / Workspace mode — Исключительный claim и режим рабочей области

Записанное authority владение одной Git рабочей областью: resource identity
репозитория, path, проверенный base commit, generation и владелец. `mode=worktree`
означает confined disposable directory внутри authority workspace root;
`mode=checkout` — явно выбранный текущий canonical checkout repository. Claim
является authority-состоянием, а не lock-файлом внутри репозитория, поэтому
оборвавшаяся сессия оставляет запись с известным владельцем, а не безымянный
каталог. Репозиторий, пересекающийся с authority project root, не может быть
заявлен.

Claim фиксируется durable до создания worktree: прерванная подготовка оставляет
`preparing`, который можно очистить, а не неатрибутируемый каталог. Release и
cleanup `worktree` записываются вместе, но удаляется только каталог с той
device/inode identity, которую создал именно этот claim; подменённый каталог
блокирует удаление. Cleanup `checkout` снимает только claim и никогда не
меняет Git topology, branch, HEAD или файлы. `checkout` допускается лишь из
чистого repository и сохраняет ту же physical exclusivity. Устаревшая
generation не удаляет ресурс более позднего владельца. Один claim на
репозиторий — граница этой поставки, а не планировщик: fairness queue,
parallel и remote claims не введены.

<a id="assisted-session"></a>
### AssistedSession / SessionHandoff — Ассистируемое исполнение и передача работы хосту

**AssistedSession** — способ исполнения шага сессией, которая существует до Run и не запускается этим authority. Ядро не порождает её, не сигналит ей и не владеет каналом к ней, поэтому adapter `core:adapter/assisted-session` объявляет `process_control=host` и не обещает fresh context. Шаг выбирает этот способ своим StepDefinition, а не конфигурацией проекта; закреплённый contract конфигурации не меняется.

**SessionHandoff** — записанный в Run факт передачи попытки: удостоверённый
principal, exact pinned skill refs, claimed Workspace с его generation и mode,
срок и состояние хоста. `SessionTask.workspace` остаётся authority scratch для
sealed context и output slots. Только task ассистируемого `workspace_write`
получает отдельно `repository_workspace` exact claimed Workspace; host меняет
код лишь там. В `assisted-session/4` task дополнительно сообщает declared
Workspace tree bindings; для output-only дерева host возвращает только typed
разрешённую location, а не digest, manifest или ArtifactRef. Хост доказывает,
какую работу держит, возвращая точные attempt
identity и envelope digest, а не предъявлением токена. Отсутствие отчёта
является собственным состоянием `disconnected` и переводит Run в `uncertain`;
оно никогда не становится успехом и не запускает автоматический повтор,
потому что эффект в claimed Workspace мог остаться.

Закрытие ассистируемой попытки опирается только на сам отчёт и объявленные outputs. Факты процесса — код возврата, время выхода, identity процесса — у неё отсутствуют и не подставляются: `ProcessOutcome` и `ExecutorEnd` остаются пустыми, потому что отчёт доказывает ответ хоста, а не остановку его сессии.

Допустимость отчёта отличается от бюджета исполнения. Бюджет выдаёт время работы и потому требует общего квалифицированного clock domain; работа хоста этим authority не ограничена, а его отчёт всегда приходит из другого процесса. Поэтому срок handoff решает только, принимается ли отчёт, и сравнивается по записанному UTC, а `deadline_trust` называет часы, на которых этот предел держится. Смена локальных часов сдвигает его: это заявленное ограничение, а не квалифицированная гарантия.

Состояние `core-state/5` существует ради этих фактов: прежние bundles остаются неизменными, а Run не утверждает dispatch, за который никто не отвечает.

<a id="package-lifecycle"></a>
### PackageStatus — Удаление, карантин и отзыв пакета

**Removed** закрывает пакет для нового разрешения и отклоняется, пока незавершённый Run его держит: uninstall не ломает работу в полёте. **Quarantined** так же прекращает разрешение, но обратим. **Revoked** дополнительно блокирует новые admissions там, где Run уже закрепил его bytes: в этом и состоит смысл security revocation, поэтому он допускается и при живых держателях.

Ни один из статусов не удаляет bytes: они остаются для прогонов, которые их закрепили, и для evidence. Отзыв терминален — ни смена статуса, ни повторный импорт тех же bytes не возвращают доверие, иначе отзыв был бы предложением, а не решением. Факты уже выполненных внешних действий не стираются.

`package verify` перечитывает каждый запечатанный байт и **сообщает** расхождение, никогда его не исправляя: починка заменила бы доказательство догадкой. Две одновременно разрешаемые версии допустимы; два разрешаемых пакета, экспортирующих одну component identity, — нет.

<a id="trust-root"></a>
### TrustRoot / PackageSignature — Корень доверия и открепляемая подпись

**TrustRoot** — записанный authority ключ, которому эта установка согласна доверять. Он никогда не приходит вместе с пакетом: ключ внутри пакета не назначает себя доверенным, поэтому проверка смотрит только на записанные корни. Отзыв корня не переписывает уже принятые решения — допуск состоялся.

**PackageSignature** — открепляемое утверждение об одном manifest digest. Оно доказывает, **кто запечатал** эти bytes, и ничего не говорит о качестве пакета или о праве этой установки его исполнять. Три величины остаются раздельными: hash архива, digest манифеста и предмет подписи; совпадение hash архива не доказывает ничего о подписи манифеста.

Если установка записала хотя бы один корень, пакет без подписи отклоняется, а не принимается как «локально доверенный»: наличие требования не смягчается его невыполнением.

Архив читается как данные, а не как указание, куда писать: traversal, absolute path, link, device и коллизия нормализованных имён отклоняют архив целиком **до** создания чего-либо, поэтому частично распакованное дерево не возникает.

<a id="resource-lease"></a>
### ResourceLease / ClaimProcess — Срок владения и опознание владельца

**ResourceLease** ограничивает, как долго владелец считается присутствующим. Его истечение переводит владение в `suspected`, но **никогда не освобождает ресурс**: отсутствие heartbeat не является доказательством, что прежний владелец остановился. Конкурирующий claim в этом состоянии блокируется отдельным кодом, а не получает ресурс, иначе живой старый процесс и новый владелец существовали бы одновременно.

**ClaimProcess** опознаёт владельца не по одному PID: записываются PID, clock session и время старта, поэтому переиспользованный номер не даёт позднему процессу унаследовать чужой claim. Продлить срок может только владеющий процесс.

Отношение конфликта — физический ресурс, а не путь, который набрал вызывающий: символическая ссылка и другой путь к тому же репозиторию являются одним ресурсом и не создают второго владельца.

<a id="control"></a>
## Управление и права

<a id="project"></a>
### Installation / Project / Workspace — Установка, проект, рабочая область

**Installation** — установленный экземпляр Pri-Fly и его локальные данные. **Project** — область конфигурации и ресурсов; Git необязателен. **Workspace** — выделенная область работы исполнителя с проверяемым владельцем. Эти корни имеют разные назначения; рабочая папка шага не должна считаться хранилищем принятых решений ядра.

Оговорка нынешнего F1: `runtime.Installation` — метаданные `.prifly/installation.json` конкретного project root, а не описание общего местоположения бинарника. Его `ID` совпадает с `StoreInfo.AuthorityID`; `ProjectConfig.ID` — отдельная identity проекта. Такое соответствие реализации не делает Installation, Project и Authority синонимами.

<a id="managed-binary-installation"></a>
### Managed binary installation — Управляемая установка binary

**Managed binary installation** — user-space executable `prifly` рядом с
receipt `prifly-managed-install/1`, созданный official bootstrap. Receipt
разрешает только explicit binary update и не является `runtime.Installation`,
project `.prifly/installation.json`, Package или authority. Source build и
скопированный binary не получают это право.

<a id="authority"></a>
### Authority — Источник управляющих решений

Единственный источник принятых допусков и состояния в заданной области. Не человек, не PID driver и не произвольная копия БД. В F1 это локальная управляющая область с SQLite и собственной identity; копирование её файлов не создаёт право параллельно продолжать исполнение.

<a id="principal"></a>
### Principal / Actor — Участник и автор команды

**Principal** — удостоверенный человек или сервис. **Actor** — удостоверенный участник, от имени которого обрабатывается конкретная команда или публикация. Значение `actor_id` из непроверенного payload не назначает полномочия; publisher шага не получает права администратора Run.

<a id="command"></a>
### Command — Команда

Явный запрос изменить управляемое состояние или выполнить разрешённую операцию. Имеет собственную identity и проверяемый payload. Команда — намерение, событие — уже принятый факт. `validate`, `preview`, `next` и чтение истории не превращаются в допуск только потому, что показали готовность работы.

<a id="command-receipt"></a>
### CommandReceipt — Подтверждение решения по команде

Сохранённый результат или отказ по удостоверенной команде, связанный с её ID и digest запроса. Exact retry при действующем праве чтения возвращает исходное решение; другой payload с тем же ID — конфликт. Receipt не доказывает успешного внешнего эффекта. Текущее Go-имя — `local.Receipt`; наличие подтверждения приёма не означает завершение всего Run.

<a id="admission"></a>
### ExecutionAdmission / Admission — Допуск попытки / операции

**ExecutionAdmission** разрешает конкретную Attempt с закреплёнными ограничениями и ресурсами. **Admission** в полном протоколе относится к одной конкретной logical operation. Это разные области полномочий: запуск worker не разрешает любые действия, которые он затем предложит. Полные managed operations и их ActionDelivery входят в F2.

<a id="action-intent"></a>
<a id="action-admission"></a>
<a id="action-delivery"></a>
### ActionIntent / ActionAdmission / ActionDelivery — Намерение, допуск и доставка операции

**ActionIntent** — неизменяемое точное предложение одной logical operation:
его operation identity, owning Attempt, tool contract, arguments, targets,
ожидаемые outputs, effect/retry class и deadlines. Оно само ничего не запускает
и не является Approval, Grant или ExecutionAdmission.

**ActionAdmission** — записанное authority решение для одного exact
ActionIntent. В Core state v19 оно хранит identity/digest intent и exact
ControlApprovals, которые consumed с ним в одной SQLite-транзакции. Core v20
дополнительно может consume ровно один `action.admit` Grant с закрытым exact
resource scope: каждый target ActionIntent должен входить в scope Grant. Grant
не отменяет Approval, если его требует policy. Отдельный Run до v19 не получает
эту запись, а v19 сохраняет approval-only поведение. ActionAdmission не является
ActionDelivery: он не выдаёт credential, не вызывает adapter и не доказывает
external effect.

**ActionDelivery** — отдельная durable запись передачи уже допущенной операции
конкретному executor. Core v21 пока сохраняет только исходное состояние
`prepared/not_started`: оно связывает operation, Admission, owning Attempt и
заявленный adapter до любого внешнего вызова. Это не credential, не dispatch,
не receipt и не доказательство external effect; delivery retry и reconciliation
остаются следующими отдельными переходами.

<a id="approval"></a>
### Approval / Grant / Policy — Согласование, делегированные права, политика

**Approval** — решение человека по точному предмету действия. **Grant** — заранее делегированные ограниченные полномочия с условиями и отзывом. **Policy** — проверяемые правила допустимости. Они не взаимозаменяемы; `required_capabilities` запрашивает возможности, а не выдаёт права. Полные approvals/grants относятся к F2; нынешнее подтверждение RunBrief не подменяет их.

<a id="control-approval"></a>
### ControlApproval / ApprovalVote — Согласование управляющего решения

**ControlApproval** — открытое решение по одному exact protected payload управляющей операции. Quorum и правило независимости замораживаются в записи при открытии, поэтому позднейшая смена policy не понижает задним числом то, что это решение требовало. Привязка идёт к digest защищённого payload, а не к документу ControlIntent: документ несёт срок, и его digest нельзя назвать заранее.

**ApprovalVote** — отдельный запечатанный artifact с решением участника. Независимость сравнивает `human_id` из enrolled principals authority, а не `actor_id` из payload: второй технический аккаунт того же человека не становится вторым согласующим. Отсутствие человека не превращается в approval, а срок сам по себе ничего не одобряет.

Потребление происходит внутри той же транзакции, что и защищённое изменение, и однократно. Consumed-решение терминально: его нельзя отозвать задним числом, потому что допуск уже состоялся. Истечение окна является вычисляемым состоянием: отказавшая команда не пишет состояние, поэтому `expired` показывается представлением, а не выдумывается как сохранённый переход.

Требование согласования для управляющих операций записано authority-side (`control-approval-policy/1`), потому что опубликованный PolicyRevision описывает approval для effect classes, а не для control operations. Установка с одним владельцем не может честно собрать независимый quorum и отказывает.

<a id="control-grant"></a>
### ControlGrant — Ограниченная делегированная авторизация

Решение, принятое один раз для конечного числа последующих применений: capabilities, resource scopes, срок и предел логических операций. Constraints запечатаны отдельным immutable artifact, изменяемым остаётся только учёт, поэтому Grant не может расширить сам себя.

Grant не отменяет требуемое согласование: выдача сама является управляющей операцией и гейтится той же policy, поэтому человеческое решение переносится на выдачу, а каждое применение только расходует предел. Grant также не выдаёт прав, которых у subject нет: отсутствие текущего object access отклоняет выдачу.

Расход происходит в той же транзакции, что и защищённое изменение, и записывается вместе с admission. Истечение и исчерпание являются вычисляемыми состояниями по тем же причинам, что и у [ControlApproval](#control-approval). Отзыв закрывает будущее применение и не переписывает уже состоявшиеся: их допуски уже произошли.

<a id="quality-waiver"></a>
### QualityWaiver — Записанный отказ требовать проверку

Явный отказ требовать одну **названную** quality check instance. Waiver не превращает проверку в `pass`, не создаёт отсутствующий artifact и не отменяет другие проверки: провал соседней остаётся провалом. Он покрывает ровно тот check instance, который назвал — не соседний, не тот же check в другом StepInstance и не boundary без step.

Проверки осмысленности (`identity`, `authorization`, `input_integrity`, `resource_confinement`, `evidence_consistency`) находятся вне досягаемости waiver полностью: это code paths ядра, которые отклоняют, а не package checks, которые декларация могла бы переименовать в optional. Обращение к ним отклоняется явным кодом, чтобы граница была заявлена, а не оставлена совпадению.

Снижение качества остаётся видимым в итоге: объявленный `succeeded`, опиравшийся на waived проверку, сообщается как `completed_with_waivers`. Waiver не повышает никакой другой outcome до успеха. Worker не вправе сам вернуть `waived`.

Снижение принадлежит области, которая на него опиралась. Waiver называет покрытый step instance, поэтому сниженный исход получают эта invocation и её предки, а не каждая область запуска; соседняя ветвь чужого послабления не наследует. `completed_with_waivers` — объявляемый исход Core, а не второй вид успеха: сценарий, способный опереться на waived проверку, обязан объявить его, иначе завершение отклоняется как `undeclared_waived_outcome`. Простой успех вместо снижения скрыл бы его, а необъявленный исход положил бы в результат значение, которого контракт не выражает.

Waiver действует только в Run состояния `core-state/6`: прогон, начатый раньше, сохраняет свою закреплённую семантику и отклоняет waiver, а не переинтерпретируется задним числом.

<a id="object-read"></a>
### ObjectRead — Право чтения записей

Текущее object access на чтение вычисляется **до** выбора записей, поэтому читатель без права не влияет на подсчёты и не получает cohort, страницу или receipt. Наличие command identity, cursor или Run ID не доказывает права читать их. Cursor привязан к principal и access scope и перепроверяется на каждой странице.

Установка, никогда не записывавшая control plane, даёт чтение проверенному владельцу ОС — ровно то, что уже проверено при открытии. Как только plane существует, решает записанный object access. Пути чтения не выполняют enrolment и не восстанавливают права: отказ наблюдается против состояния как оно записано.

<a id="policy-bundle"></a>
### PolicyBundle — Закреплённый набор правил

Версионированное immutable определение допустимых modes, effects, проверок и ограничений, выбранное exact `policy_ref`. Workflow limits могут только сужать его пределы; дочерняя invocation дополнительно ограничена ancestors. PolicyBundle не выдаёт Grant и не снимает текущие Stops.

P2-03 вводит `core:policy/local@2.0.0` с child depth 8; exact v1 с depth 0 остаётся неизменной. Default F1 — v1, нового Core — v2. Core принимает обе revisions и применяет выбранные policy caps; default не переопределяет закреплённые Runs.

<a id="stop"></a>
### Stop — Управляющий запрет

Сохранённое ограничение с identity, поколением, причиной и автором. Это объект управления, а не синоним завершения Run. Наложение запрета монотонно ограничивает новую работу; снятие требует проверки актуального состояния и конкретного Stop.

В P2-03 `scope=invocation` с exact `scope_id` относится к указанной invocation и потомкам, `scope=run` — ко всему Run. ControlEpoch остаётся общим; локальный stop не отменяет ancestor stop или Run-wide unknown barrier.

<a id="pause"></a>
### Pause — Приостановка продвижения

Запрещает новые обычные admissions и дальнейшее продвижение. Работа, уже прошедшая dispatch boundary, может завершиться и отдать результат. Не означает заморозку процесса ОС или сохранение его стека для последующего продолжения.

<a id="cancel"></a>
### Cancel — Запрос отмены

Запрещает продолжение, запрашивает остановку принадлежащей runner работы и требует учесть её последствия. Подтверждение команды ещё не означает terminal `cancelled`. Отмена не откатывает уже произошедшие внешние действия.

У дочернего вызова terminal cancelled закрывает только подтверждённо остановленное поддерево. Независимые будущие siblings не отменяются; caller ждёт с причиной `blocked_child`. Это не child outcome, не `on_error` и не разрешение повторить прежнюю работу. В P2-03 явная отмена Run позволяет завершить ожидающее дерево.

<a id="authority-control"></a>
### AuthorityControl — Управляющая область authority

Отдельная от Run область состояния одного authority: удостоверённые principals, их object access, выбранная policy и управляющие Stops. Установочный или проектный запрет является объектом управления, поэтому его наложение и снятие не создают фиктивный Run, StepInstance или worker. В P2-06 область умышленно узкая: approvals, quorum, grants и waivers добавляются отдельными срезами и не подразумеваются наличием этой записи.

<a id="session-principal"></a>
### SessionPrincipal / ObjectAccess — Principal сессии и доступ к объекту

**SessionPrincipal** — [Principal](#principal), выведенный из удостоверённого владельца процесса, а не из payload команды; в local profile это текущий OS UID установки. **ObjectAccess** — разрешённые операции конкретного principal над конкретным объектом (`installation`, `project`). `human_id` отделяет человека от технического аккаунта, поэтому второй аккаунт того же человека не становится вторым независимым согласующим. Отсутствие ObjectAccess даёт отказ, а не молчаливое расширение прав.

<a id="control-pin"></a>
### ControlPin — Привязка допуска к проверенной версии управления

Ссылка команды на точную версию AuthorityControl, против которой её допуск был вычислен вне транзакции. Stop, зафиксированный в промежутке, делает такую команду отклонённой, а не обгоняемой. Pin входит в digest запроса, поэтому повтор с прежним `command_id` после смены управления является конфликтом; исходное решение остаётся читаемым через его receipt. Это не CAS версии Run и не ControlEpoch.

<a id="release-stop"></a>
### ReleaseStop — Снятие конкретного запрета

Снимает перечисленные Stops по точным identities/generations и ожидаемому ControlEpoch. CLI использует `run release`, Go — `Engine.Release`. Это не выпуск версии продукта, не автоматическое продолжение работы и не обратная операция к необратимой отмене.

<a id="resume"></a>
### Resume — Разрешённое продолжение

После проверки текущих условий разрешает продолжить ещё не начатую часть Run. Не снимает Stops вместо ReleaseStop, не перезапускает прежнюю Attempt и не превращает неопределённый исход в известный. В F1 `release` и `resume` — отдельные действия.

<a id="generation"></a>
### ControlEpoch / Generation / Fencing — Версия ограничений, поколение, отсечение устаревшего владельца

**ControlEpoch** — монотонная версия управляющих ограничений Run; в F1 меняется при принятии restrict/release. **Generation** относится к конкретному объекту или источнику, например Stop, Attempt или clock session; общего счётчика всех поколений нет. **Fencing** отклоняет действие устаревшего владельца там, где исполнитель или целевой ресурс проверяет соответствующее поколение. Проверки core API в cooperative F1 не обещают универсального fencing внешних эффектов или контроля любой программы под тем же OS UID. `control_epoch`, storage authority `epoch`, `run_version` и строковая telemetry generation не являются одной величиной.

<a id="state"></a>
## Состояния, hooks и реакции

<a id="status"></a>
### Status — Состояние сущности

Положение конкретного Run, StepInstance, Attempt или другого объекта в его жизненном цикле. Всегда уточняем сущность. `completed` не означает положительный предметный результат. В телеметрии status команды `accepted/rejected` также нельзя читать как lifecycle status Attempt.

<a id="verdict"></a>
### Verdict — Предметный ответ шага

Типизированный ответ принятого StepResult, по которому применяется объявленный маршрут. В F1 routing предусмотрены `pass`, `fail`, `needs_revision`, `no_work`. Корректно выполненная проверка может дать `fail`: это не то же, что техническая ошибка исполнения `failed`.

<a id="outcome"></a>
### Outcome — Итог сценария

Принятый итог workflow, отдельно от lifecycle status и verdict отдельного шага. В F1 `finish` поддерживает `succeeded`, `rejected`, `no_work`; P2-03 добавляет `partial` для Core state v2, включая flat workflows. Partial требует объявленного контракта и известного результата; unknown, missing required output и незавершённая обязательная работа им не маскируются. Completed child возвращает workflow outcome, не StepResult verdict. Waivers — отдельная возможность P2-06; `completed_with_waivers` объявляется наравне с прочими исходами Core и приписывается области, опиравшейся на послабление.

<a id="diagnostic"></a>
### Diagnostic / Severity — Диагностическая запись и серьёзность

Структурированное наблюдение с устойчивой identity, источником, кодом, scope и severity. Severity, например `warn` или `error`, не является verdict. Повтор доставки одной occurrence не должен считаться новой причиной; произвольная строка `ERROR` в stdout не становится доверенной Diagnostic.

Core Diagnostic может хранить `stage_activation_id` до появления Attempt. Ошибка подготовки шага остаётся связанной с StepInstance и находится по `step_id` в телеметрии. Ошибка control stage, например связывания outputs на `finish`, имеет subject `stage_activation` и `stage_id`, без фиктивных StepInstance/Attempt. F1-only cohort и старые cuts сохраняют calculator `foundation-telemetry/1`; выбранная cohort с Core Runs использует `core-telemetry/1`, сохраняя исходные F1 records и их descriptor revisions.

<a id="hook"></a>
### Hook — Объявленный канал публикаций шага

Именованный выходной контракт StepDefinition: kind, schema, права чтения и пределы публикаций. Автор шага определяет его заранее, ядро закрепляет декларацию. В Pri-Fly это данные и проверяемый контракт, **не callback-функция внутри ядра**. F1 поддерживает `state/event`; artifact/close и подписки относятся к F2.

Первый срез P2-12 добавляет в Core `artifact` hook через StepDefinition v3:
формат JSON/blob, `one|keyed_many`, явный список content checks и разрешение
раннего потребления. Непустой список checks в Run с `core-configuration/2`
создаёт отдельную проверяемую pending границу: Authority проверяет sealed copy
каждого item до ArtifactPublication; `fail`/`inconclusive` не публикует item и
не меняет verdict producer. Подписки не следуют из наличия hook. Следующий
одноразовый срез
добавляет StepDefinition v4: только artifact hook может выбрать
`read_policy=declared_subscribers`; v3 остаётся `owner` и не открывается
workflow, который лишь знает его ref. Foundation и StepDefinition v2 сохраняют
только прежние `state/event` формы. `keyed_many` hook новых Runs допускает
отдельный explicit close через команду v3; это закрывает producer collection,
но само по себе не создаёт подписчика.

<a id="publication"></a>
### Publication — Принятая публикация hook

Конкретная запись в объявленном канале. `state` полностью заменяет своё значение по scoped CAS; в F1 версия ведётся отдельно для пары Attempt/hook. `event` фиксирует occurrence со стабильным `event_key` и дедупликацией в области StepInstance/hook. Публикация состояния `phase=finished` не меняет lifecycle StepInstance и не заменяет StepResult. Путь к файлу в payload также не делает файл принятым ArtifactRevision.

<a id="artifact-publication"></a>
### ArtifactPublication — Принятая публикация артефакта

Неизменяемая запись одного logical item artifact hook: ключ
`(StepInstance, hook, item_key)`, исходная producer Attempt, запечатанная
ArtifactRef, type/schema, evidence, classification, authority sequence/time и
разрешённая пригодность. Для cardinality `one` ядро использует фиксированный
`item_key=item`; для `keyed_many` имя задаёт producer и повторить его с другими
bytes нельзя. ArtifactPublication может быть принята до terminal producer, но
не является final `stage_output`, успехом Attempt или доставкой consumer.

Готовность объявляет durable ArtifactPublication, а не наличие blob/metadata в
artifact store: подготовленные bytes могут остаться ничем не связанным orphan,
если повторная проверка authority или SQL commit отказали. Exact command либо
уже принятый logical key возвращает прежнюю запись без перечитывания mutable
candidate; новая запись требует current unfenced publisher authority.

Если hook объявил content checks, между sealing и ArtifactPublication существует
ровно один **PendingArtifactPublication** — закрытая внутренняя граница с
identity item, sealed ArtifactRef и exact CheckExecution IDs. Она не является
Publication, не выдаётся subscriber-у и исчезает как после всех `pass`, так и
после отказа check. Evidence passing checks входит в получившийся
ArtifactPublication; failed candidate остаётся только наблюдаемостью checker,
не скрытым repair либо изменением producer verdict.

<a id="artifact-manifest"></a>
### ArtifactManifest — Точный состав закрытого artifact hook

Запечатанный authority JSON `artifact-manifest/1` со всеми принятыми
ArtifactPublication одного `(StepInstance, hook)` в порядке authority sequence,
их количеством и cut последнего item. Пустой manifest хранит явный `items=[]` и
cut 0. Count без exact membership не является manifest. ArtifactRef manifest
указывает на immutable bytes в artifact store; mutable producer path в него не
попадает. Полный набор зависимостей находится в typed `items`, а не дублируется
в ограниченный 256 ссылками `ArtifactRevision.provenance`.

<a id="artifact-closure"></a>
### ArtifactClosure — Принятое закрытие artifact hook

Durable факт `artifact-closure/1`, который связывает `keyed_many` hook с одним
точным ArtifactManifest. Logical identity — `(StepInstance, hook)`; повтор с тем
же упорядоченным membership возвращает прежнюю запись, другой membership
конфликтует. После ArtifactClosure новые items этого hook не принимаются. Close
не завершает Attempt, не создаёт final output и без assignment не двигает
RunVersion. До появления stream lowering это ещё не доставка `Closed`
consumer-у и не доказательство отсутствия interruption.

<a id="publish-step-publication-command"></a>
### PublishStepPublicationCommand — Запрос публикации hook

Удостоверяемая команда `step.publish` с identity попытки, именем hook, payload и условиями проверки своего scope. Go-имя — `runtime.PublishCommand`. Это запрос, который может быть отклонён. Он не является принятой Publication и не даёт разрешения публиковать в чужой канал. Поле `expected_state_version` относится к собственному state hook, а не RunVersion.

Версия 2 сохраняет state/event variants и добавляет artifact variant: confined
workspace-relative `candidate_path`, точные `expected_digest` и
`expected_size_bytes`, а для `keyed_many` — `item_key`. Candidate path не
сохраняется как ArtifactRef и не становится доступом к произвольному файлу.
Версия 3 сохраняет эти variants и добавляет `kind=close` с обязательным, включая
пустой, упорядоченным `item_keys`. Authority принимает close только при exact
совпадении со всеми items канала. Версии 1/2 и прежние public bundles остаются
закрытыми: v1 не принимает artifact, обе не принимают close.

<a id="subscription"></a>
### Subscription — Подписка

Сохраняемая связь с разрешённым источником публикаций и правилами
реакции/доставки. В F1 не реализована. Узкий Core-путь `once` связывает direct
sibling producer и обычный wait; существующие WaitRegistration, InboxEvent и
`event_ref` сохраняют одну доставку без отдельного cursor. Режим
`each_publication` получает собственную durable PublicationSubscription на
каждую direct repeat activation: typed handle, generation, cursor и pending
assignment. Открытый CLI, знание ArtifactRef или совпадение имени события сами
по себе не создают подписку и не запускают другой шаг. Generic внешние
`subscriptions` остаются unsupported.

<a id="publication-source-definition"></a>
### PublicationSourceDefinition — Источник публикаций

Закрытое immutable-определение связи. Версия `publication-source/1` выбирает
один retained item для `once`; `publication-source/2` выбирает retained
`each_publication`, exact schemas handle/cursor/delivery и весь stream.
Версии `/3` и `/4` повторяют эти формы с initial `new_only`: Authority хранит
свой event-sequence cut при регистрации и читает только публикации после него.
Версии `/5` и `/6` отдельно выбирают `interrupt_on_terminal_failure`: только
terminal failed producer invocation немедленно направляет once consumer по его
declared failure route, а stream получает tagged `Interrupted` delivery с
причиной `producer_terminal_failed`. Обработанная `on_error` ошибка producer
не является terminal failure и этот путь не запускает.
Версии `/7` и `/8` отдельно объявляют exact delivery type: `json` с pinned
schema либо `blob` с ровно одним declared media type. Blob hook schema остаётся
descriptor contract producer-side sealing; blob consumer port принимает exact
format/media sealed bytes и не обязан повторять descriptor schema ref.
Все версии называют sibling producer branch, direct step, artifact hook/schema,
initial read и конечное ожидание при отказе producer. Source не выдаёт право
сам: hook обязан разрешить `declared_subscribers`, а compiler проверяет точную
parallel-композицию и объявленную одновременность. Go —
`flow.PublicationSourceDefinition`.

<a id="publication-cursor"></a>
### PublicationCursor — Курсор публикационной подписки

Authority-owned позиция следующей ещё не обработанной доставки конкретной
Subscription generation. Первое тело repeat получает позицию 0 вместе с
handle; следующее тело получает `next_cursor` из результата предыдущей
итерации. Cursor не является EventSequence, ReadCut или клиентским offset и не
двигается при одной лишь публикации: pending PublicationAssignment должна быть
урегулирована телом repeat.

<a id="publication-assignment"></a>
### PublicationAssignment / PublicationDelivery — Назначение и доставка публикации

**PublicationAssignment** — durable ledger record одного cursor конкретного
subscriber: `Item`, `Closed` либо `Interrupted`, exact delivery ArtifactRef,
body/wait identities и состояние `assigned|processed`. Pending assignment
остаётся тем же при новых публикациях и повторном продвижении driver; следующий
item его не подменяет.

**PublicationDelivery** — immutable tagged JSON, которое читает wait/choice.
`Item` содержит ArtifactPublication и exact sealed ArtifactRef, `Closed` — exact
ArtifactClosure/manifest, `Interrupted` — причину без item или closure. Closed
и Interrupted расходуют отдельную iteration, но только Item вызывает consumer
call. Ledger не обещает exactly-once внешний эффект consumer-а.

<a id="guard"></a>
### Guard — Защитное условие

Применяет условие к допустимости действия, например старта или продолжения работы. Не произвольный script/eval и не решение модели внутри ядра. Live start/stop conditions и реакции на источники состояния относятся к P2-12; реализация snapshot Choice в P2-02 не включает их автоматически.

<a id="predicate"></a>
### Predicate — Выражение условия

Закрытый AST над разрешёнными данными: `eq`, `ne`, `exists`, `all`, `any`. `eq/ne` сравнивают одинаковые scalar types: string, boolean, null или математически целое ±9007199254740991; `1`, `1.0`, `1e0` равны как числа, строка `"1"` имеет другой тип. Missing даёт unknown; несовместимые типы и некорректные данные дают ошибку, не false. `exists` проверяет presence любого допустимого JSON value, поэтому присутствующие null/object/array дают true.

Пределы P2-02: depth до 16 с root на уровне 1; до 256 Predicate operator nodes суммарно по всем branches одного Choice. Operand и FieldRef — данные, не дополнительные operator nodes. Отдельный предел сериализованных FieldRefs — 256 KiB по всем occurrences, включая повторы: полный trace должен помещаться в journal без усечения. `all/any` обходят args в порядке определения до первой ошибки: ранний boolean результат не скрывает позднюю ошибку. Полные truth tables и правила проверки — в [сценариях, пакетах и контексте](../workflow-and-context/spec.md) и [CLI protocol](../cli-protocol/spec.md).

<a id="truth-value"></a>
### Truth value — Значение истинности

Результат корректно вычисленного Predicate: `true`, `false` или `unknown`. Unknown означает отсутствие достаточных данных, а не ошибку исполнения, отмену или uncertain внешний эффект. `error` и `not_evaluated` описывают ход вычисления ветви в trace и не являются дополнительными truth values. Go — `flow.Truth`.

<a id="history"></a>
## История и телеметрия

<a id="verified-cut"></a>
### Verified cut — Проверенная отметка

Отметка в самой authority о том, до какого cut её содержимое уже было проверено. Открытие проверяет записанное после этой отметки и двигает её; полная проверка всей базы остаётся отдельной операцией `doctor`. Отметка — утверждение о прошлой проверке, а не о неизменности: повреждение уже проверенных страниц ловит `doctor`, а не каждое открытие.

<a id="pinned-bytes"></a>
### Pinned bytes — Разделяемые закреплённые байты

Байты, которые Run закрепил при создании и которые не меняются за его жизнь: определения, ресурсы контекста, envelope допущенной попытки. Они хранятся один раз по digest, а снимок и событие ссылаются на них. Логическая форма Run и его digest от этого не меняются: восстановление возвращает ровно те байты, которые были записаны, и отсутствие разделяемых байт — это отказ по целостности, а не пустое поле.

<a id="event"></a>
### RunEvent / Journal / Snapshot — Событие, журнал, снимок

**RunEvent** — принятый факт ядра в истории одного Run; Go — `local.Event`. **Journal** — упорядоченная сохраняемая история. **Snapshot / projection** — состояние, полученное из этих фактов для чтения и проверки. Publisher event hook и событие журнала связаны, но не являются одним контрактом. Редактирование экспортированного JSON не меняет управляющее состояние.

<a id="repeat-decision"></a>
### RepeatDecision — Запись решения после итерации

Факт ядра `repeat-decision/1` в `stage.repeat_decided`: owning Run/Invocation/StageActivation, exact workflow/body identities, iteration, body status/outcome, until result или not_evaluated/error, прочитанные refs/pointers/presence, route и successor либо failure, commit Observation. Current state хранит только последний decision; каждый прежний остаётся в history. Continue decision создаёт следующий body и увеличивает count атомарно; worker admission выполняется отдельно.

Это control record, не StepResult, DecisionArtifact worker или новое разрешение на внешние эффекты. Trace использует существующий ChoiceInput shape без нового дублирующего типа: `availability=present|absent|unavailable`, exact source ref и producer при наличии. Body identity дополнительно фиксирует контекст iteration_output. Until error не маскируется unknown; timestamp Observation не является входом условия.

<a id="choice-decision"></a>
### ChoiceDecision — Запись решения о ветви

Версионированный факт ядра в `stage.choice_decided`: identity activation и точная WorkflowRevision, порядок и результат рассмотрения branches, использованные refs/pointers/presence, выбранный переход либо причина отказа. Trace различает вычисленные условия, ошибку и неисследованный хвост; не выдумывает прочитанные inputs. Исходный AST и branch order остаются в закреплённой WorkflowRevision. Решение сохраняется атомарно с переходом или отказом и после restart не вычисляется заново по новым данным.

ChoiceDecision не является предметным DecisionArtifact worker, StepResult или разрешением на действия. Для неё публикуется отдельный контракт `choice-decision/1`; прежние Run/state/read DTO не получают новых полей. `flow.ChoiceResult` — временный результат evaluator, ещё не сохранённое решение. Его `BranchEvaluation` содержит только завершённые оценки, а не весь trace.

`ChoiceEvaluation` — элемент записанного ordered trace ветвей. `ChoiceInput` — описание фактического чтения: FieldRef, exact source ArtifactRef и producer activation при наличии, `availability=present|absent|unavailable`. Отсутствующий pointer не теряет source ref. Unavailable означает отказ чтения/проверки, не optional absence и не truth unknown. Повторные чтения одного FieldRef не создают дубликаты trace; сохраняется порядок первого обращения. Observation фиксируется в transaction принятия решения и не участвует в сравнении повторно доставленной команды.
<a id="recovery"></a>
### Recovery / Replay — Восстановление / воспроизведение сохранённой истории

**Recovery** восстанавливает допустимое состояние работы после сбоя, сохраняя невыясненные исходы и текущие ограничения. **Replay** в Pri-Fly читает и проверяет принятые факты/проекции для восстановления состояния; не запускает worker заново и не повторяет внешние действия. Утрата необходимых bytes или неподдерживаемая версия reader остаётся явным ограничением, а не поводом придумать недостающее состояние.

В `core-workflow/1` известный технический отказ может потребляться объявленным `on_error`. Событие `stage.error_handled` сохраняет activation, Attempt при её наличии, код отказа и выбранную следующую стадию. Неудачный StepInstance остаётся failed; preparation failure до допуска не создаёт фиктивную Attempt. Cancellation и uncertain не являются обычными ошибками обработчика. Принятый verdict без `on.<verdict>` остаётся принятым результатом, но завершает Run с ошибкой маршрутизации; `on_error` не подменяет отсутствующий verdict handler.

<a id="resolution"></a>
### Resolution — Разрешение неопределённого обязательства

Явное решение владельца о судьбе обязательства, исход которого authority не смогла установить. Recovery сохраняет неопределённость и удерживает занятый slot; Resolution — единственный способ её снять, и снимает её не догадкой, а заявлением владельца о том, что внешний эффект был применён или не был. Она закрывает attempt или check как failed с записанным исходом и причиной, освобождает slot и пересчитывает признак незакрытых эффектов. Resolution не маршрутизируется объявленным `on_error`: обработчик описывает известный технический отказ, а не решение человека о неизвестном. Она отказывает, пока живой driver удерживает работу: пока владелец процесса на месте, судьба обязательства ещё может выясниться сама.

<a id="run-version"></a>
### RunVersion — Версия управляющего состояния запуска

Счётчик конкретного Run для проверки ожидаемой версии при изменении состояния — optimistic compare-and-swap (CAS). В F1 принятый управляющий переход повышает его на один; собственные публикации, отказы и samples его не повышают. Это не версия workflow, не номер события и не global cut.

<a id="event-sequence"></a>
### EventSequence — Номер события внутри запуска

Последовательность событий журнала одного Run. Одна команда может записать несколько событий, поэтому EventSequence не равен RunVersion. В текущих структурах используются `Seq` → `seq`, `EventSeq` → `event_seq` и `RunView.EventSequence` → `event_sequence`; это закреплённые имена разных представлений одного понятия, не три разных счётчика.

<a id="read-cut"></a>
### ReadCut — Фиксированная граница чтения истории

Общая для Authority граница сохранённых записей, на которой согласованно читаются snapshots, receipts и samples. В прежних текстах — `knowledge cut`, `fixed cut`; текущие Go-поля — `Cut`, JSON — `cut`, отдельного Go-типа ReadCut нет. Граница продвигается также при записи отказа, receipt-only публикации и нового sample batch, поэтому не равна RunVersion или EventSequence.

Чтение на прежнем cut не подмешивает более поздние факты. Cut не является timestamp, разрешением доступа или обещанием бесконечного retention. Exact retry возвращает старый Receipt.Cut, но диагностические samples нового обращения могут отдельно продвинуть текущую общую границу.

<a id="run-view"></a>
### RunView — Представление запуска для чтения

Согласованное представление состояния и времени с явными RunVersion, EventSequence и ReadCut. Public contracts — `FoundationRunView`, прежний Core `CoreRunView` и scoped `CoreRunViewV2`; это не baseline `RunSnapshot` v1. State/read v2 сохраняют настоящее invocation tree; старые Runs не переводятся на него автоматически. Разные версии read DTO нельзя объявлять взаимозаменяемыми из-за похожего содержимого.

<a id="observation"></a>
### Observation — Наблюдение часов

В текущем Go это запись времени: UTC timestamp, clock session, monotonic offset, источник и свойства часов. Не длительность и не произвольный metric sample. Monotonic offsets разных sessions не сопоставляются напрямую; UTC после перезапуска не восстанавливает прежнюю monotonic шкалу.

<a id="duration"></a>
### Duration — Длительность с качеством оценки

Величина интервала с единицей, методом и признаками известности; в Go-структуре Pri-Fly значения заданы в миллисекундах. `runtime.Duration` не равен `time.Duration` стандартной библиотеки. Неизвестное значение — `null` с причиной, а не измеренный ноль; незакрытый интервал не объявляется полной длительностью завершённого исполнения.

<a id="reported-cost"></a>
### ReportedCost — Сообщённая стоимость попытки

Точная неотрицательная десятичная сумма, которую один самоназванный источник
сообщил для конкретной Attempt, вместе с трёхбуквенной валютой. Запись
принадлежит Attempt и потому однозначно связана со StepInstance; она не является
стоимостью всей host session. Передавший её удостоверённый principal и время
приёма сохраняются обычным SessionHandoff и событием authority.

Несколько источников остаются несколькими записями и не примиряются. Отсутствие
ReportedCost означает «не наблюдалось», а `amount="0"` — явно сообщённый ноль.
Значение не вычисляется из токенов, не пересчитывается по таблице цен, не
конвертируется и не повышается до `provider_reported_charge` или
`settled_charge`: источник назван, но этой поставкой не квалифицирован.

<a id="telemetry-sample"></a>
### TelemetrySample / Measurement — Измерение

Количественное наблюдение или вычисленный показатель с единицей, методом, субъектом, происхождением и качеством. Внутренние measurements ядра представлены `TelemetrySampleData`, публичная проекция — `TelemetryRecord`. Число от worker не повышается до OS/provider measurement. Дополнительные samples могут теряться; обязательные управляющие факты должны сохраняться независимо от sampling.

<a id="telemetry-descriptor"></a>
### TelemetryMetricDescriptor — Описание метрики

Контракт имени, версии, единицы, kind, scope, происхождения, dimensions и допустимых агрегаций. Сходные названия метрик не разрешают складывать значения с разным смыслом или единицами. Go-имя — `TelemetryDescriptor`; metadata метрики не является самим измерением.

<a id="coverage"></a>
### Quality / Coverage — Качество значения / полнота наблюдений

**Quality** отвечает, как получено конкретное значение: measured, estimated, partial, unavailable или not_applicable в соответствующем контракте. **Coverage** отвечает, какая область наблюдалась, сколько ожидалось и что неизвестно или потеряно. Один measured sample не доказывает полного покрытия интервала или всех дочерних процессов.

<a id="telemetry-report"></a>
### TelemetryQuery / TelemetryReport — Запрос и отчёт телеметрии

Ограниченное чтение сохранённой истории с явными фильтрами, границей, единицами, методами и полнотой. В F1 доступны catalog, records, aggregate. **Cohort** — выбранная совокупность Runs; фильтр наблюдений внутри неё не должен скрыто менять знаменатель показателя надёжности. Cursor пагинации связан с запросом, владельцем и cut; сам не выдаёт полномочий.

Для invocation state v2 используются `core-timing/1`, `core-telemetry/2` и timing descriptor revision 2. Выбор зависит от Runs на выбранном cut; более поздний Run не меняет старый отчёт. Nested rollup учитывает уникальные реальные Attempts, а не складывает родительские итоги с теми же листьями. Workflow dimensions сохраняют root Run cohort; identities и дерево показывают child scope.

<a id="extensions"></a>
## Расширения F2

Это словарь целевых возможностей, не текущая таблица поддерживаемых команд. Точная последовательность, ограничения и приёмка определяются [delivery roadmap](../delivery-roadmap/spec.md).

| Термин | Значение и граница |
|---|---|
| [Choice](#choice) | Выбор ветви по закреплённым данным и объявленным predicates; правила true/false/unknown и ошибок задаёт контракт, не ИИ |
| [Call](#call) | Включение дочернего workflow в тот же Run; не независимый RunStart |
| [Repeat](#repeat) | Цикл с отдельной body invocation на итерацию, явным переносом состояния и конечными лимитами; не бесконечный polling |
| [Parallel / Join](#parallel-join) | Ограниченное выполнение ветвей и объявленное правило их объединения; завершение одной ветви не всегда завершает остальные |
| Map | Выполнение по заранее закреплённому составу коллекции; membership не дописывается на ходу |
| Wait | Сохраняемое ожидание допустимого источника, с correlation и сроками; не просто блокирующий sleep |
| Trigger | Объявленное основание для старта с отдельной проверкой допуска; внешнее событие не выдаёт права само себе |
| [Early artifact publication](#artifact-publication) | Принятие и доступность отдельной ArtifactRevision до завершения producer; не досрочный успех всей Attempt |
| [ArtifactClosure](#artifact-closure) | Точное закрытие keyed-many hook запечатанным manifest; не timeout, terminal producer или доставка stream consumer |
| [Scheduler](#admission-queue) | Планирование и допуск готовой работы с учётом ресурсов и ограничений; не LLM, выбирающая следующий шаг |
| ResourceClaim / Lease | Владение ресурсом и ограниченный срок такого владения; срок, поколение и fencing задаются контрактом ресурса |
| ActionIntent | Закреплённое намерение конкретной операции с точными arguments/target; не весь RunBrief |
| ActionDelivery | Одна доставка logical operation исполнителю; повтор доставки не равен новой Attempt всего worker |
| Effect / EffectReceipt | Внешнее последствие операции и свидетельство о нём; CommandReceipt и код выхода процесса не доказывают effect автоматически |
| Retry | Новая техническая попытка в рамках разрешённой политики; неизвестный прежний эффект не делает повтор безопасным |
| Reconciliation | Выяснение фактического исхода неопределённой операции; не угадывание и не безусловный повтор |
| Fork | Явное создание нового Run на разрешённых исходных данных; не копия БД, притворяющаяся прежней Authority |
| Reuse | Повторное использование подходящего результата/evidence после проверки identities, контекста и актуальности; не перенос прежних прав |
| Compensation | Отдельное разрешённое действие, компенсирующее последствия прежнего; не гарантированный транзакционный rollback внешнего мира |
| Retention / Garbage collection | Сроки сохранения и удаление действительно не удерживаемых данных; не удаление активных pins или чужих workspaces ради очистки |

<a id="profiles"></a>
### Semantics profile / Trust profile / F1 / F2 — Профиль и фаза

**Semantics profile** фиксирует правила исполнения, например `foundation-sequence/1`. **Trust profile** фиксирует границу доверия, например cooperative execution; это не доказательство изоляции. Execution mode, interaction mode и capacity — отдельные оси, не синонимы профиля семантики. **F1/F2** — фазы разработки из roadmap, не имена runtime-состояний. Реализация F2 не должна молча менять смысл сохранённых F1 Runs.

`core-workflow/1` — отдельный профиль расширений. Он не означает, что все операторы F2 уже поставлены. Protocol, definition, semantics, state/read и storage/event versions различаются: WorkflowRevision v2 может использовать protocol v1; `core-state/1` и `core-read/1` не заменяют старые F1 записи. SQLite/event envelope v1 не требуют миграции только из-за появления нового профиля. Неизвестные версии/события дают отказ; автоматического upcast или переписи истории нет.

P2-03 вводит `core-state/2` / `core-read/2` для новых workflows с calls либо объявленным partial. Flat Core workflows без них сохраняют state/read v1. Новый ready frontier принадлежит каждой invocation; прежнее Run.ready_stages в state v2 не дублируется. Несовместимый reader отказывает для всей authority при новой форме состояния/событий, а не пропускает её ради чтения другого старого Run.

P2-04 вводит state/read/next/preview v3 для нового Run, если repeat присутствует где-либо в compiled closure. Прежние Runs не upcast-ятся: flat Core v1 и calls/partial v2 остаются поддержанными. Новые RepeatProgress, RepeatDecision и iteration fields не добавляются в опубликованные старые bundles. Сам номер state не требует нового timing calculator при неизменных arithmetic/labels/order; прежние fixed cuts сохраняют результаты.

<a id="parallel-join"></a>
### Parallel / Join — Ветвление и объединение

**Parallel** — этап, объявляющий несколько ветвей; каждая ветвь — обычный дочерний workflow, а не облегчённая форма. **Join** — объявленное правило, по которому исход этапа выводится из исходов ветвей: `mode` (`all` / `quorum`), принимаемые исходы, политика отбора и обращение с остатком. Маршруты этапа — только `satisfied` / `unsatisfied`: это результаты join, а не исходы отдельной ветви.

`branches` — одно имя для двух разных форм. У `choice` элемент содержит predicate и переход, у `parallel` — ссылку на workflow и bindings; различает их только `kind` этапа. Это свойство опубликованных контрактов, и разбор обязан выбирать форму по виду этапа.

Бюджет параллельного этапа — сумма ветвей, а не наибольшая из них: выполниться могут все. Кворум, отменяющий остаток, тратит меньше, но граница не вправе рассчитывать на более дешёвый путь.

Одновременность ветвей ограничена тремя объявлениями, каждое со своей причиной отказа: квалификация сборки, объявленный предел самого workflow, и требование к join — одновременность допускается только там, где каждая ветвь обязана урегулироваться (`all` + `wait`). Join, способный решить рано, должен был бы остановить идущие ветви и подтвердить остановку; пока этого нет, сочетание отклоняется. Ёмкость установки действует отдельно, и меньшая из двух главенствует.

Решение объединения принимается по первой урегулированной, но ещё не решённой ветви в запечатанном порядке; счётчики следуют множеству решённых, а не позиции. Маршрут «записано» сворачивает ветвь в объединение, ничего не создавая и не урегулировая. Готовность нескольких областей — это ожидающая работа; ограничивается исполняющаяся, и область с собственной идущей работой фронта не держит.

Ассистируемый шаг объявляет `workspace_write` либо `none`, и объявление решает, выдаётся ли рабочая копия. Шаг без записи claim не получает и вправе писать только в свой объявленный слот вывода: две предлагающие ветви разделяет отсутствие права, а не соглашение между ними.

**AggregateManifest** — сводка ветвей, единственный выход параллельного этапа (порт `results`). Урегулированный join выпускает её запечатанным артефактом: вошедшие ветви с их идентичностью, run/invocation, статусом, исходом, признаком опоры на послабление и выходами. Форма поставляется как `core:schema/aggregate-manifest`; переложить её в собственную форму можно проекцией, а не заменой — типы сходятся по точной ссылке, а не по похожести. В сводке только вошедшие ветви: у не входившей нет ни invocation, ни результата. Неудавшийся join сводки не выпускает вовсе — пустая читалась бы как «ни с одной ветвью ничего не случилось».

Исполнение вводит `core-state/7` / `core-read/7`. Ветвь — обычная invocation с закреплённым определением и собственным урегулированным исходом; порядок ветвей запечатывается при входе. Одна урегулированная ветвь даёт одно durable **JoinDecision** — свидетельство о ней, не команда повторить. Вердикт выводится из счётчиков наблюдённых и принятых ветвей, поэтому не зависит от переноса значения между решениями.

`remainder` определяет судьбу остальных ветвей. При последовательном входе `cancel` означает, что они не входили вовсе: отменять нечего и брошенной работы нет; решение записывает `not_entered`, а не «отменено». `selection` схема жёстко связывает с `mode`, поэтому он записывается, но самостоятельного решения не несёт, пока агрегатный выход не потребляется. Упавшая ветвь не даёт исхода: это техническая неудача дочернего workflow, а не неудовлетворённый join.

<a id="admission-queue"></a>
### Admission capacity / Admission queue — Ёмкость и очередь допуска

**Ёмкость допуска** — сколько попыток authority допускает одновременно. Это записанное решение, а не константа: оно меняется в той же транзакции, что его фиксирует, поэтому применяемая граница и записанная причина не могут описывать разные числа. Потолок — заявление о квалификации, а не о мощности оборудования. Понижение ниже уже допущенного отклоняется: альтернативы — выселять живую работу либо оставить authority выше собственной границы.

**Очередь допуска** решает, чья очередь, когда мест нет. Объявленная политика — дольше всех ждущий первым, ничьи разрешаются идентичностью Run. Обгон отклоняется как `admission_deferred` с именем Run впереди; без этого занятый authority может морить ждущего бесконечно.

Место удерживается повторным запросом: хранилище видит строку, а не Run, и не отличает живого ожидающего от брошенного. Терпение измеряется в решениях о допуске, а не в тактах authority, и превышает размер очереди — иначе одна граница обессмысливает другую. Очередь ограничена по размеру: за ограниченным числом слотов не должна прятаться неограниченная величина.

Очередь не является ожиданием: отказ возвращается сразу, повторить попытку должен вызывающий.

<a id="capability-manifest"></a>
### CapabilityManifest — Манифест возможностей

Версионированное описание возможностей конкретной сборки: поддерживаемые protocol/storage/event versions, profiles, definition/state/read versions и реализованные capabilities. CLI `capabilities` — чтение метаданных `core.version`, не выдача прав. Go `ProfileCapabilities` — элемент этого manifest для конкретного Semantics profile. Manifest не является Grant, release evidence или обещанием поддержки будущего оператора. Перед исполнением проверяется профиль закреплённого Run, а не текущий default проекта.

`capabilities/2` содержит списки `state_versions` / `read_versions`; singular поля обозначают основную новую версию, а не исключают прежние поддерживаемые Runs. Новый ответ проверяется `CoreCapabilitiesV2`; опубликованный schema `CapabilityManifest` для capabilities/1 не переписывается.

<a id="bindings"></a>
## Соответствия Go и JSON

Это явная карта нынешней реализации. Пути указаны от корня репозитория; `—` означает привязку типа, а не отсутствие wire-контракта. Каноническое название не обязывает искусственно удлинять каждый локальный Go-идентификатор: важны точный смысл и зарегистрированное соответствие.

Таблицу проверяет `TestGlossaryBindings`: существование определения, package/type/прямого struct field и соответствие JSON tag. Проверка не доказывает правильность смыслового определения, полноту всей лексики или соблюдение любого внешнего стандарта. Неизвестные будущие F2-типы в таблицу не добавляются.

<!-- glossary-bindings:start -->
| Термин | Исходник от корня | Go | JSON |
|---|---|---|---|
| [WorkflowRevision](#workflow-revision) | `internal/flow/types.go` | `flow.WorkflowRevision` | — |
| [Resolution](#resolution) | `internal/runtime/model.go` | `runtime.ObligationResolution` | — |
| [Resolution](#resolution) | `internal/runtime/model.go` | `runtime.ObligationResolution.Outcome` | `outcome` |
| [ReleaseManifest](#release-manifest) | `internal/release/release.go` | `release.Manifest` | — |
| [ReleaseManifest](#release-manifest) | `internal/release/release.go` | `release.Manifest.SchemaVersion` | `schema_version` |
| [Managed binary installation](#managed-binary-installation) | `internal/release/release.go` | `release.Receipt` | — |
| [Managed binary installation](#managed-binary-installation) | `internal/release/release.go` | `release.Receipt.SchemaVersion` | `schema_version` |
| [TaskInput](#task-input) | `internal/runtime/task_intake.go` | `runtime.TaskInput` | — |
| [Stage](#stage) | `internal/flow/types.go` | `flow.Stage` | — |
| [StepDefinition](#step-definition) | `internal/flow/types.go` | `flow.StepDefinition` | — |
| [Workspace tree binding](#workspace-tree-binding) | `internal/flow/types.go` | `flow.WorkspaceTreeBinding` | — |
| [Workspace tree binding](#workspace-tree-binding) | `internal/flow/types.go` | `flow.StepDefinition.WorkspaceTrees` | `workspace_trees` |
| [Hook](#hook) | `internal/flow/types.go` | `flow.Hook` | — |
| [Hook](#hook) | `internal/flow/types.go` | `flow.StepDefinition.Hooks` | `hooks` |
| [PublicationSourceDefinition](#publication-source-definition) | `internal/flow/publication_sources.go` | `flow.PublicationSourceDefinition` | — |
| [Definition reference](#definition-ref) | `internal/flow/types.go` | `flow.Ref` | — |
| [Binding](#binding) | `internal/flow/types.go` | `flow.Binding` | — |
| [Subscription](#subscription) | `internal/flow/types.go` | `flow.Binding.SourceRef` | `source_ref` |
| [Binding](#binding) | `internal/flow/types.go` | `flow.Binding.Pointer` | `pointer` |
| [Binding](#binding) | `internal/flow/types.go` | `flow.Binding.ProjectedSchemaRef` | `projected_schema_ref` |
| [Stage](#stage) | `internal/flow/types.go` | `flow.Stage.OnError` | `on_error` |
| [Call](#call) | `internal/flow/types.go` | `flow.Stage.WorkflowRef` | `workflow_ref` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.BodyWorkflowRef` | `body_workflow_ref` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.InitialBindings` | `initial_bindings` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.NextBindings` | `next_bindings` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.ContinueOn` | `continue_on` |
| [Predicate](#predicate) | `internal/flow/types.go` | `flow.Stage.Until` | `until` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.MaxIterations` | `max_iterations` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.OnComplete` | `on_complete` |
| [Repeat](#repeat) | `internal/flow/types.go` | `flow.Stage.OnLimit` | `on_limit` |
| [Repeat](#repeat) | `internal/flow/repeats.go` | `flow.RepeatResult` | — |
| [Choice](#choice) | `internal/flow/types.go` | `flow.Stage.Selection` | `selection` |
| [Choice](#choice) | `internal/flow/types.go` | `flow.Stage.Branches` | `branches` |
| [Choice](#choice) | `internal/flow/types.go` | `flow.Stage.Default` | `default` |
| [Choice](#choice) | `internal/flow/types.go` | `flow.Stage.OnUnknown` | `on_unknown` |
| [ChoiceBranch](#choice-branch) | `internal/flow/types.go` | `flow.ChoiceBranch` | — |
| [Predicate](#predicate) | `internal/flow/types.go` | `flow.Predicate` | — |
| [Operand](#operand) | `internal/flow/types.go` | `flow.Operand` | — |
| [FieldRef](#field-ref) | `internal/flow/types.go` | `flow.FieldRef` | — |
| [Truth value](#truth-value) | `internal/flow/conditions.go` | `flow.Truth` | — |
| [Choice](#choice) | `internal/flow/conditions.go` | `flow.ChoiceResult` | — |
| [Choice](#choice) | `internal/flow/conditions.go` | `flow.BranchEvaluation` | — |
| [ChoiceDecision](#choice-decision) | `internal/runtime/choice.go` | `runtime.ChoiceDecision` | — |
| [ChoiceEvaluation](#choice-decision) | `internal/runtime/choice.go` | `runtime.ChoiceEvaluation` | — |
| [ChoiceInput](#choice-decision) | `internal/runtime/choice.go` | `runtime.ChoiceInput` | — |
| [RepeatProgress](#repeat-progress) | `internal/runtime/repeat_model.go` | `runtime.RepeatProgress` | — |
| [RepeatProgress](#repeat-progress) | `internal/runtime/model.go` | `runtime.Activation.Repeat` | `repeat` |
| [Iteration](#iteration) | `internal/runtime/repeat_model.go` | `runtime.RepeatProgress.IterationCount` | `iteration_count` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/repeat_model.go` | `runtime.RepeatProgress.CurrentBodyInvocationID` | `current_body_workflow_invocation_id` |
| [RepeatDecision](#repeat-decision) | `internal/runtime/repeat_model.go` | `runtime.RepeatProgress.LastDecision` | `last_decision` |
| [Iteration](#iteration) | `internal/runtime/invocation.go` | `runtime.Invocation.Iteration` | `iteration` |
| [RepeatDecision](#repeat-decision) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision` | — |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.InvocationID` | `workflow_invocation_id` |
| [StageActivation](#stage-activation) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.ActivationID` | `stage_activation_id` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.BodyInvocationID` | `body_workflow_invocation_id` |
| [Iteration](#iteration) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.Iteration` | `iteration` |
| [Outcome](#outcome) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.BodyOutcome` | `body_outcome` |
| [RepeatDecision](#repeat-decision) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.UntilResult` | `until_result` |
| [ChoiceInput](#choice-decision) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.Inputs` | `inputs` |
| [RepeatDecision](#repeat-decision) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.Route` | `route` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.NextBodyInvocationID` | `next_body_workflow_invocation_id` |
| [Observation](#observation) | `internal/runtime/repeat_model.go` | `runtime.RepeatDecision.Observed` | `observation` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/choice.go` | `runtime.ChoiceDecision.InvocationID` | `workflow_invocation_id` |
| [StageActivation](#stage-activation) | `internal/runtime/choice.go` | `runtime.ChoiceDecision.ActivationID` | `stage_activation_id` |
| [Observation](#observation) | `internal/runtime/choice.go` | `runtime.ChoiceDecision.Observed` | `observation` |
| [FieldRef](#field-ref) | `internal/runtime/choice.go` | `runtime.ChoiceInput.FieldRef` | `field_ref` |
| [ArtifactRef](#artifact-ref) | `internal/runtime/choice.go` | `runtime.ChoiceInput.SourceRef` | `source_ref` |
| [StageActivation](#stage-activation) | `internal/runtime/choice.go` | `runtime.ChoiceInput.ProducerActivationID` | `producer_activation_id` |
| [InputConfiguration](#input-configuration) | `internal/flow/types.go` | `flow.InputConfiguration` | — |
| [InputConfiguration](#input-configuration) | `internal/flow/types.go` | `flow.InputPort.Configuration` | `configuration` |
| [PolicyBundle](#policy-bundle) | `internal/flow/types.go` | `flow.WorkflowRevision.PolicyRef` | `policy_ref` |
| [AuthorityControl](#authority-control) | `internal/runtime/authority.go` | `runtime.AuthorityControl` | — |
| [ControlEpoch](#generation) | `internal/runtime/authority.go` | `runtime.AuthorityControl.ControlEpoch` | `control_epoch` |
| [PolicyBundle](#policy-bundle) | `internal/runtime/authority.go` | `runtime.AuthorityControl.PolicyRef` | `policy_ref` |
| [SessionPrincipal](#session-principal) | `internal/runtime/authority.go` | `runtime.AuthorityPrincipal` | — |
| [SessionPrincipal](#session-principal) | `internal/runtime/authority.go` | `runtime.AuthorityPrincipal.HumanID` | `human_id` |
| [ObjectAccess](#session-principal) | `internal/runtime/authority.go` | `runtime.AuthorityAccess` | — |
| [ObjectAccess](#session-principal) | `internal/runtime/authority.go` | `runtime.AuthorityAccess.PrincipalID` | `principal_id` |
| [Stop](#stop) | `internal/runtime/authority.go` | `runtime.AuthorityStop` | — |
| [Generation](#generation) | `internal/runtime/authority.go` | `runtime.AuthorityStop.Generation` | `generation` |
| [ControlPin](#control-pin) | `internal/local/store.go` | `local.ControlPin` | — |
| [SealedPackage](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageRecord` | — |
| [SealedPackage](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageEntry` | — |
| [SealedPackage](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageEntry.ManifestDigest` | `manifest_digest` |
| [PackageComponent](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageEntry.Components` | `components` |
| [PackageComponent](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageFile` | — |
| [TrustDecision](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageTrust` | — |
| [TrustDecision](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageTrust.ControlAdmissionID` | `control_admission_id` |
| [TrustDecision](#sealed-package) | `internal/runtime/packages.go` | `runtime.PackageOrigin` | — |
| [PackageStatus](#package-lifecycle) | `internal/runtime/packages.go` | `runtime.PackageEntry.Status` | `status` |
| [PackageStatus](#package-lifecycle) | `internal/runtime/packages.go` | `runtime.PackageEntry.StatusReason` | `status_reason` |
| [PackageStatus](#package-lifecycle) | `internal/runtime/package_lifecycle.go` | `runtime.PackageVerification` | — |
| [TrustRoot](#trust-root) | `internal/runtime/package_archive.go` | `runtime.TrustRoot` | — |
| [PackageSignature](#trust-root) | `internal/runtime/package_archive.go` | `runtime.PackageSignature` | — |
| [PackageSignature](#trust-root) | `internal/runtime/package_archive.go` | `runtime.PackageSignature.ManifestDigest` | `manifest_digest` |
| [PackageSignature](#trust-root) | `internal/runtime/packages.go` | `runtime.PackageTrust.SignedBy` | `signed_by` |
| [PackageSignature](#trust-root) | `internal/runtime/packages.go` | `runtime.PackageOrigin.ArchiveDigest` | `archive_digest` |
| [WorktreeClaim](#worktree-claim) | `internal/runtime/worktrees.go` | `runtime.ClaimRecord` | — |
| [WorktreeClaim](#worktree-claim) | `internal/runtime/worktrees.go` | `runtime.WorktreeClaim` | — |
| [WorktreeClaim](#worktree-claim) | `internal/runtime/worktrees.go` | `runtime.WorktreeClaim.Mode` | `mode` |
| [Generation](#generation) | `internal/runtime/worktrees.go` | `runtime.WorktreeClaim.Generation` | `generation` |
| [WorktreeClaim](#worktree-claim) | `internal/runtime/worktrees.go` | `runtime.RepositoryIdentity` | — |
| [WorktreeClaim](#worktree-claim) | `internal/runtime/worktrees.go` | `runtime.WorktreeClaim.Repository` | `repository` |
| [ResourceLease](#resource-lease) | `internal/runtime/worktrees.go` | `runtime.WorktreeClaim.LeaseUntil` | `lease_until` |
| [ClaimProcess](#resource-lease) | `internal/runtime/worktrees.go` | `runtime.ClaimProcess` | — |
| [ClaimProcess](#resource-lease) | `internal/runtime/worktrees.go` | `runtime.WorktreeClaim.Process` | `process` |
| [AssistedSession](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionHandoff` | — |
| [AssistedSession](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionHandoff.HostState` | `host_state` |
| [SessionHandoff](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionHandoff.WorkspaceMode` | `workspace_mode` |
| [SessionHandoff](#assisted-session) | `internal/runtime/model.go` | `runtime.Attempt.Session` | `session` |
| [SessionHandoff](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionTask` | — |
| [SessionHandoff](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionTask.WorkspaceMode` | `workspace_mode` |
| [SessionHandoff](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionTask.RepositoryWorkspace` | `repository_workspace` |
| [Workspace tree binding](#workspace-tree-binding) | `internal/runtime/workspace_trees.go` | `runtime.WorkspaceTreeHandoff` | — |
| [Workspace tree binding](#workspace-tree-binding) | `internal/runtime/sessions.go` | `runtime.SessionTask.WorkspaceTrees` | `workspace_trees` |
| [SessionHandoff](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionSubmission` | — |
| [SessionHandoff](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionSubmission.EnvelopeDigest` | `envelope_digest` |
| [AssistedSession](#assisted-session) | `internal/runtime/sessions.go` | `runtime.SessionHandoff.DeadlineTrust` | `deadline_trust` |
| [ReportedCost](#reported-cost) | `internal/runtime/sessions.go` | `runtime.ReportedCost` | — |
| [ReportedCost](#reported-cost) | `internal/runtime/model.go` | `runtime.Attempt.ReportedCosts` | `reported_costs` |
| [ReportedCost](#reported-cost) | `internal/runtime/sessions.go` | `runtime.SessionSubmission.ReportedCosts` | `reported_costs` |
| [ControlApproval](#control-approval) | `internal/runtime/approvals.go` | `runtime.Approval` | — |
| [ControlApproval](#control-approval) | `internal/runtime/approvals.go` | `runtime.Approval.IntentDigest` | `intent_digest` |
| [ControlApproval](#control-approval) | `internal/runtime/approvals.go` | `runtime.Approval.RequiredApprovals` | `required_approvals` |
| [ControlApproval](#control-approval) | `internal/runtime/approvals.go` | `runtime.Approval.Independence` | `independence` |
| [ControlApproval](#control-approval) | `internal/runtime/approvals.go` | `runtime.Approval.ConsumedByAdmissionID` | `consumed_by_admission_id` |
| [ControlApproval](#control-approval) | `internal/runtime/approvals.go` | `runtime.ControlApprovalPolicy` | — |
| [ApprovalVote](#control-approval) | `internal/runtime/approvals.go` | `runtime.Approval.VoteRefs` | `vote_refs` |
| [ControlGrant](#control-grant) | `internal/runtime/grants.go` | `runtime.Grant` | — |
| [ControlGrant](#control-grant) | `internal/runtime/grants.go` | `runtime.ControlGrant` | — |
| [ControlGrant](#control-grant) | `internal/runtime/grants.go` | `runtime.ControlGrant.UsedCount` | `used_logical_operations` |
| [ControlGrant](#control-grant) | `internal/runtime/grants.go` | `runtime.Grant.MaxLogicalOperations` | `max_logical_operations` |
| [ControlGrant](#control-grant) | `internal/runtime/grants.go` | `runtime.Grant.ConstraintsRef` | `constraints_ref` |
| [ControlGrant](#control-grant) | `internal/runtime/grants.go` | `runtime.GrantUse` | — |
| [ActionAdmission](#action-admission) | `internal/runtime/grants.go` | `runtime.GrantUse.ActionAdmissionID` | `action_admission_id` |
| [Resource identity](#control-grant) | `internal/runtime/grants.go` | `runtime.ResourceIdentity` | — |
| [QualityWaiver](#quality-waiver) | `internal/runtime/waivers.go` | `runtime.Waiver` | — |
| [QualityWaiver](#quality-waiver) | `internal/runtime/waivers.go` | `runtime.Waiver.CheckRef` | `check_ref` |
| [QualityWaiver](#quality-waiver) | `internal/runtime/waivers.go` | `runtime.Waiver.ApproverID` | `approver_id` |
| [QualityWaiver](#quality-waiver) | `internal/runtime/waivers.go` | `runtime.Waiver.AppliedTo` | `applied_check_execution_ids` |
| [QualityWaiver](#quality-waiver) | `internal/runtime/model.go` | `runtime.Run.Waivers` | `waivers` |
| [Outcome](#outcome) | `internal/runtime/model.go` | `runtime.Run.WaiverApplied` | `waiver_applied` |
| [EffectiveConfiguration](#effective-configuration) | `internal/runtime/compatibility.go` | `runtime.EffectiveConfiguration` | — |
| [ConfigurationValue](#effective-configuration) | `internal/runtime/compatibility.go` | `runtime.ConfigurationValue` | — |
| [EffectiveConfiguration](#effective-configuration) | `internal/runtime/model.go` | `runtime.Run.EffectiveConfiguration` | `effective_configuration` |
| [EffectiveConfiguration](#effective-configuration) | `internal/runtime/model.go` | `runtime.Run.WorkflowConfigurations` | `workflow_configurations` |
| [InputConfiguration](#input-configuration) | `internal/runtime/model.go` | `runtime.Configuration.InputValues` | `input_values` |
| [WorkflowAlias](#workflow-alias) | `internal/runtime/model.go` | `runtime.RegistryFile.Aliases` | `aliases` |
| [ProjectionManifest](#projection-manifest) | `internal/runtime/compatibility.go` | `runtime.ProjectionManifest` | — |
| [ProjectionManifest](#projection-manifest) | `internal/runtime/compatibility.go` | `runtime.ProjectionManifest.Source` | `source_ref` |
| [CapabilityManifest](#capability-manifest) | `internal/runtime/compatibility.go` | `runtime.CapabilityManifest` | — |
| [CapabilityManifest](#capability-manifest) | `internal/runtime/compatibility.go` | `runtime.ProfileCapabilities` | — |
| [CapabilityManifest](#capability-manifest) | `internal/runtime/compatibility.go` | `runtime.ProfileCapabilities.StateVersions` | `state_versions` |
| [CapabilityManifest](#capability-manifest) | `internal/runtime/compatibility.go` | `runtime.ProfileCapabilities.ReadVersions` | `read_versions` |
| [RunBrief](#run-brief) | `internal/runtime/model.go` | `runtime.Brief` | — |
| [Run](#run) | `internal/runtime/model.go` | `runtime.Run` | — |
| [DecisionCatalog](#decision-catalog) | `internal/runtime/decisions.go` | `runtime.DecisionCatalog` | — |
| [DecisionCatalog](#decision-catalog) | `internal/runtime/decisions.go` | `runtime.DecisionDefinition` | — |
| [DecisionCatalog](#decision-catalog) | `internal/runtime/model.go` | `runtime.Run.DecisionCatalog` | `decision_catalog` |
| [DecisionSheet](#decision-sheet) | `internal/runtime/decisions.go` | `runtime.DecisionSheet` | — |
| [DecisionSheet](#decision-sheet) | `internal/runtime/model.go` | `runtime.Run.DecisionSheet` | `decision_sheet` |
| [DecisionSheet](#decision-sheet) | `internal/runtime/sessions.go` | `runtime.SessionTask.DecisionSheet` | `decision_sheet` |
| [DecisionRequest](#decision-request) | `internal/runtime/decisions.go` | `runtime.DecisionRequest` | — |
| [DecisionAnswer](#decision-request) | `internal/runtime/decisions.go` | `runtime.DecisionAnswer` | — |
| [DecisionRequest](#decision-request) | `internal/runtime/model.go` | `runtime.Run.PendingDecision` | `pending_decision` |
| [DecisionRequest](#decision-request) | `internal/runtime/sessions.go` | `runtime.SessionSubmission.DecisionRequest` | `decision_request` |
| [DecisionRecord](#decision-record) | `internal/runtime/decisions.go` | `runtime.DecisionRecord` | — |
| [DecisionRecord](#decision-record) | `internal/runtime/model.go` | `runtime.Run.DecisionLedger` | `decision_ledger` |
| [ActionIntent](#action-intent) | `internal/runtime/actions.go` | `runtime.ActionIntent` | — |
| [ActionAdmission](#action-admission) | `internal/runtime/actions.go` | `runtime.ActionAdmission` | — |
| [ActionDelivery](#action-delivery) | `internal/runtime/actions.go` | `runtime.ActionDelivery` | — |
| [ActionIntent](#action-intent) | `internal/runtime/model.go` | `runtime.Run.ActionIntents` | `action_intents` |
| [ActionAdmission](#action-admission) | `internal/runtime/model.go` | `runtime.Run.ActionAdmissions` | `action_admissions` |
| [ActionDelivery](#action-delivery) | `internal/runtime/model.go` | `runtime.Run.ActionDeliveries` | `action_deliveries` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/model.go` | `runtime.Run.RootInvocationID` | `root_workflow_invocation_id` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/model.go` | `runtime.Run.Invocations` | `invocations` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/invocation.go` | `runtime.Invocation` | — |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/invocation.go` | `runtime.Invocation.ParentInvocationID` | `parent_invocation_id` |
| [StageActivation](#stage-activation) | `internal/runtime/invocation.go` | `runtime.Invocation.CallerActivationID` | `caller_stage_activation_id` |
| [WorkflowRevision](#workflow-revision) | `internal/runtime/invocation.go` | `runtime.Invocation.WorkflowRef` | `workflow_ref` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/invocation.go` | `runtime.Invocation.Inputs` | `input_refs` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/invocation.go` | `runtime.Invocation.Outputs` | `output_refs` |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/invocation.go` | `runtime.Invocation.Ready` | `ready_stages` |
| [Outcome](#outcome) | `internal/runtime/invocation.go` | `runtime.Invocation.Outcome` | `outcome` |
| [StageActivation](#stage-activation) | `internal/runtime/model.go` | `runtime.Activation` | — |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/model.go` | `runtime.Activation.InvocationID` | `workflow_invocation_id` |
| [StepInstance](#step-instance) | `internal/runtime/model.go` | `runtime.Step` | — |
| [Attempt](#attempt) | `internal/runtime/model.go` | `runtime.Attempt` | — |
| [StepInstance](#step-instance) | `internal/runtime/model.go` | `runtime.Attempt.StepID` | `step_instance_id` |
| [StageActivation](#stage-activation) | `internal/runtime/model.go` | `runtime.Attempt.ActivationID` | `stage_activation_id` |
| [StepResult](#step-result) | `internal/runtime/model.go` | `runtime.Result` | — |
| [StepInstance](#step-instance) | `internal/runtime/model.go` | `runtime.Result.StepInstanceID` | `step_instance_id` |
| [ArtifactRevision](#artifact-revision) | `internal/runtime/model.go` | `runtime.Artifact` | — |
| [WorkspaceTreeManifest](#workspace-tree-manifest) | `internal/runtime/workspace_trees.go` | `runtime.WorkspaceTreeManifest` | — |
| [WorkspaceTreeManifest](#workspace-tree-manifest) | `internal/runtime/workspace_trees.go` | `runtime.WorkspaceTreeManifest.Files` | `files` |
| [ArtifactRef](#artifact-ref) | `internal/runtime/model.go` | `runtime.ArtifactRef` | — |
| [ContextManifest](#context-manifest) | `internal/runtime/model.go` | `runtime.ContextManifest` | — |
| [ContextManifest](#context-manifest) | `internal/runtime/context.go` | `runtime.FullContextManifest` | — |
| [ContextManifest](#context-manifest) | `internal/runtime/context.go` | `runtime.FullContextEntry.ArtifactRef` | `artifact_ref` |
| [Context resource](#context-resource) | `internal/flow/context.go` | `flow.ContextResource` | — |
| [Context resource](#context-resource) | `internal/runtime/context_resources.go` | `runtime.PinnedResource` | — |
| [Context resource](#context-resource) | `internal/runtime/model.go` | `runtime.Run.ContextResources` | `context_resources` |
| [Context profile](#context-profile) | `internal/runtime/context.go` | `runtime.ContextProfile` | — |
| [Context profile](#context-profile) | `internal/runtime/model.go` | `runtime.ExecutorConfig.ContextProfileRef` | `context_profile_ref` |
| [SourceSnapshot](#source-snapshot) | `internal/runtime/sources.go` | `runtime.SourceSnapshot` | — |
| [SourceSnapshot](#source-snapshot) | `internal/runtime/sources.go` | `runtime.SourceSnapshot.ContentRef` | `content_ref` |
| [ContextRequest](#context-request) | `internal/runtime/sources.go` | `runtime.ContextRequest` | — |
| [CheckDefinition](#check-execution) | `internal/flow/checks.go` | `flow.CheckDefinition` | — |
| [CheckExecution](#check-execution) | `internal/runtime/check_execution.go` | `runtime.CheckExecution` | — |
| [CheckExecution](#check-execution) | `internal/runtime/model.go` | `runtime.Run.CheckExecutions` | `check_executions` |
| [CheckExecution](#check-execution) | `internal/runtime/model.go` | `runtime.Run.ActiveCheckID` | `active_check_execution_id` |
| [CheckRequest](#check-execution) | `internal/runtime/check_protocol.go` | `runtime.CheckRequest` | — |
| [CheckResult](#check-execution) | `internal/runtime/check_protocol.go` | `runtime.CheckResult.RequestDigest` | `request_digest` |
| [PendingAcceptance](#pending-acceptance) | `internal/runtime/acceptance.go` | `runtime.PendingAcceptance` | — |
| [PendingAcceptance](#pending-acceptance) | `internal/runtime/acceptance.go` | `runtime.PendingAcceptance.Bindings` | `bindings` |
| [PendingAcceptance](#pending-acceptance) | `internal/runtime/acceptance.go` | `runtime.PendingAcceptance.PreparedArtifacts` | `prepared_artifacts` |
| [PendingAcceptance](#pending-acceptance) | `internal/runtime/acceptance.go` | `runtime.PendingCheck` | — |
| [PendingAcceptance](#pending-acceptance) | `internal/runtime/model.go` | `runtime.Run.PendingAcceptance` | `pending_acceptance` |
| [Evidence](#evidence) | `internal/runtime/acceptance.go` | `runtime.EvidenceRef` | — |
| [CheckDefinition](#check-execution) | `internal/runtime/start.go` | `runtime.Preview.CheckExecutors` | `check_executors` |
| [Publication](#publication) | `internal/runtime/model.go` | `runtime.Publication` | — |
| [ArtifactPublication](#artifact-publication) | `internal/runtime/model.go` | `runtime.ArtifactPublication` | — |
| [ArtifactPublication](#artifact-publication) | `internal/runtime/model.go` | `runtime.Run.ArtifactPublications` | `artifact_publications` |
| [ArtifactPublication](#artifact-publication) | `internal/runtime/model.go` | `runtime.ArtifactPublication.Artifact` | `artifact_ref` |
| [ArtifactManifest](#artifact-manifest) | `internal/runtime/model.go` | `runtime.ArtifactManifest` | — |
| [ArtifactManifest](#artifact-manifest) | `internal/runtime/model.go` | `runtime.ArtifactManifest.Items` | `items` |
| [ArtifactClosure](#artifact-closure) | `internal/runtime/model.go` | `runtime.ArtifactClosure` | — |
| [ArtifactClosure](#artifact-closure) | `internal/runtime/model.go` | `runtime.ArtifactClosure.Manifest` | `manifest_ref` |
| [ArtifactClosure](#artifact-closure) | `internal/runtime/model.go` | `runtime.Run.ArtifactClosures` | `artifact_closures` |
| [Subscription](#subscription) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationSubscription` | — |
| [Subscription](#subscription) | `internal/runtime/model.go` | `runtime.Run.PublicationSubscriptions` | `publication_subscriptions` |
| [Subscription](#subscription) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationSubscriptionHandle` | — |
| [PublicationCursor](#publication-cursor) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationCursor` | — |
| [PublicationCursor](#publication-cursor) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationSubscription.Cursor` | `cursor` |
| [PublicationAssignment](#publication-assignment) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationAssignment` | — |
| [PublicationAssignment](#publication-assignment) | `internal/runtime/model.go` | `runtime.Run.PublicationAssignments` | `publication_assignments` |
| [PublicationAssignment](#publication-assignment) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationAssignment.Delivery` | `delivery_ref` |
| [PublicationDelivery](#publication-assignment) | `internal/runtime/publication_stream_model.go` | `runtime.PublicationDelivery` | — |
| [PublicationAssignment](#publication-assignment) | `internal/runtime/wait_model.go` | `runtime.WaitProgress.PublicationAssignmentID` | `publication_assignment_id` |
| [PublishStepPublicationCommand](#publish-step-publication-command) | `internal/runtime/model.go` | `runtime.PublishCommand` | — |
| [PublishStepPublicationCommand](#publish-step-publication-command) | `internal/runtime/model.go` | `runtime.PublishCommand.ExpectedStateVersion` | `expected_state_version` |
| [PublishStepPublicationCommand](#publish-step-publication-command) | `internal/runtime/model.go` | `runtime.PublishCommand.CandidatePath` | `candidate_path` |
| [PublishStepPublicationCommand](#publish-step-publication-command) | `internal/runtime/model.go` | `runtime.PublishCommand.ItemKey` | `item_key` |
| [PublishStepPublicationCommand](#publish-step-publication-command) | `internal/runtime/model.go` | `runtime.PublishCommand.ItemKeys` | `item_keys` |
| [Publication](#publication) | `internal/runtime/model.go` | `runtime.Publication.Version` | `state_version` |
| [Publication](#publication) | `internal/runtime/model.go` | `runtime.Publication.EventKey` | `event_key` |
| [Diagnostic](#diagnostic) | `internal/runtime/model.go` | `runtime.Diagnostic` | — |
| [StageActivation](#stage-activation) | `internal/runtime/model.go` | `runtime.Diagnostic.ActivationID` | `stage_activation_id` |
| [Stop](#stop) | `internal/runtime/model.go` | `runtime.Stop` | — |
| [Stop](#stop) | `internal/runtime/model.go` | `runtime.Stop.Scope` | `scope` |
| [Stop](#stop) | `internal/runtime/model.go` | `runtime.Stop.ScopeID` | `scope_id` |
| [ControlEpoch](#generation) | `internal/runtime/model.go` | `runtime.Run.ControlEpoch` | `control_epoch` |
| [Generation](#generation) | `internal/runtime/model.go` | `runtime.Stop.Generation` | `generation` |
| [Status](#status) | `internal/runtime/model.go` | `runtime.Attempt.Status` | `status` |
| [Verdict](#verdict) | `internal/runtime/model.go` | `runtime.Result.Verdict` | `verdict` |
| [Outcome](#outcome) | `internal/runtime/model.go` | `runtime.Run.Outcome` | `outcome` |
| [Authority](#authority) | `internal/runtime/model.go` | `runtime.Run.AuthorityID` | `authority_id` |
| [Installation](#project) | `internal/runtime/model.go` | `runtime.Installation` | — |
| [Authority](#authority) | `internal/runtime/model.go` | `runtime.Installation.ID` | `id` |
| [Project](#project) | `internal/runtime/model.go` | `runtime.ProjectConfig.ID` | `id` |
| [RunView](#run-view) | `internal/runtime/model.go` | `runtime.RunView` | — |
| [WorkflowInvocation](#workflow-invocation) | `internal/runtime/engine.go` | `runtime.NextView.InvocationID` | `workflow_invocation_id` |
| [Stage](#stage) | `internal/runtime/engine.go` | `runtime.NextView.StageID` | `stage_id` |
| [WorkflowRevision](#workflow-revision) | `internal/runtime/start.go` | `runtime.WorkflowPreview` | — |
| [WorkflowRevision](#workflow-revision) | `internal/runtime/start.go` | `runtime.Preview.Workflows` | `workflows` |
| [RunVersion](#run-version) | `internal/runtime/model.go` | `runtime.RunView.RunVersion` | `run_version` |
| [EventSequence](#event-sequence) | `internal/runtime/model.go` | `runtime.RunView.EventSequence` | `event_sequence` |
| [ReadCut](#read-cut) | `internal/runtime/model.go` | `runtime.RunView.Cut` | `cut` |
| [Observation](#observation) | `internal/runtime/model.go` | `runtime.Observation` | — |
| [Observation](#observation) | `internal/runtime/model.go` | `runtime.Observation.MonotonicMS` | `monotonic_ms` |
| [CommandReceipt](#command-receipt) | `internal/local/store.go` | `local.Receipt` | — |
| [CommandReceipt](#command-receipt) | `internal/local/store.go` | `local.Receipt.ID` | `command_id` |
| [RunVersion](#run-version) | `internal/local/store.go` | `local.Receipt.Version` | `run_version` |
| [EventSequence](#event-sequence) | `internal/local/store.go` | `local.Receipt.EventSeq` | `event_seq` |
| [EventSequence](#event-sequence) | `internal/local/store.go` | `local.Event.Seq` | `seq` |
| [ReadCut](#read-cut) | `internal/local/store.go` | `local.Receipt.Cut` | `cut` |
| [Duration](#duration) | `internal/runtime/timing.go` | `runtime.Duration` | — |
| [Duration](#duration) | `internal/runtime/timing.go` | `runtime.Duration.ValueMS` | `value_ms` |
| [TelemetrySample](#telemetry-sample) | `internal/runtime/telemetry.go` | `runtime.TelemetrySampleData` | — |
| [TelemetryMetricDescriptor](#telemetry-descriptor) | `internal/runtime/telemetry.go` | `runtime.TelemetryDescriptor` | — |
| [Coverage](#coverage) | `internal/runtime/telemetry.go` | `runtime.TelemetryCoverage` | — |
| [TelemetryQuery](#telemetry-report) | `internal/runtime/telemetry.go` | `runtime.TelemetryQuery` | — |
| [TelemetryReport](#telemetry-report) | `internal/runtime/telemetry.go` | `runtime.TelemetryResponse` | — |
| [ReadCut](#read-cut) | `internal/runtime/telemetry.go` | `runtime.TelemetryQuery.Cut` | `cut` |
<!-- glossary-bindings:end -->

Имена в колонках Go/JSON — совместимые текущие соответствия, не список обязательных переименований. Например, `runtime.Result` не нужно срочно заменять новым wire-полем `step_result`. При целевом переименовании меняются все затронутые ссылки и тесты; сохранённые контракты учитываются отдельно.

<a id="maintenance"></a>
## Как менять словарь и предотвращать расхождения

1. **Перед добавлением понятия** найти существующее определение здесь и в ТЗ. Синоним без нового смысла не требует нового типа или новой сущности. «Задача», «статус», «версия», «результат», «публикация» в спорном месте требуют уточнения объекта.
2. **При новой сущности** добавить каноническое название, русское определение, границы, отличие от соседних понятий и область F1/F2. Конкретные Go/JSON-соответствия добавлять только после появления реализации. Словарь не создаёт будущий DTO вместо реализации.
3. **При изменении значения или имени** в одном изменении обновить словарь, исходный раздел ТЗ, затронутые Go-типы/поля, schemas, CLI, примеры и тесты. Записать причину. Не менять данные или wire-имя ради стилистики без решения о совместимости.
4. **Сохранённая семантика защищена версиями.** Уточнение формулировки не переинтерпретирует прежние Runs. Изменение смысла требует соответствующего решения о contract/profile/storage versions и migration/reader checks. Старый release evidence не переписывается как будто новая версия уже проверена.
5. **Ревью проверяет смысл**, а `TestGlossaryBindings` — перечисленные механические соответствия. Если тест упал после переименования, сначала выяснить, меняется ли само понятие; нельзя только заменить ожидаемую строку ради зелёного теста.

Быстрая проверка текущей карты:

```sh
env GOTOOLCHAIN=local GOTELEMETRY=off GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" \
  .tools/go/bin/go test ./internal/runtime -run '^TestGlossaryBindings$' -count=1
```

Она также входит в `make test` и `make check`. Изменение capability specification требует OpenSpec validation и review затронутого контракта. Автоматическая проверка не ловит все смысловые расхождения в произвольном тексте: это остаётся обязанностью автора и reviewer.
