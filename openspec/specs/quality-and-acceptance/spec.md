## Purpose

Определяет, как Pri-Fly доказывает качество, отделяет обещание от результата
и ведёт полный каталог проверяемых product acceptance scenarios.

## Requirements

### Requirement: Качество разделяет specification, conformance и qualification

Pri-Fly MUST различать качество нормы, соответствие реализации контракту и
qualification конкретного deployment или adapter profile. Каждое заявленное
свойство MUST иметь проверяемое условие, subject/version, method, evidence и
известное ограничение; отсутствие проверки не становится pass.

#### Scenario: Schema проходит на DTO

- **WHEN** JSON Schema validation succeeds
- **THEN** результат подтверждает только форму DTO, а не безопасность effect или profile qualification

### Requirement: Заявленная самостоятельность проходит применимые конфигурации

Release MUST проверять empty core, local deterministic workflow и собственный
малый package перед заявлением самостоятельного универсального продукта.
Остальные profiles MUST объявляться доступными только после своей qualification.

#### Scenario: Показан coding demo

- **WHEN** demo зависит от Git, task provider или LLM
- **THEN** он не доказывает empty core или универсальность продукта

### Requirement: Технические проверки соответствуют границе свойства

Core MUST использовать обычные unit, property, integration и component checks
без обязательных LLM или платных API. Реальные adapter capability и semantic
quality AI package MUST квалифицироваться отдельно.

#### Scenario: Fixture описывает effect

- **WHEN** core suite использует synthetic receipt
- **THEN** отчёт отмечает fixture и не выдаёт его за выполненный external effect

### Requirement: Model invariants проверяются на допустимых interleavings

Property/model checks MUST сохранять authoritative writer, admission before
effect, idempotency scope, immutable accepted result, current stop и bounded
budgets на параллельных ветвях, retry и restart.

#### Scenario: Stop приходит в гонке с effect

- **WHEN** generated interleaving меняет порядок stop и dispatch
- **THEN** проверка находит counterexample, если старый control может ослабить current restriction

### Requirement: Fault injection проверяет состояние мира

Qualification MUST вводить сбои вокруг sealing, commit, admission, dispatch,
effect, receipt, lease, stop, checkpoint и recovery. Проверка MUST учитывать
число effects, claims, consumed approvals, known/unknown state и допустимость retry.

#### Scenario: Effect применён, а ответ потерян

- **WHEN** target applies action before response disappears
- **THEN** повторный зелёный запуск не заменяет reconciliation фактического числа effects

### Requirement: Доказательство соразмерно механизму

Evidence MUST относиться к тому же subject/version, что и принятый artifact.
Human review MUST решать смысловые вопросы, но не заменять hash, schema или
исполняемую enforcement boundary; AI quality MUST быть отделено от correctness core.

#### Scenario: Другой model review

- **WHEN** review выполняет агент с другим именем
- **THEN** это не считается независимой проверкой без самостоятельной evidence boundary

### Requirement: Negative и adversarial cases проверяют enforcement

Security suite MUST включать malicious paths, injection, forged controls и
receipts, IDOR, callbacks, malformed JSON, oversize inputs, identity ambiguity
и stale evidence. Ограничение cooperative profile MUST быть видимо пользователю.

#### Scenario: Prompt запрещает действие

- **WHEN** tool остаётся доступным без enforcement
- **THEN** prompt instruction не закрывает security case

### Requirement: Performance report сохраняет гарантии

Benchmark MUST публиковать hardware/software profile, workload, distribution,
durability/authorization settings и failures рядом с latency. Он MUST отличать
control transaction latency от queue, execution и external-effect wait.
Repository MUST содержать воспроизводимые benchmarks горячих путей authority
(открытие, чтение Run, admission, publication, telemetry на объявленном
потолке) на детерминированно генерируемых fixture БД объявленного размера; их
результаты фиксируются как evidence change, а регресс относительно
записанного baseline MUST быть замечен до release.

#### Scenario: Быстрый benchmark отключает durability

- **WHEN** измерение исключает обязательную persistence или redaction
- **THEN** результат не может объявляться production profile

#### Scenario: Горячий путь стал квадратичным

- **WHEN** benchmark на fixture БД удвоенного размера показывает рост latency
  быстрее объявленной сложности
- **THEN** release gate не проходит, пока причина не записана и не исправлена

### Requirement: Recovery qualification учитывает внешний unknown effect

Durable commit, backup и restore MUST проверяться в заявленной failure model.
Restore MUST восстановить state, blobs и locks, исключить второй active authority
и сверить незавершённые effects до нового dispatch.

#### Scenario: База восстановлена до отправки

- **WHEN** внешний effect мог произойти после snapshot
- **THEN** rollback локальной базы не объявляет effect безопасным

### Requirement: Управление доступно и честно показывает состояние

UI и CLI MUST показывать работу, ожидание, результат, unknown state, разрешение
и способ остановки без чтения большого журнала. Control decisions MUST быть
доступны с keyboard/focus/text status и показывать точный subject и scope.

#### Scenario: Широкий stop

- **WHEN** пользователь видит destructive control
- **THEN** интерфейс не скрывает broad scope коротким заголовком

### Requirement: Документация является проверяемой частью контракта

Guide MUST покрывать independent installation, свой step, workflow operators,
human wait, permissions, stop/reconcile, export/restore и uninstall. Examples
MUST validate on supported profile и явно называть external placeholders.

#### Scenario: Quick start зависит от проекта автора

- **WHEN** пример требует private worktree или AI Factory
- **THEN** он не считается universal quick start

### Requirement: Qualified release проходит definition of done

Profile MUST объявляться qualified только после applicable P0/P1 cases,
отсутствия critical integrity/security/data-loss/false-success defects,
migration/restore drill, verified capabilities, runbook, limitations и rollback
criteria. Protected core integrity/security check MUST NOT be waived.

#### Scenario: Возможность пока не реализована

- **WHEN** required case не был выполнен
- **THEN** его статус не становится not-applicable или qualified без обоснованной profile boundary

### Requirement: Изменение контракта создаёт regression и revised assessment

Изменённый requirement, DTO, predicate или outcome MUST иметь compatibility
decision и regression case. Adapter/provider change MUST requalify affected
guarantees; revised assessment MUST сохранять historical observations.

#### Scenario: Старый package получил version bump

- **WHEN** implementation or checker changed
- **THEN** старое evidence не переносится автоматически на новую version

### Requirement: Универсальность проверяется сквозными сценариями

Quality suite MUST проверять declared model на разных предметных flows,
включая data transformation, research, document publication, batch, human
decision, incident, coding, schedule, multi-worker и change of owner goal.

#### Scenario: Один coding demo успешен

- **WHEN** выполнен только один отраслевой сценарий
- **THEN** это не заменяет coverage остальных declared universal flows

### Requirement: Acceptance catalog сохраняет проверяемую трассировку

Catalog MUST содержать отдельные Given/When/Then cases, которые связывают
норму с проверкой, но не выдают traceability за достаточность assertion или
выполненный test result. Conflict between case and norm MUST быть resolved
путём уточнения нормы, а не convenient pass.

#### Scenario: Document checker завершился успешно

- **WHEN** ссылки каталога проверены механически
- **THEN** product qualification не объявляется выполненной

### Requirement: Acceptance catalog содержит полный declared product corpus

Постоянная спецификация MUST хранить 148 отдельных product acceptance scenarios
с их priority, verification kind, declared execution status, subject links и
evidence boundary. Каждый scenario MUST быть проверяемым независимо от общего
зелёного результата другого variant.

#### Scenario: Пустая независимая установка

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/runtime-resources/`, `openspec/specs/workflow-and-context/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Чистая среда без packages, Git repository, tracker, AI Factory и LLM credentials.
- **WHEN** Установить build, выполнить init/doctor/list и запросить отсутствующий workflow.
- **THEN** Диагностика и пустые списки доступны; unknown workflow отклонён без effects, скрытой установки standard и обращения к модели.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Controller не обращается к ИИ

- **Priority:** `P0`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Сохранены typed observations для choice, repeat, timeout, retry и завершения; все LLM/network вызовы controller запрещены test guard.
- **WHEN** Воспроизвести вычисление переходов и проверки допуска на этих входах.
- **THEN** Результат вычисляет код; guard не фиксирует вызов модели; worker prose и next_step не изменяют переход, authority или лимиты.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Проверка предмета до массовой работы

- **Priority:** `P1`
- **Verification kind:** `ux`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/control-security-ux/`
- **GIVEN** Policy требует подтверждения brief; draft для запроса о Pri-Fly ошибочно описывает pricing engine.
- **WHEN** Показать preview до bulk dispatch; пользователь отклоняет его с пояснением «это pricing, не Pri-Fly».
- **THEN** После явного отказа массовая работа заблокирована до исправления и подтверждения brief; код не объявляет себя способным самостоятельно понять невысказанное намерение человека.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Изменение цели не продолжает старые intents

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/control-security-ux/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Run уже имеет результаты и pending intents; пользователь удостоверенно меняет предмет задачи.
- **WHEN** Принять изменение scope и поздний ответ прежнего worker.
- **THEN** Новые admissions по прежнему scope прекращены; изменения оформлены revision/fork; старые материалы имеют provenance и не принимаются за результат новой цели; прежние effects сверяются.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: План модели остаётся предложением

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/domain-execution/`, `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`
- **GIVEN** Есть активный pinned workflow и предложенный моделью иной список stages.
- **WHEN** Передать предложение через preview, затем попытаться применить его как готовый маршрут.
- **THEN** Preview не исполняет эффекты; новый граф требует validation и разрешённого revision/fork; модель не изменяет текущие bindings и transitions напрямую.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Роль берётся из удостоверенной области

- **Priority:** `P1`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Оператор проекта A прислал actor_id администратора и ссылку на объект проекта B.
- **WHEN** Запросить действие от чужого имени и затем обычное разрешённое чтение своего проекта.
- **THEN** Самоописание не повышает права; чужой scope не доступен; своё разрешённое чтение работает без обязательной корпоративной регистрации.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Assisted без host не обещает wakeup

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`
- **GIVEN** Assisted adapter умеет dispatch, но не имеет подтверждённого background wakeup; workflow ждёт таймер.
- **WHEN** Отключить host и повторно открыть status после наступления срока.
- **THEN** Нет выдуманного фонового выполнения; видны ожидание host и неподдержанная capability; получение следующего хода не объявлено запуском worker.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Managed isolated требует реального runner

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/architecture-decisions/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`
- **GIVEN** Один adapter только печатает инструкцию, другой квалифицирован на spawn, наблюдение и confinement.
- **WHEN** Запросить managed-isolated исполнение с каждым adapter.
- **THEN** Первый отвергнут как unsupported; второй подтверждает реальный process lifecycle и границы доступа, а не только наличие envelope.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Отказ и молчание человека не равны согласию

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/workflow-and-context/`, `openspec/specs/product-model/`
- **GIVEN** Human workflow имеет отдельные accept/reject/timeout пути; Grant на автоматическое решение отсутствует.
- **WHEN** В одном запуске отклонить запрос, в другом не отвечать до declared timeout.
- **THEN** Выбраны соответственно rejection/timeout пути; ни ответ 'да' из другой беседы, ни истечение срока не создают Approval или success route.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Свой маленький workflow без чужого lifecycle

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/workflow-and-context/`, `openspec/specs/architecture-decisions/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Владелец создал локальный CSV-normalizer с typed input/output и checker, без пакетов разработки.
- **WHEN** Зарегистрировать пакет и исполнить минимальный workflow через public protocol.
- **THEN** Файл преобразован и проверен без изменения core, task-close, roadmap, review кода, Git и LLM; неизвестные capabilities не подменены shell fallback.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Модель выражает разные классы сценариев

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Подготовлены определения для обработки данных, исследования, документа, human request, инцидента и проверки кода.
- **WHEN** Проверить их выражение через опубликованные stages, adapters и acceptance contracts.
- **THEN** Ни один сценарий не требует core-условия по имени проекта или обязательного LLM controller; отсутствие готового отраслевого adapter явно отмечено.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Логические границы не превращены в обязательные сервисы

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Доступен dependency inventory reference core и схема его модулей.
- **WHEN** Проследить путь локальной command от CLI до journal и artifact store.
- **THEN** Путь не требует PostgreSQL, Redis, Kafka, Temporal, Kubernetes или обязательного UI; router не делает I/O, worker не получает private state writer.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: SourceSnapshot не привязывает core к tracker

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`
- **GIVEN** Один run получает typed snapshot независимого source adapter, второй получает явные локальные inputs без задачи.
- **WHEN** Выполнить одинаковый compatible workflow с обоими наборами входов.
- **THEN** Не создаётся фиктивный task_id; core не читает поля GitLab/roadmap по имени; source provenance и scope проверяются adapter контрактом.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Импорт artifact до создания Run

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** Run ещё не существует; уполномоченный principal импортирует локальный файл или SourceSnapshot в проект.
- **WHEN** Seal-ить input, сохранить import evidence и затем создать run со ссылкой на него.
- **THEN** Producer имеет import/source/principal provenance без фиктивных Step/Attempt; exact bytes доступны через digest; последующий run принимает ту же immutable revision.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Durable receipt входящего batch не равен применению

- **Priority:** `P1`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Batch содержит пригодный event и schema-valid event с неправильной correlation; источник повторяет batch после потери ответа.
- **WHEN** Оборвать ответ после inbox commit и повторить доставку.
- **THEN** Пригодный event применяется не более одного раза; второй сохраняет quarantine reason; receipt не объявляет оба events применёнными; иной payload под source_event_id конфликтует.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Unsupported capability не наследует чужую qualification

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Workflow требует fresh context и target readback, но выбранный adapter умеет только отправить запрос.
- **WHEN** Validate и попытаться начать исполнение.
- **THEN** Объяснено конкретное отсутствующее свойство; не запускаются другой provider, legacy tool или неподтверждённый режим; диагностика пустого core остаётся доступной.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.


#### Scenario: Identity пакета не допускает замены bytes

- **Priority:** `P0`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Зарегистрирован пакет с exact id/version/digest; второй manifest использует те же id/version, но иное содержимое.
- **WHEN** Проверить обе регистрации и попытаться исполнить mutable alias вместо exact reference.
- **THEN** Конфликт bytes отвергнут; alias разрешается только до lock; case/Unicode не нормализуются скрыто для обхода identity.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Архив пакета не выходит за свой root

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Пакеты содержат traversal, absolute path, symlink наружу и коллизию нормализованных имён.
- **WHEN** Получить и проверить каждый архив до регистрации.
- **THEN** Опасные entries отвергнуты до записи вне staging root; существующие пользовательские файлы и прежняя регистрация не изменены.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Lock включает транзитивные инструкции и schemas

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`
- **GIVEN** Workflow импортирует child, step, renderer, schema и adapter dependency.
- **WHEN** Создать run, затем заменить доступные aliases этих зависимостей новыми версиями.
- **THEN** Run использует исходный полный lock; ни инструкция, ни converter, ни схема не подтягиваются как latest в следующем шаге.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Отсутствующие pinned bytes не заменяются похожей версией

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/workflow-and-context/`
- **GIVEN** У активного run удалена необходимая pinned dependency; в каталоге доступна новая совместимая по названию версия.
- **WHEN** Выполнить resume и подготовку dependent step.
- **THEN** Получен pinned_resource_unavailable либо эквивалентный declared отказ; новая версия не исполнена под старым digest.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Установка атомарна и не запускает hooks

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`
- **GIVEN** Пакет предлагает post-install shell и новый exported workflow; прежний installed set уже существует.
- **WHEN** Остановить установку до atomic registration, затем повторить её.
- **THEN** Hook не исполнялся; до commit новый export недоступен; прежний набор цел; после commit зарегистрированы только проверенные files и refs.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Offline доверие не назначается ключом внутри пакета

- **Priority:** `P1`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Все dependency bytes доступны локально; пакет содержит собственный signing key, но не принят trust policy.
- **WHEN** Попытаться установить его offline, затем явно принять local trust допустимым владельцем.
- **THEN** Ключ не назначает себя доверенным; недоступная обязательная внешняя проверка не считается пройденной; разрешённый local trust записан без operational permissions.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Capabilities пакета не являются Grant

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Подписанный пакет требует network write; host допускает лишь чтение, а child workflow просит более широкие права.
- **WHEN** Запросить admission внешней записи через parent и child.
- **THEN** Оба запроса ограничены пересечением прав и отклонены; install approval, подпись и наследование не расширяют host scope.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Config precedence не перезаписывает ограничения

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Package default, project config и run input задают разрешённое значение и одновременно конфликтующие safety limits.
- **WHEN** Разрешить effective configuration, включая неизвестный параметр и попытку поднять права через run input.
- **THEN** Обычное значение следует объявленному precedence; неизвестное отвергнуто; ограничения пересечены, а не заменены более широким последним значением.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Обновление установлено рядом с активным lock

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/product-model/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Run использует version1; владелец выбирает version2 с изменённым checker и config migration.
- **WHEN** Установить version2 и продолжить старый run, затем preview нового.
- **THEN** Старый run сохраняет v1; новый выбор показывает зависимости, changes и requested rights; migration создаёт новую config revision, не меняя прежние bytes.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Uninstall сохраняет pinned execution и evidence

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/product-model/`, `openspec/specs/cli-protocol/`
- **GIVEN** Удаляемый пакет удерживают active run, compensation и retained evidence.
- **WHEN** Выполнить обычный remove и запросить export истории.
- **THEN** Новые resolutions закрыты, удерживаемые bytes сохранены либо remove объяснимо отвергнут; чужая работа не удалена; ссылки экспорта не обещают отсутствующее содержимое.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Quarantine отменяет trusted reuse cached pass

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Cached pass принадлежит checker revision, отозванной после security incident; входной artifact digest не менялся.
- **WHEN** Попытаться использовать cache для нового acceptance и параллельно принять физический receipt уже выполненного эффекта.
- **THEN** Trusted reuse/acceptance заблокирован до revalidation или допустимого trust decision; cache hit не восстанавливает доверие; receipt и unknown obligations не удалены.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Минимальный свой step имеет public contract

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/product-model/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Автор предоставил standalone command step, typed ports и проверку результата, но не изменял private helpers core.
- **WHEN** Установить, validate, запустить и удалить этот пакет через CLI.
- **THEN** Весь lifecycle работает без обязательных stage names, marketplace и нового framework; отсутствие private imports не мешает пользовательскому расширению.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Удаление AI Factory не ломает независимый сценарий

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/product-model/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Установлены optional AIF/task-close package и независимый noncoding workflow.
- **WHEN** Удалить AIF из новых resolutions и исполнить noncoding workflow.
- **THEN** Не требуется legacy ENGINE, /TZ, SMSPlace paths или Git; удалённая интеграция не была скрытой core dependency.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Qualification относится к конкретной операции

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`
- **GIVEN** У provider read API имеет qualified readback, а write API только response acknowledgement.
- **WHEN** Запросить profile с подтверждённым внешним применением для write.
- **THEN** Гарантия read API не переносится на write; admission требует правильного operation capability и target binding, иначе unsupported с точной причиной.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Каталог пакета не считает самоаттестацию испытанием

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/product-model/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Пакет содержит manifests и tests_passed=true, но отсутствуют actual qualification results и негативные примеры.
- **WHEN** Оценить candidate release и его отображение в каталоге.
- **THEN** Package виден как specified/implemented по фактам, но не qualified; отсутствующие proofs и requested permissions указаны; self-reported boolean не заменяет evidence.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Command, worker и check сохраняют разные контракты

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Workflow содержит command, worker и check с явно заданным исходом вызова worker; одна StepDefinition используется дважды.
- **WHEN** Выполнить нормальный и отрицательный исход check.
- **THEN** Созданы разные StepInstances; command не требует prompt; worker вызывается лишь на declared ветви; exit0 с verdict fail не превращается в pass.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Ports требуют точной schema либо converter

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Producer и consumer имеют похожие имена полей, но разные schema revisions/digests; доступен явно описанный converter.
- **WHEN** Validate прямое соединение и вариант через converter.
- **THEN** Прямое implicit coercion отвергнуто; converter принят только с совместимыми портами; remote schema refs не загружаются из сети при проверке.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Binding не выбирает последний похожий artifact

- **Priority:** `P0`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Две ветви выпускают одинаковый тип document; consumer сначала не указывает точного producer, затем использует stage_output binding.
- **WHEN** Проверить оба определения и состав input manifest.
- **THEN** Неоднозначное определение отвергнуто; допустимое связывает exact activation/output/revision независимо от времени записи файлов.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Обязательный input существует на каждом пути

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`
- **GIVEN** Только ветвь A создаёт review artifact, а общий consumer требует его на путях A и B.
- **WHEN** Validate граф без alternative binding, затем с явным optional/default подходящей schema.
- **THEN** Первый граф отвергнут до исполнения; второй принят только если отсутствие действительно разрешено контрактом и не скрывает обязательный результат.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Skip и no_work не создают отсутствующие outputs

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Producer пропущен по допустимому правилу; downstream требует output, который не был создан.
- **WHEN** Попытаться продолжить сначала без default, затем с declared compatible reuse.
- **THEN** Без output consumer не допущен; reuse проверяет exact subject и freshness; skipped/no_work/waived не превращаются в pass и не создают пустой файл.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Трёхзначная логика отличает отсутствие и null

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Predicate fixtures содержат false/true/unknown, отсутствующее optional поле, explicit null, строку '1' и число1.
- **WHEN** Вычислить eq/ne/exists/all/any по закреплённой AST semantics.
- **THEN** Нет строково-числового coercion; exists отличает отсутствие от null; truth tables соблюдены; schema error не становится false или unknown.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Exclusive choice не угадывает единственную ветвь

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Fixtures дают две true ветви, одну true плюс unknown и все false.
- **WHEN** Выполнить exclusive selection с объявленным default/on_unknown.
- **THEN** Две true дают ambiguity; unknown не доказывает уникальность; default используется только при доказанном false всех альтернатив; выбор имеет объяснение и input refs.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: First_match использует pinned порядок

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** В первом fixture unknown предшествует true; во втором true предшествует unknown; порядок branches закреплён.
- **WHEN** Вычислить first_match и explain для обоих fixtures.
- **THEN** Первый выбор ждёт/on_unknown, второй выбирает уже первую true; скрытая сортировка branches отсутствует; explain показывает исходный порядок и факты.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Condition не является исполняемым скриптом

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Condition содержит shell/SQL/template expression, неизвестный AST operator или превышение declared AST depth/node cap.
- **WHEN** Передать fixtures в валидатор и router.
- **THEN** Входы отвергнуты до исполнения; filesystem, environment, сеть и LLM не читаются; превышение лимита не обрезает отрицательные условия.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Структура definition не заменяет semantic validation

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`
- **GIVEN** JSON соответствует форме DTO, но имеет неизвестный stage target, недостижимый terminal path или запрещённую dependency.
- **WHEN** Выполнить все уровни validation; отдельно ограничить время валидатора.
- **THEN** Неверный граф не допущен; причина distinguishes reference/graph/capability; timeout означает 'не доказано', а не success.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Parallel соблюдает глобальные слоты и справедливость

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/runtime-resources/`, `openspec/specs/product-model/`, `openspec/specs/cli-protocol/`
- **GIVEN** Два runs имеют много ready branches; глобальный лимит active Step Attempts равен4, дочерние определения тоже запрашивают parallelism4.
- **WHEN** Конкурентно допускать работу и освобождать слоты.
- **THEN** Активных Step Attempts не более четырёх суммарно; claims учтены до запуска; появляющиеся слоты обслуживают оба run по объявленной политике, без starvation.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Quorum не публикует aggregate до settlement remainder

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/runtime-resources/`
- **GIVEN** Quorum2 из3 достигнут, но третья уже допущенная ветвь остаётся live; remainder=cancel.
- **WHEN** Выбрать qualifying set, запросить cancellation и попытаться запустить aggregate consumer до подтверждения остальных исходов.
- **THEN** Selected set сохранён, но output/transition удержаны; claims и reservations loser не освобождены; consumer допускается только после known terminal/cancel-confirmed remainder.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Satisfied join не равен успешному verdict

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** Child outcomes включают succeeded, completed_with_waivers и rejected; accept_outcomes задан явно.
- **WHEN** Вычислить all/quorum и выбранный finish outcome.
- **THEN** Засчитываются только разрешённые исходы; join возвращает satisfied/unsatisfied/empty и сохраняет исходы; waiver и missing required_for outputs не маскируются безусловным succeeded.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Map проверяет всю коллекцию до первого child

- **Priority:** `P0`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Коллекция содержит duplicate stable key, неверный item и превышение max_items; отдельный fixture содержит typed keys число1 и строку'1'.
- **WHEN** Validate и попытаться materialize map.
- **THEN** Невалидная коллекция отклонена целиком до admission; тип входит в key identity, индекс массива не используется как business key; max_items не превышает profile cap.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Изменение source collection не расширяет активный map

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Map закрепил sealed collection и child identities; живой источник добавил элемент и изменил порядок.
- **WHEN** Повторить intake прежнего manifest и продолжить работу при изменённом источнике.
- **THEN** Существующие children не дублируются и не переименовываются; новые bytes требуют предусмотренной новой activation/revision; item artifacts имеют provenance к исходной коллекции.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Пустой map имеет явный исход

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`
- **GIVEN** Входная коллекция пуста; одно определение имеет on.empty и совместимые outputs, второе этого пути не имеет.
- **WHEN** Validate и выполнить допустимое empty expansion.
- **THEN** Неполное определение отвергнуто; допустимое выбирает declared empty/no_work путь и не создаёт фиктивные successful child artifacts.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Общий budget ограничивает пустые и вложенные expansions

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/cli-protocol/`
- **GIVEN** Есть10 открытых root runs и легальный map32; дополнительные вложенные maps/repeats превышают общий StepInstance budget, другой граф создаёт только controls.
- **WHEN** Создать child invocations, запросить11-й root, продвигать граф до общих limits и выполнить restart/resume.
- **THEN** Root и child invocations учитываются в одном Run/journal/budget без child RunStart или отдельного CAS; map32 не требует32 root slots,11-й root ограничен;10000 instances/100000 controls на Run и4global Attempts соблюдены; counters не сброшены, stop/recovery доступны.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Repeat переносит явное состояние и сохраняет причины rework

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/runtime-resources/`
- **GIVEN** Body возвращает needs_revision, затем допустимый результат; initial/next bindings и max_iterations закреплены.
- **WHEN** Выполнить итерации, технический delivery retry и достижение iteration limit.
- **THEN** Новая semantic iteration имеет свою identity и artifact state; delivery retry не увеличивает semantic counter; restart не обнуляет предел; on_limit не продлевает ceiling молча.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Call graph ацикличен и экспортирует объявленные outcomes

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`
- **GIVEN** Определения содержат прямой и транзитивный cycle через alias; валидный nested child имеет partial/rejected exports и свой StageActivation path.
- **WHEN** Разрешить closure, validate call graph/outcome mappings и вычислить scope stop для child invocation.
- **THEN** Любая baseline recursion отвергнута; child identity остаётся внутри единственного Run, exports сохраняют provenance/outcomes; scoped stop охватывает descendants, unknown значимого child effect блокирует ordinary admissions всего Run.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Event и timeout разрешают wait только один раз

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Для активного wait одновременно доступны корректный event и доверенный timeout observation.
- **WHEN** Доставить их в разных порядках и повторить каждый callback.
- **THEN** Journal фиксирует один resolution согласно declared ordering; второй сохраняет late/duplicate reason; другой run/nonce/schema не открывает wait.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Callback раньше wait сохраняется через WaitRegistration

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/cli-protocol/`, `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`
- **GIVEN** До producer effect создан ReserveWaitCommand с текущей invocation, declared wait stage, source/schema/generation/nonce.
- **WHEN** Target немедленно возвращает callback до входа graph в wait; затем маршрут активирует этот wait.
- **THEN** Event durable в inbox и не меняет маршрут заранее; при actual activation применяется ровно один раз; отсутствие registration/handshake не маскируется гарантией callback delivery.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Невыбранная и истёкшая wait registration не запускает работу

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Регистрация принадлежит ветви, которая не выбрана; другая истекает по TTL; configured inbox count/byte caps конечны.
- **WHEN** Прислать ранние, поздние, повторные события и event сверх capacity, затем активировать иной wait.
- **THEN** Невыбранная registration отменена; старые nonce не открывают новый wait; caps соблюдены с явным receipt/отказом; authentication source не заменена знанием nonce.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Schedule slot не дублируется при DST и restart

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Fixture calendar содержит gap/fold, missed slots и overlap; timezone/parser semantics и catch-up budget закреплены.
- **WHEN** Обработать расписание, перезапустить driver и повторить те же logical slots.
- **THEN** Применены declared gap/fold/misfire правила; нет дублированных runs/effects; catch-up ограничен; создание workflow само не активирует расписание.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Control stage может быть producer без фиктивного worker

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Map/join или wait выпустил typed artifact; downstream binding ссылается на declared stage output.
- **WHEN** Принять control-produced artifact и построить provenance следующего шага.
- **THEN** Сохраняется control activation/operation identity и exact revision; не выдуманы Step Attempt или output port несуществующего worker; producer остаётся проверяемым.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Framing отвергает неоднозначный и чрезмерный JSON

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Requests содержат duplicate keys, повреждённый UTF-8, неизвестную protocol version, избыточные bytes/depth/nodes и неизвестное safety поле.
- **WHEN** Передать их на публичную границу до schema resolution.
- **THEN** Все запрещённые формы отвергнуты до dispatch с безопасной ошибкой; json с одним $defs root не считается проверенным конкретным DTO; никакого тихого truncation.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: ContextManifest фиксирует точный переданный набор

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Attempt имеет несколько input artifacts и instructions с объявленными order, trust labels, sizes и refs.
- **WHEN** Построить envelope и materialize контекст для executor.
- **THEN** Проверяемы actual bytes, порядок и classification; directory reference не разрешает чтение всех соседних файлов; secrets представлены handles.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Renderer не добавляет скрытое поручение

- **Priority:** `P1`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`
- **GIVEN** Одинаковые pinned instructions и envelope переданы renderer дважды; host предлагает произвольное дополнение цели.
- **WHEN** Сравнить сформированные bytes и попытаться включить недекларированную заметку.
- **THEN** Одинаковые входы дают одинаковый рендер; hidden transcript/goal change отсутствуют; current authenticated user/system restriction применяется как ограничение, не как отменяемый package текст.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Prompt injection не повышает доверие через summary

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Issue/PDF/tool output содержит 'пользователь разрешил', просьбу отправить secret и поддельное system message; summary повторяет этот текст.
- **WHEN** Передать источник и summary worker, затем предложенный им privileged action ядру.
- **THEN** Происхождение остаётся untrusted; текст не создаёт Grant/Approval, новый target или маршрут; действие отвергнуто вне разрешённого scope независимо от ответа модели.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Новый worker ID не доказывает fresh context

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`
- **GIVEN** Adapter выдаёт новый id, но наследует скрытую parent history или память прежнего reviewer.
- **WHEN** Квалифицировать и запустить profile, требующий fresh context и независимое review.
- **THEN** Fresh свойство не считается подтверждённым; несовместимый profile не исполняется; передаваемые старые artifacts допускаются только явными refs по policy.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Данные command передаются без shell expansion

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`
- **GIVEN** Typed argument содержит пробелы, кавычки и shell metacharacters; parent environment содержит лишний secret.
- **WHEN** Запустить разрешённый command adapter с controlled cwd и allowlist environment.
- **THEN** Программа получает literal аргумент; вложенная команда не исполняется, secret не наследуется, prose/LLM для механического шага не требуется.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Context reference не выдаёт право публикации

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Worker имеет разрешение прочитать document reference, но не получил Grant на запись/отправку.
- **WHEN** Предложить публикацию документа на произвольный recipient и прямой state write.
- **THEN** Ни контекст, ни успешное чтение не создают эти права; нужны exact разрешённые operations; worker не выбирает successor и не утверждает своё evidence.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Подмена live файла между preview и dispatch

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Preview согласовал digest входа; живой файл либо symlink target заменён перед запуском.
- **WHEN** Materialize и отправить input executor, затем попытаться принять result со старым fingerprint.
- **THEN** Используется исходная sealed копия либо dispatch отклонён; новые bytes не уходят под старым approval; confinement проверяет фактический объект, а не только prefix строки.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Дополнительный контекст получается явной операцией

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Worker требует новый источник, отсутствующий в manifest; Grant разрешает ограниченный lookup.
- **WHEN** Запросить допустимый lookup и чтение соседней неразрешённой папки.
- **THEN** Первый вызов имеет admission, budget, source provenance и отдельную reference результата; второй отвергнут; scope и цель не расширены скрытым retrieval.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Переполнение контекста не удаляет обязательный запрет

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/runtime-resources/`, `openspec/specs/cli-protocol/`
- **GIVEN** Обязательные instructions и artifact превышают caps; дополнительные варианты задают Context.max_tokens=null для LLM worker и для mechanical no-LLM step.
- **WHEN** Подготовить отправку без объявленного summary/chunking пути и проверить применимость token limit каждого варианта.
- **THEN** Overflow блокирует dispatch без truncation запретов; Context.max_tokens=null допустим только без LLM, модель требует конечный cap; estimate не выдан за exact bytes, summary требует своей provenance.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Credentials и запрещённые данные не отправляются модели

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Secret reference нужен tool adapter; input classification запрещает выбранного внешнего provider.
- **WHEN** Сформировать prompt, logs и попытку provider call.
- **THEN** Credential не попадает в prompt/history/metadata; отправка запрещённого класса данных блокируется; безопасная диагностика не повторяет значение найденного secret.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Валидный descriptor не доказывает содержимое PDF

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`
- **GIVEN** Artifact descriptor проходит JSON Schema, но bytes не соответствуют заявленному формату или content checker отсутствует.
- **WHEN** Выполнить acceptance бинарного output.
- **THEN** Неверное содержимое не принимается; отсутствие checker даёт ограниченную заявленную гарантию либо отказ по profile, но не фиктивную проверку качества документа.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Evidence различает исполнение, форму и смысл

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Worker сообщает pass; имеются process exit0 и schema_valid, но требуемого semantic review либо behavioural check нет.
- **WHEN** Сопоставить evidence с declared acceptance.
- **THEN** Недостающее утверждение не выведено из других; review показан как мнение определённого validator; не запрашивается скрытая цепочка мыслей как обязательный proof.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Reuse реагирует на значимые изменения, не bookkeeping

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`
- **GIVEN** Есть evidence с pinned subject/checker/runtime; один update меняет только heartbeat, другой меняет checker либо проверяемые bytes.
- **WHEN** Вычислить eligibility повторного использования для каждого случая.
- **THEN** Harmless update не вызывает бессмысленный повтор; значимые изменения, expiry или отсутствие bytes запрещают reuse; причина объясняется по declared dependencies.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Declared новые данные не требуют fork, замена исходных требует

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Workflow ожидает snapshot от объявленного fetch step; отдельно пользователь заменяет initial input вне предусмотренного графа.
- **WHEN** Принять оба события соответствующими каналами.
- **THEN** Обычный output продолжает тот же pinned контракт; замена исходного предмета требует нового допуска/revision/fork; stale result не переносится в изменённый контекст.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Read-only router не выдаёт admission

- **Priority:** `P0`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`
- **GIVEN** У run есть ready instance; сохранены journal version, claim set и budget balances.
- **WHEN** Многократно вызвать next/explain, затем отдельно отправить разрешённую mutation command.
- **THEN** Чтения ничего не меняют и не запускают tool; только committed command создаёт допуск с проверками; CLI/library используют одну authority semantics.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Конкурентный CAS не оставляет частичный допуск

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Две обычные команды разных клиентов имеют одну expected_run_version и конкурируют за один ресурс/остаток бюджета.
- **WHEN** Выполнить команды конкурентно и оборвать проигравшую внутри transaction boundary.
- **THEN** Побеждает один допустимый commit; event, claim, reservation и state проигравшей не записаны частично; version равна последнему committed seq.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Потерянный ответ command не вызывает повторный эффект

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** Command committed, но ответ клиенту потерян; затем run_version изменилась.
- **WHEN** Повторить exact request с тем же principal/command_id и затем прислать другой payload под этим id.
- **THEN** Exact duplicate возвращает прежний receipt до stale-version rejection без effect; иной payload конфликтует; исправленная rejected command использует новый id.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Receipt replay проверяет текущее право чтения

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Principal получил receipt, затем потерял read permission; другой principal знает command_id и изменяет project_id в payload.
- **WHEN** Запросить прежний результат обоими способами.
- **THEN** Недоступное содержимое и существование чужого объекта не раскрыты; command scope берётся из authenticated authority/principal; прежний commit не исчезает и не повторяется.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Crash windows blob sealing и reference commit

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Output upload проходит staging, проверку bytes, sealing и transaction reference; параллельно работает GC.
- **WHEN** Ввести сбой до sealing, после sealing и после reference commit.
- **THEN** Нет successful step с missing blob; orphan распознаётся; GC не удаляет active pinned upload или уже committed reference; partial upload не считается готовым artifact.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Потерянный referenced blob не восстанавливается заглушкой

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/architecture-decisions/`, `openspec/specs/control-security-ux/`
- **GIVEN** Journal ссылается на accepted artifact, но bytes отсутствуют либо имеют другой digest.
- **WHEN** Открыть dependent input и попытаться закрыть consumer.
- **THEN** Integrity incident и блокировка зависимости видны; не создаётся пустой replacement и не переносится trusted evidence на непроверенные bytes.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Независимый результат переживает чужой heartbeat

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/cli-protocol/`
- **GIVEN** Две параллельные Attempts работают над разными pinned inputs; первая меняет global run_version.
- **WHEN** Принять result второй с актуальным command CAS, затем повторить при изменённой relevant input/resource generation.
- **THEN** Первый result допустим несмотря на старый admitted_run_version; второй отвергнут по реальной зависимости; новое значение CAS не скрывает stale input.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Terminal result immutable и различается с progress

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`
- **GIVEN** Attempt уже имеет accepted terminal result; поступают exact duplicate, иной terminal payload и поздний progress.
- **WHEN** Выполнить intake всех сообщений, включая сообщение от старой отменённой Attempt.
- **THEN** Duplicate дедуплицирован; иной terminal payload конфликтует; progress не закрывает step; late evidence сохранено без замены нового результата.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: ExecutionAdmission не разрешает произвольные tools

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`, `openspec/specs/cli-protocol/`
- **GIVEN** Worker получил envelope со scoped Grants и собирается выполнить две разные tool operations.
- **WHEN** Допустить первую exact operation, а вторую изменить по recipient или arguments без нового admission.
- **THEN** У каждого разрешённого вызова свой ActionIntent/Admission; общий envelope не является blanket approval; изменённая операция отвергнута, уже работающий worker не обходит current gates.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Transport retry создаёт Delivery, не Step Attempt

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** Одна operation имеет safe retry qualification и owning Step Attempt; target запросил повтор доставки.
- **WHEN** Повторить exact operation в пределах deadline/budget.
- **THEN** Создан новый ActionDelivery id/ordinal; operation/intent/Admission/owning Attempt прежние; worker не запущен заново и approval не потреблён повторно.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Whole-worker restart не повторяет первый успешный tool

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Worker создал объект первой operation, затем исчез при второй; whole-step checkpoint/replay qualification отсутствует.
- **WHEN** Запросить автоматический retry всего StepInstance.
- **THEN** Новая Step Attempt не допущена вслепую; требуется reconciliation либо qualified protocol ledger всех прежних operations; идемпотентность второго вызова не доказывает безопасность первого.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: HTTP202 остаётся pending outcome

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Target возвращает HTTP202/job id и продолжает обработку; выполнение штатно наблюдается.
- **WHEN** Принять transport response, продолжить независимую ветвь и попытаться finish run до target terminal state.
- **THEN** Delivery response_received имеет effect_status=null; это не applied и не unknown; независимая работа допустима, terminal finish запрещён; final receipt ждёт классификации эффекта.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Потеря ответа после effect вводит uncertainty barrier

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Target применил запись, но final response потерян; другой worker уже работает и предлагает новый tool call.
- **WHEN** Обработать timeout и попытаться новые ExecutionAdmission/Admission, а также terminal failed/cancelled.
- **THEN** Effect unknown сохранён; ordinary admissions всего run, включая tools live worker, заблокированы; терминальный исход не скрывает неизвестность; разрешённый recovery доступен.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Qualified recovery resend без отдельного GET

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Unknown operation имеет durable payload-bound idempotency target, действующий TTL и только POST для получения прежнего результата.
- **WHEN** Выполнить authorized resend, затем варианты с новым ключом, изменённым target и истёкшим TTL.
- **THEN** Только exact resend допускается как reconciliation под новым Delivery; один logical effect подтверждён; прочие варианты запрещены, current controls и budget не обходятся.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Известный partial отличается от неизвестного остатка

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/cli-protocol/`
- **GIVEN** В одном receipt подтверждены применённый subset и неприменённый остаток; в другом судьба остатка неизвестна.
- **WHEN** Классифицировать effect и сохранить EffectManifest/provenance.
- **THEN** Первый outcome partially_applied; второй unknown с сохранёнными известными фактами; частичный HTTP response не выдаётся за полную сверку.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Compensation имеет новый допуск и правильный контекст

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/workflow-and-context/`, `openspec/specs/cli-protocol/`, `openspec/specs/domain-execution/`
- **GIVEN** Известный applied effect допускает declared compensation с preconditions и dependency order.
- **WHEN** Подготовить compensation child и попытаться использовать compensation_context в обычном несвязанном шаге.
- **THEN** Компенсация получает свой intent, права, budget и bindings исходных receipts; чужой контекст недоступен; исходный effect остаётся в истории, глобальный rollback не обещается.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Отказ компенсации оставляет residual obligations

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Compensation нужна по workflow, но право отозвано либо её собственный внешний результат потерян.
- **WHEN** Продолжить recovery и попытаться завершить run как succeeded.
- **THEN** Право не восстанавливается ради отката; failure/unknown компенсации и остаточный effect видны; обязательные unresolved obligations не скрыты successful finish.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Terminal outcome отражает объявленные обязательства

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/workflow-and-context/`, `openspec/specs/control-security-ux/`
- **GIVEN** Fixtures для succeeded, completed_with_waivers, no_work, rejected, partial содержат разные required_for outputs и effect obligations.
- **WHEN** Вычислить finish при полном evidence, отсутствующем mandatory output и pending/unknown effect.
- **THEN** Допускаются только доказанные contract outcomes; partial не скрывает свои required outputs; ни completed, ни failed/cancelled не закрывают pending/unknown.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Семантическая доработка отличается от технического retry

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Step получил needs_revision; отдельно failed step находится в открытом run, а другой run уже terminal.
- **WHEN** Запросить declared rework, безопасный retry открытого шага и возобновление terminal run.
- **THEN** Rework создаёт новую iteration/StepInstance; whole-step retry требует qualification и нового ExecutionAdmission; terminal history не переоткрывается, нужен связанный новый run.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Claims учитывают physical aliases и весь conflict set

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`
- **GIVEN** Разные projects/runs называют один physical directory или target object разными aliases; запрошены пересекающиеся shared/exclusive/capacity scopes.
- **WHEN** Конкурентно выдать claim sets и попытаться расширить scope после чужого admission.
- **THEN** Конфликт определяется реальной identity и capacity model; весь набор выдаётся атомарно; alias/project label не создаёт второй owner; расширение проверено до эффекта.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Истёкший lease не освобождает живого старого writer

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Lease истёк, но старый process либо сетевой запрос может ещё изменить ресурс; gateway проверил token до отправки.
- **WHEN** Выдать новый конфликтующий admission и доставить старый request позже.
- **THEN** Без actual target fencing новый admission удержан до сверки; gateway-only check не считается защитой; старый PID не используется без boot/start identity.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Cleanup не удаляет ресурс нового поколения

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`
- **GIVEN** Старый cleanup задержан; имя workspace/container/DB занято новым owner или существует неизвестный descendant process.
- **WHEN** Выполнить cleanup/recreate и запросить unconditional terminal completion.
- **THEN** Deletion проверяет exact owner/generation и блокируется при неизвестности; kill клиента не доказывает остановку server effect; незавершённый обязательный teardown остаётся видимым.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Локальная и remote identity имеют честную границу

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/product-model/`, `openspec/specs/domain-execution/`
- **GIVEN** Local profile использует один OS-account; remote request имеет неверные issuer/audience или просроченное удостоверение.
- **WHEN** Выполнить local read и remote privileged command, затем запросить независимый quorum двух aliases одного человека.
- **THEN** Local режим не требует облачной регистрации, но не доказывает разных людей; неверная remote identity отвергнута; aliases не засчитываются независимыми согласующими.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: IDOR, forged callbacks и чужие exports отвергаются

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Principal проектаA знает ids run/artifact/approval/export проектаB; callback имеет чужие scope, issuer либо nonce.
- **WHEN** Запросить read/list/download/decision и доставить callback.
- **THEN** Ни данные, ни чувствительное существование чужого объекта не раскрыты; callback не закрывает ожидание и не повышает trusted status; payload project_id не назначает authority.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Quorum approval учитывает действительных участников

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`
- **GIVEN** Policy требует двух независимых согласующих и запрещает автору protected revision approve своё действие.
- **WHEN** Принять повторное голосование одного человека через другой service account, затем два допустимых решения.
- **THEN** Дубликат и недопустимый автор не засчитаны; approved появляется лишь при достаточном quorum exact intent; requester не становится approver автоматически.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Intent pin-ит существующий subject и контракт будущего output

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`
- **GIVEN** Один intent публикует готовый файл, другой создаёт ещё несуществующий artifact; адресаты и output schema известны.
- **WHEN** Запросить approval и затем подменить существующие bytes, recipient либо protected arguments.
- **THEN** Для публикации pinned actual digest, для создания — expected contract без выдуманного будущего hash; изменение protected поля требует нового intent/approval.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Consume конкурирует с expiry/revoke атомарно

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Approval approved и имеет consume_before; одновременно идут два consume и допустимые revoke/expiry commands.
- **WHEN** Перебрать порядки commit и потерю ответа после consume.
- **THEN** Создан максимум один logical Admission; история не допускает одновременно несовместимых решений; duplicate возвращает прежнюю связь без повторного consumption.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Revoke после consume действует на ещё не начатый dispatch

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`, `openspec/specs/cli-protocol/`
- **GIVEN** Admission создан; одна delivery ещё не прошла local dispatch gate, другая прошла его до stop и может дойти до target позже.
- **WHEN** Отозвать разрешение и обработать обе deliveries.
- **THEN** Новая отправка запрещена current gates; ранее допущенная in-flight показана отдельно и наблюдается; historical consumed не переписан, сетевой эффект не объявлен мгновенно отменённым.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Глобальный release использует ControlIntent без фиктивного Run

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** Installation stop действует без run_id; есть exact ControlIntent с scope, stop ids/generations, expected epoch и policy approvals.
- **WHEN** Выполнить ReleaseStopCommand, затем повторить с изменённым target/epoch и прежним intent.
- **THEN** Допустимый ControlAdmission атомарно потребляет согласие и меняет exact scope; изменённый payload отвергнут; digest не цикличен через сам intent/approval refs; фиктивный worker не создаётся.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Pure resume не снимает stop

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`, `openspec/specs/product-model/`
- **GIVEN** Run paused под активным stop; клиент имеет актуальный run CAS и reason, но release ещё не выполнен.
- **WHEN** Вызвать ResumeCommand; затем выполнить guarded release, добавить новый stop и снова resume.
- **THEN** Оба resume при активном stop блокируются; reason/current CAS не заменяют release intent/approval; новый stop между commits не теряется.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Stale UI не мешает durable stop

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`
- **GIVEN** Run version меняется heartbeats; оператор имеет право stop и устаревшую observed version; отдельно storage не может commit.
- **WHEN** Выполнить RestrictCommand в обоих условиях.
- **THEN** При доступном storage запрет применяется к current scope/epoch до acknowledgement; при сбое записи success acknowledgement отсутствует; UI не обещает остановку по отправке команды.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Release не снимает чужие и более новые stops

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Активны installation stop и project stop; после подготовки release появился дополнительный stop нового поколения.
- **WHEN** Снять только выбранный exact stop и повторить старую release command.
- **THEN** Остальные ограничения сохраняются; stale identities/epoch не удаляют новый запрет; повтор command не расширяет освобождённую область.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Recovery после stop не является bypass

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`
- **GIVEN** Run uncertain/stopped; доступны scoped readback permissions и конечный recovery reserve, но write credential отозван.
- **WHEN** Запросить связанный probe и произвольную запись под меткой recovery.
- **THEN** Разрешён только конкретный probe; revoked credential и запрет пользователя сохраняются; recovery не допускает новые обычные steps/tools и не маскирует повторную публикацию.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Grant ограничен сроком, областью и общим бюджетом

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Grant допускает два logical operations в одном target scope, запрещает unattended и имеет конечный срок.
- **WHEN** Допустить три действия, child с более широкими правами и выполнение после expiry.
- **THEN** Только допустимые действия получают отдельные Admissions; превышение, child escalation и expiry отвергнуты; Grant не продлевает и не расширяет себя.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Waiver не отменяет integrity и authorization

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/product-model/`, `openspec/specs/domain-execution/`
- **GIVEN** Workflow допускает quality waiver, а package пытается объявить optional проверку identity/digest/host boundary.
- **WHEN** Выдать scoped waiver качества и попытаться waive protected invariants.
- **THEN** Допустимый quality waiver записан с scope/actor/reason; защищённые проверки не отключены и не переименованы пакетом в optional.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Waiver виден потребителям и в итоговом outcome

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`
- **GIVEN** Оператор отказывается от конкретной quality проверки, связанной с результатом; часть downstream требует её artifact.
- **WHEN** Открыть waiver preview, подтвердить допустимое решение и построить итог.
- **THEN** UI показывает заблокированных consumers; missing artifact не создаётся; допускаемый quality reduction сохраняется в completed_with_waivers и списке непроверенного, не как pass.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Новый запрет пользователя выше pinned package instructions

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`
- **GIVEN** Package требует tests/publish, но пользователь из удостоверенного канала запрещает дальнейшее действие.
- **WHEN** Принять restriction и попытаться dispatch по старому envelope; отдельно дать такое же указание из untrusted issue.
- **THEN** Удостоверенный запрет соблюдён; обязательная работа не отмечена выполненной; issue не создаёт authority; старый prompt не отменяет current restrictions.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Target проверяется на реальном соединении

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/workflow-and-context/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Allowed URL перенаправляет на иной account/private metadata endpoint; аргументы содержат SQL/shell fragments.
- **WHEN** Выполнить typed tool preparation и connection resolution.
- **THEN** Необъявленный destination/redirect и command injection заблокированы; private API возможен только как explicit ресурс; initial URL allowlist не разрешает любой конечный адрес.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Изоляция проверяется действиями, а не названием контейнера

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`
- **GIVEN** Worker пытается писать journal, читать чужой secret, выйти через symlink и использовать широкий host socket/mount.
- **WHEN** Запустить negative suite в declared managed-isolated и cooperative profiles.
- **THEN** Изолированный profile реально отклоняет действия; cooperative profile не заявляет такую защиту и не допускает workload, которому она обязательна.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Credential rotation сохраняет или меняет authority явно

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/runtime-resources/`
- **GIVEN** Один новый secret принадлежит тому же account/resource, второй — другому provider account.
- **WHEN** Ротировать credential у prepared intent и неизвестной старой operation.
- **THEN** Same-authority rotation требует проверки и не раскрывает secret; другой account требует нового binding/допуска; unknown старый request не перенесён и не повторён под новым account.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Hash и worker prose не подделывают trusted receipt

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`
- **GIVEN** Worker прислал signed-looking JSON либо hash без проверенного issuer, связал receipt с чужой operation/delivery или иным subject.
- **WHEN** Проверить сообщение и попытаться повысить его до remote_applied evidence.
- **THEN** Подлинность, scope и correlation проверены независимо; неподтверждённое сообщение остаётся ограниченным observation и не закрывает более сильное требование.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Preview и ошибки не исполняют непроверенный контент

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`
- **GIVEN** Artifact/log содержит HTML, terminal escape sequence, опасную ссылку и remote image URL.
- **WHEN** Показать preview, error details и CLI JSON output.
- **THEN** Контент экранирован, remote embedded resources не загружены автоматически; исходное скачивание отделено от preview; машинный поток не содержит управляющих escape codes worker.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Data policy действует на все выходные каналы

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Secret и закрытые данные попали в error, stdout/stderr, evidence text, thumbnail metadata и export candidate.
- **WHEN** Пропустить каждый candidate через реальный writer и доступ viewer с меньшими правами.
- **THEN** Неразрешённые bytes не опубликованы; redaction имеет собственный digest/provenance; diagnostic не повторяет secret; regex scan не назван полной защитой.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Ручной результат и break-glass не являются force success

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`
- **GIVEN** Человек выполнил работу вне executor и предлагает done=true без evidence; аварийный profile допускает лишь ограниченное сокращение quality checks.
- **WHEN** Импортировать manual result и попытаться обойти identity/authorization либо править state напрямую.
- **THEN** Приняты только scoped artifacts/receipts с actor/provenance и нужными проверками; invariant bypass и direct state rewrite отвергнуты; incident reference и остаточный риск сохранены.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Crash boundaries сохраняют точное состояние эффекта

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Управляемый test target ведёт независимый ledger; проверяемый build допускает остановку до/после admission commit, dispatch, фактического effect и receipt persistence.
- **WHEN** В каждой точке завершить process, открыть journal заново и выполнить разрешённое recovery.
- **THEN** Acknowledged commit сохранён; отсутствие launch доказано либо отмечена unknown; effects, consumptions и reservations сверены с target; новый запуск не считается доказательством отсутствия дубля.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Checkpoint и текущая проекция не образуют вторую историю

- **Priority:** `P0`
- **Verification kind:** `unit`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`
- **GIVEN** Один journal имеет проверенный checkpoint и последующие events; другой checkpoint испорчен; третий содержит неподдерживаемую event/reducer version.
- **WHEN** Построить состояние полным replay и через checkpoint, затем открыть неподдерживаемые данные старым reader.
- **THEN** Поддержанные пути дают одинаковый state digest/as_of_seq; повреждённый checkpoint не становится authority; несовместимость явна и не разрешает effect; revised assessment сохраняет исходные события.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Restore старого snapshot не воскрешает consumed права

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Backup сделан до approval consumption, target effect и нового fence; после него часть receipts утрачена локально, referenced blobs доступны по manifest.
- **WHEN** Восстановить backup на отдельной среде и запросить dispatch с прежним approval/fence counter.
- **THEN** Среда остаётся в recovery mode; старое approved не означает свободное consumption, counter+1 не доказывает актуальный fence; effect scopes закрыты до независимой сверки и проверки полноты blobs.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Erasure переживает более старое восстановление

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Данные удалены после backup; актуальный erasure ledger хранится за его recovery boundary, а в одном варианте ledger недоступен.
- **WHEN** Восстановить старый journal/blob snapshot и запросить выдачу этих bytes пользователю или worker.
- **THEN** До проверки актуального erasure checkpoint данные quarantined; доступный ledger применяется до открытия; недоступный ledger не заменён tombstone из того же старого backup.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Clone backup и shared filesystem не создают HA

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/architecture-decisions/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Local profile хранит SQLite на локальном FS; его копию пытаются одновременно открыть как вторую authority, а другой config указывает общий network mount.
- **WHEN** Активировать обе копии и запросить конфликтующие effects; проверить preflight shared-storage config.
- **THEN** Clone не допускает эффекты до безопасного authority recovery; network/shared coordinator не получает local guarantees; нужен отдельно qualified profile, а не два writer поверх одной копии.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Clock rollback не продлевает approval и deadline

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`, `openspec/specs/control-security-ux/`
- **GIVEN** Записаны trusted time observations; wall clock откатился либо host перезагрузился; worker прислал удобный occurred_at.
- **WHEN** Выполнить replay и запросить admission с истекающим сроком; сравнить monotonic readings разных boot sessions.
- **THEN** Replay не читает живые часы; несопоставимые значения не сравниваются как единая шкала; недоверенный timestamp не продлевает право; при неопределённом current time допуск отклонён.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Durable timer не порождает повторный transition

- **Priority:** `P1`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/workflow-and-context/`, `openspec/specs/domain-execution/`
- **GIVEN** Managed host остановлен перед deadline, timer delivery повторяется после restart; отдельный assisted host не запущен.
- **WHEN** Доставить один просроченный timer дважды и затем поздний event прежней wait generation.
- **THEN** Managed режим принимает максимум один логический timeout; поздний event не открывает новое ожидание; assisted UI честно показывает отсутствие активного host и обещанного wakeup.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Общий budget не размножается в tools и children

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/control-security-ux/`, `openspec/specs/product-model/`, `openspec/specs/architecture-decisions/`
- **GIVEN** ExecutionAdmission имеет резерв, worker вызывает несколько tools, child requests конкурируют за остаток, один provider usage неизвестен.
- **WHEN** Параллельно reserve/transfer/settle, повторить settlement и дождаться TTL неизвестного вызова.
- **THEN** Затраты учитываются ровно в выбранной budget model без двойного списания или новых денег; unknown резерв не освобождён по TTL; hard cap заявлен лишь при enforceable target limit, estimate подписан отдельно.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Disk full и queue pressure останавливают новый effect

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/domain-execution/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Диск не вмещает durable receipt либо очередь достигла лимита; имеется отдельный конечный recovery reserve.
- **WHEN** Запросить новое обычное действие, залить чрезмерный artifact и выполнить разрешённую cancellation/reconciliation.
- **THEN** Новый effect не начинается без возможности durable учёта; oversized upload не обходит cap; обычная работа получает backpressure; recovery расходует только свой разрешённый reserve, без бесконечного продолжения.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Local-1 измеряется с полной durability и ошибками

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Зафиксированы 4 CPU, 16 GiB RAM, local SSD, минимум 20 GiB свободно, OS/FS/SQLite и mandatory settings; store пустой либо содержит 1 млн events с типичным payload 2 KiB.
- **WHEN** Измерить минимум 10 000 control commands при sustained 10/s и burst 50/s в течение 10s, включая configured concurrency, readers и фоновые checkpoints.
- **THEN** Report содержит p50/p95/p99, ошибки и contention для каждой нагрузки; P95 commit target 250ms проверен отдельно от provider time; цель не объявлена достигнутой при отключённом fsync/authz или исключённых отказах.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: RPO/RTO разделены по failure model и effect recovery

- **Priority:** `P1`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/runtime-resources/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Local-1 использует исправное storage; отдельно есть backup каждые15min размером до10GiB и среда для power-loss qualification.
- **WHEN** Измерить process restart, isolated restore и потерю питания как разные drills; зафиксировать ещё неизвестный внешний effect.
- **THEN** Process target ≤60s относится к контролю/классификации; RPO≤15min и RTO≤30min проверяются до safe read/recovery при заданном backup; effect-resume RTO отдельный; отсутствие backup или power-loss evidence не замаскировано общей гарантией.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Реальная SQLite configuration соответствует durable profile

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/runtime-resources/`
- **GIVEN** Build может использовать разные linked SQLite versions; открывается несколько connections, а journal schema имеет foreign keys и uniqueness constraints.
- **WHEN** Проверить actual build/обязательные WAL исправления, настройки каждой connection, invalid FK/duplicate writes и concurrent transaction rollback.
- **THEN** Неподдерживаемая сборка не qualified; FK действуют в каждом connection, требуемые WAL/FULL включены; ошибочная запись не оставляет половину transition или два receipts одной identity.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Telemetry и отчёт восстанавливаются из journal

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/product-model/`
- **GIVEN** Run содержит waiting/uncertain ветви, retry deliveries, waiver, orphan resource и недоступный provider usage; export отчёта потерян, log превысил cap.
- **WHEN** Снова построить report и метрики, затем сопоставить их с event records.
- **THEN** Причины ожидания, unknown, retry и rework воспроизводятся; usage остаётся null с причиной, truncation обозначен, structured terminal result сохранён; raw secrets и высококардинальные ids не попали в общие metric labels.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Security update находит затронутые runs без переписи прошлого

- **Priority:** `P0`
- **Verification kind:** `security`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/architecture-decisions/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/domain-execution/`
- **GIVEN** У конкретной interpreter/checker/package version обнаружен дефект, способный дать ложный pass; известны runs с этой версией.
- **WHEN** Ограничить будущие admissions, найти affected records и выпустить revised assessments после исправления.
- **THEN** Исходные outputs/events сохранены; новая оценка с reason/version связана с ними; затронутые capabilities проходят повторную qualification; version bump не переносит автоматически старое доверие.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Retention не стирает безопасную identity операций

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/product-model/`, `openspec/specs/control-security-ux/`
- **GIVEN** Явная retention policy удаляет старые подробности; существуют active manifests, unresolved effect и давно исполненная operation с истёкшим transport retention.
- **WHEN** Выполнить GC/export и повторно прислать старую business operation identity.
- **THEN** Критические references/receipts не удалены; старая identity остаётся non-reusable tombstone либо namespace закрыт; TTL не разрешает второй effect; export не раскрывает запрещённые данные.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Storage upgrade и export не запускают работу

- **Priority:** `P0`
- **Verification kind:** `fault_injection`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/domain-execution/`, `openspec/specs/architecture-decisions/`, `openspec/specs/cli-protocol/`
- **GIVEN** Есть versioned journal с pending/unknown operation, portable export и несовместимый старый reader; migration подготовлена на копии.
- **WHEN** Остановить admissions, сделать backup, выполнить upgrade и попытаться rollback binary/import export как готовую active authority.
- **THEN** Schema compatibility и replay проверены; migration/export не вызывают пользовательские tools; неизвестность сохранена; incompatible rollback и автоматическая активация imported effects запрещены.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: CLI, library и UI не имеют отдельных переходов состояния

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`
- **GIVEN** Один и тот же run доступен через поддерживаемые interfaces; UI выполняет тот же typed command, что CLI/library.
- **WHEN** Сравнить command result, terminology, errors и JSON output; попытаться обойти gate через иной interface.
- **THEN** Идентичные commands под одинаковой authority имеют одинаковую семантику; gate не обходится; machine JSON не смешан с prose, human UI не создаёт вторую модель состояния.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Preview показывает цель и не является разрешением

- **Priority:** `P0`
- **Verification kind:** `ux`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`, `openspec/specs/product-model/`
- **GIVEN** Пользователь готовит массовый workflow с дорогими actions, branches и target outputs; некоторые значения заранее неизвестны.
- **WHEN** Открыть preview/dry run, изменить scope и попытаться выполнить effect, ссылаясь только на preview.
- **THEN** Цель, объём, ветви, ограничения и unknown estimates видны; preview не исполняет tools и не создаёт approvals; изменённый scope снова показан перед соответствующим допуском.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Прогресс показывает реальное ожидание и исполнителя

- **Priority:** `P1`
- **Verification kind:** `ux`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`
- **GIVEN** Есть обычная работа, HTTP202 pending, paused step, unknown effect, map collection ещё не sealed и assisted session без host.
- **WHEN** Открыть run view, обновить данные после потери связи и прочитать доступную текстовую сводку.
- **THEN** Статусы и актуальность различимы; pending не назван applied, неизвестный denominator не даёт выдуманный процент; видно, кто реально исполняет и какие действия безопасны дальше.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Approval UI не скрывает scope за коротким названием

- **Priority:** `P0`
- **Verification kind:** `ux`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`
- **GIVEN** Два intents имеют похожие labels, но разные recipients, input digests или protected arguments; batch содержит больше items, чем одна страница.
- **WHEN** Открыть решение, сравнить diff, просмотреть batch и подтвердить выбранный exact intent.
- **THEN** Показаны subject/target, последствия, expiry и граница согласия; выбор не расширяется на скрытые items; последующая подмена требует нового решения, старый label не подменяет identity.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Ошибка объясняет безопасный следующий шаг

- **Priority:** `P1`
- **Verification kind:** `ux`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/cli-protocol/`
- **GIVEN** Ошибки включают stale command version, missing artifact, denied permission, unknown effect и неподдержанную capability.
- **WHEN** Прочитать их в CLI JSON и human view, затем выбрать предложенное продолжение.
- **THEN** Есть стабильный code, безопасная диагностика и актуальное состояние; retry/readback/replan не смешаны; кнопка retry не означает новую операцию при unknown; secret payload не показан.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Управление доступно без цвета и чтения полного лога

- **Priority:** `P1`
- **Verification kind:** `ux`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Поддерживаемый UI/CLI исследуют минимум пять пользователей, не участвовавших в проектировании, включая keyboard и assistive navigation сценарии.
- **WHEN** Дать одинаковые задания: понять wait, reject, stop, отличить pause/cancel, восстановить known failure и распознать unknown/waiver.
- **THEN** Действия имеют labels, focus и textual status; критические неверные трактовки зафиксированы, исправлены и проверены повторно; пять участников не названы статистическим доказательством рынка.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Смысловая оценка не подменяет проверку исполнения

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/quality-and-acceptance/`, `openspec/specs/product-model/`
- **GIVEN** AI draft имеет rubric/corpus и human review, а mechanical checker требует конкретных bytes/schema; модель пишет убедительное «проверено».
- **WHEN** Сопоставить report с actual tool receipts, checker results, model/configuration и scope semantic review.
- **THEN** Отдельно показаны исполнение, schema verification и смысловая оценка; prose не доказывает test pass; повторный model ID не доказывает независимость; ограничения и разрешение на внешнюю передачу данных явны.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Explain и replay не повторяют внешнюю публикацию

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/domain-execution/`, `openspec/specs/cli-protocol/`
- **GIVEN** Журнал содержит выполненную публикацию и pinned predicates; один старый input удалён по допустимой retention policy.
- **WHEN** Вызвать explain/replay и запросить cache reuse на изменённом subject.
- **THEN** Объяснение связано с event sequence и версиями; эффекты не исполняются, отсутствующие prerequisites явно ограничивают replay; изменённый subject не получает чужой cached pass.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Разные предметные сценарии выражаются одной моделью

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/control-security-ux/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/product-model/`
- **GIVEN** Набор workflows включает transform, research, document review/publish, batch, human request, incident, optional coding, schedule, parallel workers и correction of intent.
- **WHEN** Пройти normal path и указанное исключение каждого сценария на подходящем qualified profile.
- **THEN** Не нужны core branches по названию проекта/шага; отказ, изменённый source, duplicates, unknown и новая цель дают предусмотренные outcomes; отсутствие готового отраслевого пакета не скрыто выдуманным выполнением.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Acceptance traceability отделена от выполненных тестов

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/quality-and-acceptance/`, `openspec/specs/cli-protocol/`, `openspec/specs/control-security-ux/`
- **GIVEN** Поставка спецификации содержит requirements, схемы, fixture validation report и этот каталог; runtime qualification report ещё отсутствует.
- **WHEN** Проверить ссылки case→requirement, Given/When/Then и claims в итоговом документе.
- **THEN** Каждое требование01–08 имеет содержательную проверку; static/schema validation не названы runtime pass; status specified_not_executed сохранён, дополнительные RT-AC/package/control cases не отменены.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Quick start проверен на реально поставленном build

- **Priority:** `P1`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`, `openspec/specs/cli-protocol/`
- **GIVEN** Версия документации соответствует protocol/build; приведены empty install, свой step, condition, map/repeat, wait, stop/reconcile, export/restore и uninstall.
- **WHEN** Воспроизвести advertised commands на чистом поддерживаемом profile без скрытых credentials и файлов автора.
- **THEN** Примеры проходят validation и заявленные действия; внешние dependencies/fixtures обозначены; universal quick start не требует SMSPlace, /TZ, PHPUnit, worktree либо AI Factory.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Универсальный release работает в трёх обязательных конфигурациях

- **Priority:** `P0`
- **Verification kind:** `runtime_integration`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/product-model/`, `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`
- **GIVEN** Есть release artifact и чистая машина без LLM credentials, Git repository, внешнего account и отраслевых пакетов.
- **WHEN** Проверить пустую установку, deterministic workflow без task/Git/AI и установку/запуск собственного небольшого пакета.
- **THEN** Все три конфигурации исполняют свой контракт без изменения core; coding demo не заменяет их; профиль заявляется qualified лишь при выполненных applicable P0/P1 и отсутствии открытых критических дефектов.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Qualification не наследует невозможные гарантии

- **Priority:** `P0`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`, `openspec/specs/runtime-resources/`
- **GIVEN** Профили core-local, managed-isolated и optional team-remote имеют различный evidence; target adapter не умеет durable dedup/fencing либо readback.
- **WHEN** Проверить capability matrix и запрос workload, которому нужна отсутствующая гарантия.
- **THEN** Гарантия не объявлена по имени profile/framework; unsafe workload blocked или явно сужен до допустимого режима; нет blanket exactly-once, мгновенного undo или formal proof без соответствующего доказательства.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Handover связывает результаты с конкретной поставкой

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/quality-and-acceptance/`, `openspec/specs/architecture-decisions/`, `openspec/specs/product-model/`
- **GIVEN** Подготовлен candidate release с build digest, protocol/storage/package inventory, qualification matrix и известными ограничениями.
- **WHEN** Проверить actual evidence всех applicable P0/P1, migration/restore drill, operator runbook и rollback criteria.
- **THEN** Design, implementation и qualification отмечены отдельно; отсутствие реализации не названо not applicable; unresolved auth/data-loss/duplicate-effect/false-success defect не waived ради объявления готовности.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Owner defaults не блокируют пустой core и не выдают лишних прав

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/architecture-decisions/`, `openspec/specs/product-model/`
- **GIVEN** Владелец ещё не выбрал AI provider, remote identity, isolation, retention и paid workload; имеется минимальная локальная установка.
- **WHEN** Проверить default configuration и затем запросить capability с обязательным нерешённым owner decision.
- **THEN** Core и безопасный deterministic сценарий доступны; неопределённая чувствительная capability disabled/blocked; указан accountable owner и требуемое решение, право не угадано из package intent.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Лицензия, происхождение и runtime trust проверяются отдельно

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/architecture-decisions/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Package имеет content digest и подпись; лицензия отсутствует либо ограничивает распространение; второй пакет имеет допустимую лицензию, но не доверен для исполнения.
- **WHEN** Проверить release inventory, notices и installation/admission policy.
- **THEN** Подпись не доказывает право на распространение или безопасность; license obligations отражены в поставке; допустимая лицензия не расширяет tool rights и не отменяет trust/quarantine checks.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Границы модулей не требуют лишних сервисов или бесконечного SDK

- **Priority:** `P2`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/architecture-decisions/`, `openspec/specs/product-model/`, `openspec/specs/workflow-and-context/`
- **GIVEN** Reference build разделяет core, package, adapter и config логически; предложено добавить broker/cluster или менять core ради одного нового tool.
- **WHEN** Сопоставить dependency graph и расширение с ADR trigger, measured workload и публичным минимальным worker/adapter protocol.
- **THEN** CLI/library, journal и artifacts работают без обязательных Kafka/Temporal/Kubernetes; прикладное поведение остаётся в package/adapter; новый инфраструктурный слой требует конкретной причины, не предположения о будущем масштабе.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

#### Scenario: Пилот даёт принципы, а не обязательные stage names

- **Priority:** `P1`
- **Verification kind:** `design_review`
- **Declared status:** `specified_not_executed`
- **Subject paths:** `openspec/specs/architecture-decisions/`, `openspec/specs/product-model/`, `openspec/specs/workflow-and-context/`, `openspec/specs/quality-and-acceptance/`
- **GIVEN** Исторический источник — dirty working tree codex/aif-task-close-v23-pilot на HEAD620d1eca85a8eb650a3f46ecc9d94aa4b9c410b0; целевая поставка создаётся независимо.
- **WHEN** Проверить imports, distributable paths, quick start и workflow definitions, отключив все AIF/SMSPlace adapters.
- **THEN** В core нет обязательных зависимостей от исходных helpers, GitLab, /TZ, task-close lifecycle или файлов пилота; сохранены deterministic control, typed evidence и явные границы; исследовательский source не изменён ради поставки.
- **Evidence boundary:** статус описывает будущую проверку; квалификация требует отдельного отчёта для конкретных build и profile.

### Requirement: Неподтверждённая гарантия остаётся ограничением

Pri-Fly MUST NOT заявлять полный domain coverage, universal AI correctness,
exactly-once for arbitrary APIs, rollback completed effect, hostile-code
isolation through Git worktree или distributed coordination from local SQLite.
Unsupported profile, bounded protocol, human decision, reconciliation или
versioned extension MUST описывать каждую такую границу.

#### Scenario: External API не подтверждает idempotency

- **WHEN** provider не даёт заявленную гарантию exactly-once
- **THEN** product показывает uncertainty и reconciliation вместо false guarantee
