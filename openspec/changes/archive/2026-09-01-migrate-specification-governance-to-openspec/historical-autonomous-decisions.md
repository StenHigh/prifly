# Журнал автономных решений

Владелец попросил продолжать разработку Pri-Fly по roadmap во время его
отсутствия и записывать решения для совместного разбора после возвращения.
Этот файл хранит рабочие развилки: контекст, выбранный путь, отвергнутые
альтернативы и условие пересмотра. Решения, меняющие архитектурный контракт,
по-прежнему оформляются отдельными ADR в `docs/decisions/`.

## 2026-09-01 — общая терминология входит в OpenSpec governance

**Контекст.** При инвентаризации перед переходом на OpenSpec выяснилось, что
`docs/glossary.md` и contributor-инструкции содержат нормативные определения,
но первоначальный список путей миграции называл только продуктовые главы.
Удалить эти файлы вместе с legacy-документацией без ownership означало бы
потерять правила, а отдельная capability только ради их хранения раздробила бы
вход в спецификацию.

**Решение.** Их перенос входит в уже существующую capability
`specification-governance`: она станет единым местом для правил изменения
спецификации и канонической терминологии. В source map это явно отмечено как
частичный перенос до отдельного focused change; текущие файлы не объявляются
устаревшими заранее.

**Не выбрано.** Не создавать искусственный раздел «термины» и не оставлять
нормативный glossary вне OpenSpec после release cleanup.

**Пересмотреть.** Если vocabulary вырастет до самостоятельного публичного
контракта с отдельным жизненным циклом, выделить его новой capability change,
а не задним числом размножать определения.

## 2026-08-31 — общий compiler не зависит от AI Factory

**Контекст.** Первоначальный удобный profile копировал AI Factory recipe при
`project init`. Это делало продукт неуниверсальным: большинство проектов не
используют AI Factory, а project compiler не должен содержать её модель.

**Решение.** Standard `project init` создаёт пустой profile `/2` с каталогами,
`packages: {}` и `launches: {}`. `project compile` понимает только нейтральный
`prifly-package-source/1` и не содержит AI Factory ID, skill, graph или launch.
AI Factory может позднее использовать его как внешний клиент, но не поставляется
и не выбирается ядром.

**Не выбрано.** Не оставлять AI Factory как скрытый default и не заменять
универсальный compiler вторым AIF-specific compiler. Это не отменяет внешний
recipe: он просто не является частью стандартной установки.

**Пересмотреть.** Добавить готовый recipe template можно только как явно
выбранный отдельный пакет, не как эффект `project init`.

## 2026-08-31 — launcher использует local executable, а не глобальный PATH

**Контекст.** Первый запуск пилотного project profile честно остановился:
Codex прочитал GitLab work item, но не нашёл команду `prifly` в PATH. Profile
уже знал local authority, однако без пути к binary это не давало host способа
исполнить созданный сценарий.

**Решение.** `project init` записывает в ignored `local.yaml` canonical путь
к тому executable, который создал profile, вместе с authority root. Generated
`prifly-run` обязан использовать `prifly_executable`, а не предполагать
глобальную установку. Путь machine-local и поэтому не попадает в Git.

**Не выбрано.** Не копировать binary в repository, не добавлять его в Git и не
менять PATH пользователя. Это не installer, updater или credential store.

**Пересмотреть.** При поставке настоящего package installer заменить запись
точного пути на установленный stable launcher только вместе с миграцией уже
созданных profiles.

## 2026-08-31 — первый launcher остаётся host-навыком, а recipe поставляется с binary

**Контекст.** Владелецу нужен обычный запуск из repository без ручных
`session task`/`session submit`, но ядро Pri-Fly намеренно не запускает модели.
Существующий проверенный AI Factory recipe был в исходниках рядом с binary, что
ломало обычную установку без checkout Pri-Fly.

**Решение.** `project init` копирует в tracked profile exact recipe и YAML
source из embedded binary, создаёт Codex skill `prifly-run` и записывает
machine-local authority path. Skill — host: он читает задачу, создаёт Run и
исполняет handoff, а человек отвечает только на вопросы сценария. Временный
package source создаётся рядом с authority, потому что import отвергает source
внутри authority. Первый запуск импортирует sealed package и устанавливает
capacity двух рецензентов.

**Не выбрано.** Не вызывать LLM из Go, не маскировать Python recipe как часть
runtime, не запускать GitLab/GitHub/Jira adapter и не публиковать обратно в
трекер. Не вводить generic plugin/profile framework до первого живого опыта.

**Пересмотреть.** После реального project run определить один read-only
source adapter. TaskInput/1 реализован отдельным срезом. Отдельно решить
update/versioning package при смене team skill: exact package не может
незаметно заменить bytes под тем же id/version.

## 2026-08-31 — приём задач отделён от roadmap и исполнения

**Контекст.** Владелец ожидает один пользовательский запуск работы из чата,
GitLab, GitHub, Jira или будущей системы, но AI Factory `ROADMAP.md` ведёт
стратегические milestones, а не очередь внешних issue.

**Решение.** Принят ADR-0020: будущий read-only source adapter до RunStart
выдаёт единый `TaskInput/1`; владелец выбирает repository и необязательную
связь с milestone AI Factory. Pri-Fly не заводит второй roadmap и не знает
протоколы провайдеров. Любая запись обратно во внешний сервис остаётся
отдельным подтверждённым внешним действием.

**Не выбрано.** Не зашивать GitLab в core, не считать status issue статусом
Run и не обещать уже работающий launcher, интеграцию или параллельное
автоматическое создание AI-сессий.

**Пересмотреть.** Перед первым provider-adapter срезом определить capability,
credential boundary и acceptance tests; `TaskInput/1` и local intake уже
реализованы. Начать с chat или GitLab, не создавая общего plugin framework
заранее.

## 2026-08-31 — единый вход задачи реализован раньше provider adapters

**Контекст.** Launcher мог сохранять source bytes и вручную составлять
RunBrief, но этот путь не давал будущим источникам один проверяемый формат и
заставлял host повторять преобразование.

**Решение.** Добавлен закрытый `TaskInput/1` и `prifly task prepare`. Команда
проверяет input, запечатывает exact bytes как SourceSnapshot, проверяет уже
переданные source refs и выводит RunBrief в local authority. Host launcher
теперь использует `brief_path` ответа. Данные из чата и трекера имеют ту же
границу; исходный raw tracker response можно добавить отдельным source ref.

**Не выбрано.** Не создаются GitLab/GitHub/Jira adapter, capability/plugin
framework, credential store, task list/polling/webhook или внешняя запись.
`TaskInput.source` остаётся declared provenance, не правом на сеть.

**Пересмотреть.** После первого реального запуска выбрать один read-only
adapter по источнику задач владельца; только тогда проверять нужную capability
и credential boundary.

## 2026-08-31 — общий profile проекта не смешивается с локальной authority

**Контекст.** Команде нужны одни и те же reviewed project steps, workflows и
skills после `git pull`; однако core правильно запрещает authority внутри
repository, который writing steps получают в worktree.

**Решение.** Принят ADR-0021: versioned profile располагается в
`<repository>/.prifly/`, skills — в tracked `<repository>/.codex/skills/`, а
state/artifacts/worktree/receipts остаются в local authority storage вне Git.
Существующий raw CLI не переопределяется задним числом: его `--project` всё
ещё authority root.

**Не выбрано.** Не дублировать skills, не коммитить SQLite/artifacts/worktree
и не ослаблять один exclusive claim ради одновременных задач в одном repo.

**Пересмотреть.** Реализовать launcher и profile parser отдельным срезом;
перед поддержкой parallel tasks одного repository определить независимый
claim/merge contract.

## 2026-08-31 — profile и launcher блокируют внедрение Pri-Fly

**Контекст.** Владелец подтвердил, что без versioned profile существующий
проект нельзя подключить без смешивания Git-материалов с local authority data,
а без launcher workflow требует от человека исполнять внутренний session
protocol вместо ответов только в точках решения.

**Решение.** Следующими выполняются два связанных пользовательских среза:
сначала безопасный project profile setup с отдельным local authority root,
затем launcher, который принимает задачу из чата или source adapter и сам
ведёт AI Factory workflow до человеческого вопроса. Они имеют приоритет над
следующими P2-09 расширениями, внешними effect delivery и новыми операторами.

**Не выбрано.** Не предлагать ручные `session task`/`session submit` как
обычный способ использования и не добавлять сначала GitHub/Jira adapter или
parallel tasks одного repository: это не снимает основной барьер внедрения.

**Пересмотреть.** После живого запуска в существующем repository выбрать
следующий adapter по реальному спросу; parallel tasks одного repository
планировать отдельным isolation/merge срезом.

## 2026-08-30 — один накопительный журнал рядом с ADR

**Контекст.** Формальные ADR уже описывают принятые продуктовые и архитектурные
решения, но использовать их для каждой автономной рабочей развилки означало бы
смешать договор продукта с журналом исполнения.

**Решение.** Небольшие решения агента записываются здесь до реализации. Если
решение меняет публичный договор, термин или порядок roadmap, сначала создаётся
отдельный ADR и синхронизируются все нормативные источники. Результаты тестов и
границы готового среза остаются в `docs/f2-progress.md`, чтобы этот журнал не
стал третьей системой статусов.

**Не выбрано.** Не меняются `milestones.csv` и прошлые release evidence: они
фиксируют приёмку, которой автономное продолжение само по себе не проводит.

**Пересмотреть.** После возвращения владельца пройти записи по порядку; спорное
решение исправлять новым срезом и новой записью, не переписывая историю.

## 2026-08-30 — первая RC останавливает расширение полного F2

**Контекст.** После работающего core AIF recipe следующей по номеру оказалась
P2-09: managed ActionIntent, admission, delivery, credentials и remote
executor. Уже был добавлен только strict parser sealed `ActionIntent`, но
дальнейший ledger не участвует в заявленном local owner-host цикле. Одновременно
пользователь попросил разделить критичное для RC и то, без чего можно обойтись.

**Решение.** До квалификации первой `core-local` RC не расширять P2-08--P2-18
без найденного пробела в самом RC пути. Считать готовыми инженерные доказательства
clean-project/YAML и improve/review; оставшимися работами считать физический
OBS-AC-06 suspend/resume и единственный release gate после него. P2-09 остаётся
на parser boundary, P2-11--P2-16, retry/reconcile/compensation, remote executor,
retention и full operator surface — в очереди после RC. Полная таблица и
критерии находятся в `docs/rc-scope.md`.

**Не выбрано.** Не объявлять P2 этапы принятыми и не выдавать RC за полный F2.
Не запускать тяжёлые full gates до стабилизации кандидата: они не заменяют
hardware evidence и не обнаруживают новый факт при каждом documentation или
узком parser-срезе.

**Пересмотреть.** Вернуть отложенную capability раньше только при конкретном
local сценарии, который не проходит без неё, с новым минимальным test/evidence;
после живого OBS-AC-06 начать RC-4 и зафиксировать результат release gate.

## 2026-08-30 — действие расходует только один Grant с точной областью

**Контекст.** P2-09 требует, чтобы допуск exact ActionIntent мог расходовать
Grant, но v19 уже опубликован как approval-only. Несколько grants без правила
композиции позволили бы неясно расширить область решения.

**Решение.** Выпущен новый v20: допускается один `action.admit` Grant, и все
targets должны буквально входить в его resource scope. Счётчик Grant,
обязательный Approval и ActionAdmission меняются одним commit. v19 не меняется.

**Не выбрано.** Не добавлять delivery, credentials, remote execution и не
компоновать несколько grants до отдельного правила их пересечения.

**Пересмотреть.** Когда появится реальный сценарий нескольких независимых
ресурсных полномочий, определить составное правило и выпустить следующую версию.

## 2026-08-30 — подготовка действия отделена от его запуска

**Решение.** Core v21 записывает prepared delivery одновременно с admission.
Это доказывает только готовность к будущей отправке; внешнего вызова, ключей
доступа и receipt ещё нет.

## 2026-08-30 — bootstrap commit recipe не наследует global GPG policy

**Контекст.** Сквозной RC test начал запускать `aif-cycle.py` как пользователь
после чистого `init`, а не вызывать его внутренние функции. На машине с global
`commit.gpgsign=true` seed commit временного repository отказал, хотя у recipe
нет ключа и подпись не является пользовательским изменением.

**Решение.** Только synthetic initial commit вызывает Git с
`-c commit.gpgsign=false`. Последующие commits принадлежат assisted host и
сохраняют его обычную Git policy. Общую project configuration API не добавлять:
recipe уже применяет свои declared limits сама, без ручной правки пользователя.

**Не выбрано.** Не отключать подпись глобально, не менять user Git config и не
подменять bootstrap новым приватным Git helper.

**Пересмотреть.** Если package начнёт создавать commits от имени пользователя,
явно описать signing identity и evidence; этот exception остаётся только для
одноразового disposable seed.

## 2026-08-30 — стоимость принимается как свидетельство, а не вычисляется ядром

**Контекст.** Авторский порядок требует первым делом показать стоимость
конкретного шага, но прямо запрещает собственной таблицей цен пересчитывать
usage. Источник может промолчать; два источника могут назвать разные суммы.

**Решение.** Ассистируемый хост может вместе с terminal result передать до
восьми `ReportedCost`: точную неотрицательную десятичную строку, валюту и
самоназванный `source`. Записи сохраняются на `Attempt`, которая уже однозначно
связана со StepInstance. Несколько источников остаются отдельными; ядро не
выбирает «правильный», не складывает валюты и не считает сумму из токенов.
Пустой список означает «не наблюдалось», а строка `0` — явно сообщённый ноль.
Публичные state/read/next и assisted-session contracts получают новые версии;
прежние bundles и сохранённые Runs остаются неизменными.

**Не выбрано.** Не вводятся rate card, интеграция с Claude/Codex/LiteLLM, OTLP,
usage units, бюджет, пересчёт сессионного cumulative counter, агрегат по Run и
поздние billing adjustments. Поэтому capability называется `reported_cost`, а
`provider_usage` остаётся честно неподдержанной.

**Пересмотреть.** Когда появится отдельный adapter intake либо P2-16 cost view,
переиспользовать запись Attempt и добавить provenance/adjustment contracts;
не расширять смысл уже принятого `reported-cost/1` задним числом.

## 2026-08-30 — P2-12 продолжается с раннего artifact intake

**Контекст.** State/event variants `step.publish`, аутентификация текущей
Attempt и immutable artifact store уже поставлены. Следующий нормативный путь
PUB-003 требует сначала принять и запечатать отдельный готовый item; подписка
PUB-004 может безопасно ссылаться только на такую запись. Внешние mutable
sources требуют отдельного authority/watch contract и не должны быть
замаскированы под worker publication.

**Решение.** Добавить versioned artifact variant той же операции
`step.publish`: StepDefinition объявляет JSON/blob type, `one` либо
`keyed_many`, предел bytes и разрешение раннего потребления; команда называет
workspace-relative candidate, ожидаемые digest/size и logical item key. Core
копирует и проверяет bytes вне DB transaction, затем повторно проверяет
publisher generation/stop и атомарно фиксирует `ArtifactPublication`. Exact
command retry должен вернуть прежний receipt, не перечитывая изменившийся
файл. Общая SQLite/EventEnvelope остаётся v1: новый смысл различают versioned
команда, Run/read DTO и `artifact-publication/1`, а не новая версия оболочки.

**Не выбрано.** В этот первый срез не входят named subscriptions, consumer
admission, close/manifest, per-item executable checks и внешние source
bindings/watch. Hook с объявленными content checks будет отклонён как
unsupported, а не принят без проверки.

**Пересмотреть.** Следующий срез связывает принятую публикацию с одним заранее
объявленным consumer; после живого overlap расширить до bounded
`each_publication` и explicit close. External observations остаются отдельной
веткой P2-12 после публикационного data path.

## 2026-08-30 — единичный clock refusal не ослабляет deadline contract

**Контекст.** Первый финальный race-gate дважды отказал repeat-fixtures с
`deadline_clock_rollback`: реальная wall clock машины отстала от monotonic clock
больше разрешённых 2 мс. Изменённый artifact path в этих сценариях не
участвует.

**Решение.** Не расширять допуск и не увеличивать executor timeout без
воспроизводимого продуктового дефекта. Оба упавших сценария прошли по пять раз
под race (10 запусков за 176.082 с), затем неизменённый код прошёл полный
race-runtime за 797.970 с и весь `make check` за 1020.13 с. Первый отказ
считается наблюдённой внешней коррекцией часов, а не основанием ослабить
fail-closed проверку.

**Пересмотреть.** Если отказ повторится на стабильных часах либо станет
воспроизводимым синтетическим clock test, выделить отдельный срез для clock
contract/test harness; не смешивать его с artifact publication.

## 2026-08-30 — первый subscriber остаётся одноразовым wait, а не заготовкой очереди

**Контекст.** PUB-004 требует и простой `once`, и потоковый
`each_publication`. Для второго прямо нужны долговечный cursor, отдельные
assignments и explicit close; существующий nonce-based Wait этого не умеет.
Однако для первого Run уже хранит ровно нужные reservation, inbox delivery и
единственный immutable event ref.

**Решение.** Поставить минимальный честный `publication_subscription_once`:
immutable `publication-source/1` связывает direct sibling producer
branch/stage/artifact hook/item с обычным wait в consumer branch. Активная либо
зарезервированная доставка назначается вместе с authority commit публикации;
раньше принятую публикацию подбирает declared `retained` при входе wait.
External event intake не обслуживает core-owned source. Реальный overlap
проверяется двумя assisted sessions при capacity 2: producer остаётся
`awaiting_host`, когда consumer уже получает admission с exact sealed bytes.
Полный контракт и границы записаны в [ADR-0011](decisions/0011-once-publication-subscription.md).

**Не выбрано.** Не добавлять пустой `Subscription` DTO/state version, cursor,
close или новый operator «на будущее». Не заявлять local-process overlap:
foreground driver синхронно ждёт его exit. Не снимать общий маркер
`subscriptions`; поддержанный subset получает отдельное capability name.

**Пересмотреть.** Следующий публикационный срез вводит настоящий handle/cursor,
assignment ledger и close только одновременно с живым bounded
`each_publication` lowering. Отдельно можно сделать async local driver, если
нужна квалификация overlap именно для managed processes.

## 2026-08-30 — ранняя доставка требует согласия автора hook

**Контекст.** `StepDefinition v3` разрешает только `read_policy=owner`.
Publication source выбирает exact sibling и тем самым описывает намерение автора
workflow, но не может сам выдать право на чужой hook: это обошло бы отдельную
проверку доступа из PUB-003/PUB-AC-13.

**Решение.** Оставить опубликованный v3 неизменным и выпустить минимальный
`StepDefinition v4`, который для artifact hook добавляет ровно одну политику
`declared_subscribers`. Compiler принимает publication source только при этой
политике; exact source и direct sibling composition затем ограничивают, кто
получает ArtifactRef. State/event hooks пока сохраняют `owner`.

**Не выбрано.** Не считать `early_consumption=true` разрешением на чтение: это
свойство момента готовности, а не субъекта доступа. Не добавлять общий ACL,
roles или свободный список readers до сценария, которому они действительно
нужны.

**Пересмотреть.** Расширять read policy только вместе с новым проверяемым
способом подписки или внешнего чтения; знание digest либо имени hook права не
даёт.

## 2026-08-30 — доставка публикации повышает RunVersion ровно один раз

**Контекст.** Обычный `CommandPublication` намеренно обходит глобальный CAS и
не повышает RunVersion: state/event hook меняет свой namespace, а не маршрут
workflow. Но active/reserved delivery меняет inbox/assignment и может сразу
перевести consumer на следующий stage. Оставить прежнюю версию означало бы,
что уже подготовленная CAS-команда не заметит изменившийся frontier.

**Решение.** Сохранить прежнее поведение чистой публикации, но разрешить её
authority transform явно поставить `AdvanceRunVersion`. Store повышает версию
один раз для всей атомарной пары publication+deliveries; события этой команды
несут новую версию. Exact receipt-only retry и публикация без назначенного
subscriber версию не меняют.

**Не выбрано.** Не делить принятие item и delivery на две транзакции и не
требовать от producer глобальную expected version: первое создаёт окно потери,
второе снова связывает независимую публикацию с чужими heartbeat/CAS.

**Пересмотреть.** Если будущий stream принимает item без немедленной
assignment, его commit остаётся publication-only; RunVersion двигает отдельное
durable claim/activation, а не само наличие bytes.

## 2026-08-30 — explicit close поставляется до cursor loop, но не выдаётся за stream

**Контекст.** Точный PUB-004 lowering требует долгоживущий subscription handle,
cursor, tagged `Item | Closed | Interrupted` и отдельный assignment ledger.
Текущий `wait` намеренно хранит один EventRef и после resolution заканчивается;
расширить его парой необязательных полей означало бы повторить запрещённую
подмену очереди одноразовым ожиданием. При этом PUB-003/006 close — отдельная
атомарная граница producer: без неё будущий loop не отличит EOF от тишины.

**Решение.** Следующим узким срезом выпустить `step.publish` v3 variant `close`
только для `keyed_many` artifact hook. Producer предъявляет полный упорядоченный
список item keys; authority сравнивает его со всеми уже принятыми publications,
строит и запечатывает manifest с точными publication/ArtifactRef, count и cut,
затем фиксирует единственный ArtifactClosure. После commit новые items канала
запрещены; exact retry возвращает прежний receipt, а logical retry с новым
command ID — новый receipt с тем же ArtifactClosure в result. Close остаётся
scoped publication и сам по себе не двигает workflow frontier.

**Не выбрано.** Не считать timeout, terminal producer или пустую очередь EOF.
Не добавлять cursor/assignment поля без исполняемого repeat-сценария. Не
принимать один count вместо exact ordered membership: потерянный либо
переставленный item должен дать конфликт, а не правдоподобный manifest.

**Пересмотреть.** Сразу после этого среза `publication-source/2` и typed
subscription binding должны опустить `each_publication` на
`repeat + wait + choice + call` и доказать два items, closure iteration и
независимые cursors. До такого теста generic `subscriptions` остаётся
unsupported.

## 2026-08-30 — v13 исправляет activation kinds, старые bundles не переписываются

**Контекст.** Живой closure test использует настоящий `parallel → wait` Run и
обнаружил, что опубликованные additive bundles v7–v12 сохранили repeat-era enum
Activation.kind: только `step|choice|call|repeat|finish`. Runtime всё это время
записывал также `parallel`, `map` и `wait`; поэтому такой старый state не
проходит собственную опубликованную schema. Это предшествующий дефект
контракта, а не следствие close.

**Решение.** Не менять bytes уже опубликованных bundles и не перештамповывать
их hashes. Новый v13 bundle перечисляет все восемь реально записываемых kinds и
проверяется живым `parallel/wait` Run вместе с exact closure. Историческое
расхождение v7–v12 остаётся явно известным compatibility erratum: исправить его
задним числом без новой версии невозможно.

**Не выбрано.** Не ослаблять живой тест до flat fixture и не менять старые JSON
ради зелёной проверки. Не мигрировать сохранённые Runs молча на v13: версия
state является частью их истории.

**Пересмотреть.** Перед release опубликовать явную политику errata/reader
fallback для затронутых старых Runs; если требуется машиночитаемый replacement,
он должен получить новый contract ID, а не новые bytes под прежним ID.

## 2026-08-30 — assisted publish разбирает тот же raw closed contract

**Контекст.** HTTP `step publish` сохранял presence полей через общий parser, а
`session publish` сначала декодировал JSON прямо в Go struct. В такой форме
некоторые explicit `null` могли исчезнуть в zero value до variant-проверки.

**Решение.** Экспортировать единственный `ParsePublishCommand` и использовать
его и для Unix HTTP, и для assisted CLI до typed transport. Новый close не
получает более слабую wire boundary только потому, что producer удерживает
session, а не process token.

**Не выбрано.** Не дублировать public-schema validator в CLI и не заводить
второй command DTO для одного wire protocol.

## 2026-08-30 — item refs manifest не дублируются в generic provenance

**Контекст.** Run допускает до 1024 publication facts, но неизменяемый
`ArtifactRevision` v1 ограничивает generic `provenance` 256 ссылками. Если
переписать все `manifest.items[].artifact_ref` туда второй раз, корректный
exact close скрыто перестаёт работать уже на 257 items.

**Решение.** Полным dependency set closure является typed и проверяемый
`ArtifactManifest.items`; metadata самого manifest artifact оставляет generic
provenance пустым. Состав не теряется и не обрезается, а допустимый предел
close не зависит от меньшего лимита другого поля.

**Не выбрано.** Не ужимать close до 256 items, не менять опубликованный
`ArtifactRevision` v1 и не строить иерархию промежуточных manifests только ради
дублирования тех же ссылок.

**Пересмотреть.** В P2-16 retention/GC должен явно pin-ить refs из typed
manifest. До этой квалификации текущий срез не обещает сборку мусора.

## 2026-08-30 — stream получает новые bindings и свой ledger, а не скрытую очередь wait

**Контекст.** `publication-source/1` честно доставляет один exact artifact через
одноразовый `WaitRegistration`. Для `each_publication` этого недостаточно:
subscription должна переживать итерации, cursor — переходить через
`next_bindings`, а pending item — оставаться тем же при retry consumer. Кроме
того, tagged delivery нужна `choice`, но JSON с вложенным `ArtifactRef` сам по
себе не делает bytes item входом consumer.

**Решение.** Выпустить additive `WorkflowRevision v3`,
`publication-source/2` и state/read family v14. Repeat binding
`from=subscription` материализует два typed значения одной durable подписки:
`handle` и начальный `cursor`; следующий cursor обязан прийти из
`iteration_output`. Stream-wait получает оба значения, создаёт отдельный
`PublicationAssignment` и отдаёт tagged `Item | Closed | Interrupted`.
`choice` читает tag, а новый binding `from=publication` передаёт consumer именно
закреплённый ArtifactRef pending Item. Compiler разрешает такой binding только
на ветке, где точный predicate доказал `kind == Item`. Cursor сдвигается и
assignment становится processed лишь после settlement тела repeat; до этого
новая публикация его не подменяет. У разных repeat activations разные
subscription IDs, cursors и ledgers.

**Не выбрано.** Не расширять смысл старого `WaitRegistration v1`, не отдавать
consumer wrapper вместо bytes, не вводить отдельный streaming scheduler и не
делать `map` дописываемым. В этом срезе initial mode остаётся `retained`, а
producer failure/тишина превращаются в tagged `Interrupted` только по конечному
deadline; `new_only`, immediate producer-failure wakeup, backpressure и GC
остаются отдельными проверяемыми срезами.

**Пересмотреть.** После живой проверки двух items, empty/non-empty close и двух
subscribers добавить `new_only` только вместе с reservation до producer effect;
не объявлять generic `subscriptions` и P2-12 закрытыми раньше этого.

## 2026-08-30 — control stage блокирует работа только своей invocation

**Контекст.** Живой stream прогон остановился перед consumer call, пока
producer Attempt продолжала работать в соседней parallel-ветви. `enterCall`,
`returnCall` и путь preparation failure проверяли глобальный `Run.Active`, хотя
admission и invariant уже используют scope-local `activeIn(invocationID)`.
Так любой активный sibling фактически запрещал call внутри параллельной ветви.

**Решение.** Во всех трёх control paths использовать существующую проверку
`activeIn` owning invocation. Работа в той же invocation по-прежнему исключает
второй frontier, а активная sibling/descendant invocation не получает права
блокировать чужой control stage. Live test оставляет producer running и
проводит оба subscriber calls, поэтому регрессия наблюдается через реальный
путь, а не только через helper.

**Не выбрано.** Не добавлять stream-specific обход и не очищать `Run.Active`:
это потеряло бы настоящую admitted работу и оставило бы общий дефект для других
parallel calls.

## 2026-08-30 — compiler обещает только direct repeat stream

**Контекст.** Композиционный proof различал лишь «внутри repeat» и «снаружи».
Поэтому второй вложенный repeat мог пройти compile, хотя runtime находит
producer через branch identity invocation, непосредственно владеющей первым
repeat; у вложенной iteration такой identity нет.

**Решение.** Считать repeat depth от direct subscriber branch и разрешать
`each_publication` wait только на глубине один. Call/map сбрасывают допустимую
композицию как раньше. Сам body принимается только в проверяемой форме: event и
timeout входят в один tagged choice, `Item` имеет ровно один
`from=publication` и ведёт прямо в call, `Closed` завершает repeat, а
`Interrupted` не делит с ним маршрут. Каждая продолжающая finish возвращает
authority `next_cursor` самого wait. Отдельные regression-проверки фиксируют
эти отказы; поддержанная direct форма остаётся доказана полным live прогоном.

**Не выбрано.** Не угадывать ancestor branch в runtime и не объявлять nested
stream без отдельного author contract, budget proof и живой квалификации.

## 2026-08-30 — читаемый YAML опускается в прежнюю WorkflowRevision

**Контекст.** Машинный JSON сохраняет точность, но человек не должен повторять
служебные поля, полные immutable refs и четырёхпольные bindings в каждом узле.
При этом отдельный «удобный» runtime-язык создал бы вторую семантику, а
агрессивные defaults могли бы незаметно изменить policy, маршруты или пределы.
AI Factory дополнительно получает refs шагов только после sealing внешних
skills, поэтому полностью готовый exact файл нельзя честно закрепить заранее.

**Решение.** Marker `authoring: prifly-workflow/1` включает только YAML-фасад.
Он даёт локальные имена exact refs, shorthand для JSON ports, bindings и
Predicate operands и безопасные структурные defaults; любое машинное поле
остаётся доступно полной формой. Один lowering выполняется до conditions,
compiler, alias resolution, package import, canonical digest и Run, поэтому
runtime и wire contracts не меняются. Главный AI Factory workflow хранится в
репозитории как `.yaml.tmpl`; recipe подставляет в него только полученные exact
refs и настраиваемые числовые ceilings. Вложенные package workflows также
выпускаются как authoring YAML.

**Не выбрано.** Не добавлять исполняемые YAML tags/anchors, environment
substitution, сетевой lookup refs, скрытый default policy/routes/loop ceilings
или новый runtime parser. Не переименовывать существующие JSON-поля и не
перештамповывать опубликованные schemas: полный JSON и полный YAML остаются
точным escape hatch.

**Пересмотреть.** Отдельную editor schema или formatter добавлять только при
реальном запросе IDE/каталога; её нельзя превращать в ещё один источник
семантики вместо lowering и canonical WorkflowRevision schemas.

## 2026-08-30 — deadline-тест ждёт запуска проверяемого процесса

**Контекст.** Полный race gate оставил deadline-вариант native checker в
`waiting`: fixture разрешала три секунды, но проверка требует увидеть старт
отдельного race-instrumented binary. Под нагрузкой gate срок мог закончиться
до `BeforeStart`, и тест измерял скорость cold start вместо остановки живого
процесса по исходному deadline.

**Решение.** Увеличить только test fixture allowance до десяти секунд. Сам
продуктовый checker deadline, протокол и сохраняемое доказательство срока не
меняются; тест по-прежнему требует actual start, затем точный cancellation или
deadline settlement. Два последовательных race-прогона прошли за 29.338s.

**Не выбрано.** Не увеличивать product timeout и не ослаблять assertion до
pending/never-started состояния: это скрыло бы поведение, ради которого тест
существует.

## 2026-08-30 — content checks проверяют pending sealed item, а не final StepResult

**Контекст.** `content_check_refs` уже входят в declaration artifact hook, но
ранний путь принимал только пустой список. PUB-003 требует проверить именно
copied/sealed bytes до visibility consumer, а glossary отдельно запрещает
приписывать отрицательному check изменение verdict producer.

**Решение.** Ввести один additive state/read family v15 с
`PendingArtifactPublication`, не отдельный scheduler. Existing CheckExecution
получает единственный новый boundary `artifact_publication`; pending record
закрепляет exact ArtifactRef и derived check IDs. Все pass evidence в одной
CAS создают обычный ArtifactPublication. Failure/inconclusive сохраняется как
check observation и event, очищает pending без publication/delivery и не
останавливает producer. Capability названа узко `artifact_publication_checks`:
generic `artifact_checks` не объявляется реализованным.

**Не выбрано.** Не хранить failed pending record, который блокировал бы Run
после завершения producer и запрещал явный следующий candidate. Не делать
automatic repair/retry, waiver или новый public command. Foreground
local-process driver всё ещё синхронно ждёт subprocess, поэтому физический
overlap publisher/check не обещан для managed process; проверяемый overlap
доступен лишь при independently live/assisted producer и capacity 2.

**Пересмотреть.** P2-16 должен дать общий reservation/backpressure proof и,
если нужен native overlap, async managed-process scheduling; не выдавать это
за свойство данного intake slice.

## 2026-08-30 — дорогие полные ворота не повторяются на каждом срезе

**Контекст.** После зелёного обычного полного `make test` новый общий
`make race` исчерпал 20-минутный лимит на суммарном runtime-пакете. Стек
указывал на прежний telemetry test во время JSON decode; изолированный
race-прогон этого теста прошёл за `8.101s`. Повторять весь многоминутный
набор после каждого малого исправления стало непропорциональной тратой
времени без нового диагностического сигнала.

**Решение.** По указанию владельца рабочие срезы подтверждать точечными
исполняемыми тестами затронутого поведения, schema freshness и `git diff
--check`; полный normal/race/examples набор запускать только на явной
product/release вехе или по прямому запросу. Непройденный общий gate всегда
записывать как неполный, а не заменять формулировкой о success.

**Не выбрано.** Не увеличивать постоянный Makefile timeout и не скрывать
общий race timeout повторными запусками. Если release gate вновь упрётся в
предел, отдельно профилировать пакет и менять его структуру/тайм-аут с
доказательством, а не в рамках продуктовой фичи.

## 2026-08-30 — `new_only` хранит cut authority, не timestamp producer

**Контекст.** Retained once/stream могли подобрать публикацию, уже лежавшую в
Run. Для `new_only` cursor не отвечает на вопрос, была ли запись видима до
регистрации: у двух subscriber'ов один и тот же cursor может иметь разные
границы права чтения.

**Решение.** Добавить отдельные immutable source contracts `/3` и `/4` и
сохранять event sequence Authority при регистрации в wait/subscription state
v16. Delivery принимает только запись строго после cut. Старые retained
contracts не получают новое поле и не меняют смысл; прежние state contracts
отвергают его явно. Это минимальная durable граница, которую можно проверить
после recovery без часов и догадок о producer workspace.

**Не выбрано.** Не резервировать будущий producer effect, не подменять тишину
немедленным failure wakeup и не вводить generic subscription scheduler. До
собственного среза new-only stream завершает ожидание существующим deadline.

## 2026-08-30 — terminal failure publisher прерывает только opt-in subscriber

**Контекст.** После new-only source authority могла уже знать, что sibling
producer terminally failed, но once/stream subscriber продолжал ждать timeout.
Это смешивало известный failure с отсутствием сигнала и не давало declared
recovery route выбрать причину.

**Решение.** Выпустить отдельные immutable `publication-source/5` и `/6` с
`interrupt_on_terminal_failure` и state/read v17. Только
`Invocation.Status=failed` создаёт payload-free once `producer_failed` либо
stream `Interrupted(producer_terminal_failed)`; handled `on_error` не
прерывает subscriber. Shared fan-out sync не выдаёт failure decision выше
ещё живой sibling, чтобы caller не становился terminal над активным child.

**Не выбрано.** Не менять v1–v4, не называть terminal failure timeout, не
переводить cancellation/unknown в failure и не добавлять final-dependent
commit/компенсацию. Эти политики потребуют отдельной модели evidence и
effects, а не расширения публикационного wait.

## 2026-08-30 — blob delivery получает отдельный source contract

**Контекст.** Artifact hook уже запечатывал blob и descriptor schema, но
publication-source v1–v6 и dataflow представляли delivery только JSON. Простое
расширение старых source нарушило бы их закрытый смысл, а forcing descriptor
schema into blob consumer port противоречит его wire schema.

**Решение.** Добавить immutable `publication-source/7` (once) и `/8` (stream)
с `format` и ровно одним allowed blob media type. Compiler требует совпадения
с hook; runtime принимает blob consumer input по format/media, поскольку
descriptor уже проверен при sealing. Старые v1–v6 остаются JSON-only.

**Не выбрано.** Не перештамповывать старые schemas, не вводить state v18 без
новой durable формы и не объявлять backpressure/retention. Boundary описана в
[ADR-0017](decisions/0017-blob-publication-delivery.md).

## 2026-08-30 — ToolDescriptor сначала импортируется как инертный контракт

**Контекст.** В protocol уже был закрытый ToolDescriptor, но package importer
отвергал `tool`, а local registry не мог закрепить его. Одновременно P2-13
требует ActionIntent/Admission/Delivery для безопасного retry: превратить
descriptor в исполняемое действие одним локальным retry означало бы повторять
неизвестный внешний эффект без receipt и reconciliation.

**Решение.** Разрешить в Core Registry3 и package inventory только sealing и
shape/identity validation ToolDescriptor. Парсер не ищет зависимости и не
выдаёт capability; descriptor остаётся PinnedDefinition до отдельного среза
ActionIntent. Повторная inventory validation удерживает тот же барьер для
ручного registry file.

**Не выбрано.** Не добавлять «безопасный» automatic retry, adapter dispatch
или credentials placeholder. Они создадут ложное свойство действия до
current policy, operation identity, receipt и uncertainty barrier.

## 2026-08-30 — первый RC отделён от полного F2

**Контекст.** Текущий backlog смешивал полезный local AI Factory путь с
managed actions, remote execution, эксплуатацией и полной qualification F2.
Последовательное закрытие всех P2 этапов не даёт раннему пользователю более
быстрый проверяемый результат и снова провоцирует дорогие широкие прогоны без
близкой release-вехи.

**Решение.** Зафиксировать отдельную границу `core-local AIF RC` в
`docs/rc-scope.md`: сначала закрыть F1 hardware gate, затем доказать clean
install/run AIF YAML и owner-driven cycles, safety/restart и один
воспроизводимый release gate. Полный F2, его статусы и требования не меняются;
это порядок engineering work, а не waiver.

**Не выбрано.** Не объявлять RC готовым по проценту кода и не переносить
external actions/retry в критический путь. Пока clean-project путь и OBS-AC-06
не доказаны, это только план работы, а не release evidence.

## 2026-08-30 — AIF happy path проверяется scripted host, не фальшивой моделью

**Контекст.** Recipe умел выдать первый assisted handoff, а прежний live test
доказывал только вторую передачу improve plan. Это оставляло без связанного
evidence порядок build/review/commit и человеческое решение по warnings.

**Решение.** Расширить один существующий temporary-project test до полного
owner-host walkthrough, который пишет exact declared output slots и вызывает
реальные CLI `session task`/`session submit`. В него входят accepted improve,
молчаливый exit после complete review, verify с warnings, explicit «не чинить»
и commit. Проверка permissions привязана к полученному handoff, а не к договору
о поведении test host.

**Не выбрано.** Не добавлять mock LLM, network fixture или новый runtime
adapter. Они проверяли бы другой продукт. Отдельный restart recipe test тоже
не добавлен: каждый CLI handoff уже открывает authority заново; ценность нового
среза появится только для fault injection внутри одной transaction.

## 2026-08-30 — OBS-AC-06 получает manual probe, но не выдуманное evidence

**Контекст.** Последний F1 gate требует настоящий sleep/wake native Mac.
Детерминированные clock tests проверяют модель, но не заменяют аппаратное
событие; агент не вправе усыплять рабочую машину владельца и объявлять
получившийся факт испытанием.

**Решение.** Добавить `scripts/verify-suspend-recovery.py`. Он готовит
изолированный temporary project, держит owned worker до явного operator wake,
записывает before/after/settled state и принимает evidence только если
calendar UTC включает заданный сон, а одна сохранённая Darwin monotonic domain
не выдаёт его за executor time. Existing evidence не перезаписывается.

**Не выбрано.** Не вызывать `pmset sleepnow`, не ставить автоматический sleep
timer и не менять статус OBS-AC-06. Пока оператор не выполнит физический шаг,
script — это средство квалификации, не её результат.

## 2026-08-30 — hardware gate RC не останавливает независимый полный roadmap

**Контекст.** RC-0 требует физического sleep/wake на машине владельца. Это
условие не может быть выполнено автономно, но исходная цель остаётся полной:
довести Pri-Fly по roadmap, а не только подготовить первый локальный кандидат.
Ранее записанное правило не брать новые P2-возможности до RC-0 превратило
внешнее ожидание в искусственную остановку всей разработки.

**Решение.** Сохранить RC-0 первым release blocker и не менять его статус без
evidence. Пока он ждёт владельца, продолжать только независимые P2-срезы с
отдельными узкими проверками и честной записью границ. Они не входят в RC и не
заменяют единственный RC-4 gate после hardware evidence.

**Не выбрано.** Не запускать физический sleep автоматически, не перештамповывать
старое evidence и не называть любое P2 изменение квалификацией RC.

**Пересмотреть.** Когда появится OBS-AC-06 evidence, остановить новый P2 срез
на его безопасной границе и пройти RC-4 на неизменяемом candidate.

## 2026-08-30 — пересмотр очереди: RC отделён от полного F2

**Контекст.** Повторная сверка roadmap, текущего `f2-progress` и живого AIF
recipe показала, что clean install/YAML, два improve-рецензента с передачей
исправленного плана и local owner-host safety уже имеют узкое evidence. В
старой рабочей очереди ещё оставались завершённые пункты ToolDescriptor и gap
audit, из-за чего список создавал ложное впечатление большого RC-объёма.

**Решение.** Единственные release blockers первого `core-local` RC — физический
OBS-AC-06 suspend/resume и один финальный RC-4 gate после него. RC-1--RC-3
переведены в режим защиты от регрессии, а не feature backlog. P2-08 scheduler,
P2-09 managed actions и остальные возможности полного F2 явно не входят в
candidate; их разрешено делать только отдельными срезами, пока физический gate
ждёт владельца. Рабочая таблица находится в `docs/rc-scope.md`.

**Не выбрано.** Не включать внешние действия, remote trust, retry, retention
или полный operator surface в RC ради процента готовности. Не повторять полный
gate до появления hardware evidence.

**Пересмотреть.** При OBS-AC-06 сначала заморозить candidate и выполнить RC-4;
если найден конкретный разрыв RC-1--RC-3, вернуть только этот разрыв в очередь
с минимальной проверкой.

## 2026-08-30 — ActionIntent сначала закрытый proposal, не действие

**Контекст.** ToolDescriptor описывает доступную операцию, но не её конкретные
arguments/target. Retry поверх одного ExecutionAdmission повторил бы неизвестный
effect, потому что у worker ещё нет отдельной logical operation identity.

**Решение.** Ввести strict runtime parser published ActionIntent до durable
ledger. Он фиксирует exact proposal и отвергает widening fields; admission и
delivery остаются следующими независимыми transitions.

**Не выбрано.** Не принимать ActionIntent как command, не записывать его без
owning Attempt и не включать dispatch. Это сохранило бы форму объекта, но не
дало бы нужного authority barrier.

## 2026-08-30 — durable ActionIntent копирует descriptor, а не pin-closure Run

**Контекст.** После того как assisted host выбрал operation, registry может
измениться или исчезнуть до будущего approval. В текущей модели Start
закрепляет executable closure workflow, но не все установленное множество
ToolDescriptor; требовать фиктивный `tool_refs` в StepDefinition означало бы
расширять authoring contract ради одного proposal ledger.

**Решение.** `session action` до Admission разрешает exact current trusted
ToolDescriptor и сохраняет проверенные canonical bytes прямо рядом с
ActionIntent. Новый Run с assisted step выбирает state/read v18; exact command
retry возвращает retained receipt даже если descriptor уже исчез из registry.
Stop/cancel и handoff ownership по-прежнему проверяются перед первой записью.
Подробная compatibility граница находится в [ADR-0016](decisions/0016-action-intent-proposal.md).

**Не выбрано.** Не добавлять `tool_refs` в workflow или StepDefinition, не
сохранять mutable path registry, не создавать ActionAdmission, delivery или
executor call. Это был бы другой, значительно более широкий transition.

## 2026-08-30 — ActionAdmission записывается вместе с consume Approval

**Контекст.** ActionIntent v18 сделал proposal durable, но отдельные
`ApplyAuthority` и Run transaction позволили бы Approval стать consumed без
admission либо наоборот. Это нарушило бы CTRL-006 и создало бы ложное право
после restart.

**Решение.** Добавить только внутренний reducer pinned AuthorityControl к
существующей SQLite `Apply`: accepted Run mutation и новый control snapshot
получают один cut и один commit; rejection и storage-budget rollback не
сохраняют ни один из них. Новый v19 Run хранит ActionAdmission с exact intent
digest и approval refs. `action.admit` использует текущий control pin, requires
active assisted Attempt и consumes approval в этом coupled commit. Появились
`action propose|admit`; прежний `session action` остаётся alias proposal.

**Не выбрано.** Не добавлять ActionDelivery, adapter call, credentials,
ActionReceipt, retry/reconcile или action Grant consumption. Поле `grant_refs`
явно отвергается: записать ссылку, которую ядро не расходует, означало бы
объявить несуществующую авторизацию.

## 2026-08-30 — ActionDelivery начинается с подготовленной записи, а не с вызова

**Контекст.** После ActionAdmission с точным Grant оставался разрыв между
решением authority и будущим обращением к target. Вызвать adapter внутри
admission transaction нельзя: SQLite commit не делает внешний effect атомарным,
а crash между ними нельзя выдавать за отсутствие действия.

**Решение.** Core v21 вместе с accepted ActionAdmission сохраняет одну
ActionDelivery в состоянии `prepared/not_started`. Она связывает exact
operation, owning Attempt, Admission и declared adapter до первого внешнего
вызова. Поэтому последующий dispatch будет отдельным наблюдаемым переходом, а
не скрытым побочным эффектом решения.

**Не выбрано.** Не выдавать credential, не вызывать adapter и не создавать
ActionReceipt в этом срезе. Без qualified executor и target-side evidence это
создало бы ложное утверждение о внешнем effect.

## 2026-08-30 — Завершение просроченной доставки отложено до нового договора

**Контекст.** У ActionDelivery уже есть срок, после которого нельзя начинать
внешнюю работу. Однако действующий исходный договор не содержит состояния
«срок истёк» и не допускает отдельной отметки такого наблюдения.

**Решение.** Не записывать новое конечное состояние и не выдавать его за
реализованную возможность. Будущий квалифицированный исполнитель обязан
проверять существующий срок до начала работы; durable завершение без запуска
можно добавить только вместе с отдельной версией исходного договора.

**Не выбрано.** Не называть отсутствие запуска транспортной ошибкой или
прерыванием: это создало бы ложный факт о том, чего не было.

## 2026-08-31 — Новый запуск создаётся отдельно от старого

**Контекст.** При изменении задачи нельзя продолжать прежний запуск так, как
будто его прежние согласования и рабочее состояние относятся к новой работе.

**Решение.** Fork создаёт отдельный Core Run в одной транзакции с проверкой
точной текущей версии исходного Run. В новый Run попадают только новые
закреплённые входы; старый output можно использовать лишь если он назван и как
reuse reference, и как вход нового Run. Старый Run, approvals, worker state и
границы внешних effects не изменяются и не наследуются.

**Не выбрано.** Не делать общий механизм доверенного reuse, cross-project
sharing или перенос права на внешний effect. Для них нужен отдельный договор с
проверкой срока, отзыва и области действия.

## 2026-08-31 — Reuse не переживает потерю доверия к исходному пакету

**Контекст.** Immutable output сам по себе не доказывает, что его можно
считать доверенным после quarantine или revocation пакета, который участвовал
в исходной работе. Проверить status только перед созданием Run недостаточно:
он может измениться до commit.

**Решение.** Fork с reuse отмечает точную текущую версию package authority
state вместе с control state. SQLite transaction повторно проверяет обе версии;
при изменении fork не создаётся и требует новой оценки. Quarantine, revocation,
removal и потерянная dependency делают reuse источника недопустимым.

**Не выбрано.** Не расширять этот барьер до общего reuse evidence. Сопоставление
существенных inputs, checker, context и freshness остаётся отдельной частью
P2-13, которой нужны явные declared dependencies.

## 2026-08-31 — Отзыв доверия читается в момент допуска

**Контекст.** Реестр пакетов, загруженный при открытии программы, намеренно
служит снимком для разрешения состава работы. Но если использовать тот же
снимок для security revocation, уже открытая программа не заметит отзыв до
перезапуска и допустит новую работу с отозванным компонентом.

**Решение.** Перед каждым допуском Run и Check читается текущее authority
состояние пакетов; только revoked package блокирует новый допуск. История
старых запусков остаётся неизменной.

**Не выбрано.** Не перезагружать весь реестр глобально и не приравнивать
quarantine или removal к revocation. Это разные жизненные состояния. Также не
заявляется новый атомарный договор для параллельной смены package state во
время другой записи: такая граница требует отдельного среза.

## 2026-08-31 — Просмотр пакета привязан к его точному манифесту

**Контекст.** Список пакетов полезен для обзора, но не отвечает на вопрос, что
именно было принято для одной конкретной identity. Поиск только по имени и
версии позволил бы показать не те bytes при конфликте или подмене.

**Решение.** `package inspect` принимает exact immutable ref, сверяет digest
с сохранённым манифестом и только затем показывает metadata, origin, lock,
trust и lifecycle. Requested capabilities выводятся как декларация пакета, не
как предоставленные ему права.

**Не выбрано.** Не использовать путь пакета, alias или id/version как замену
identity; не чинить найденные bytes в read-only команде и не выполнять
содержимое ради извлечения metadata.

## 2026-08-31 — Состояние доверия закрепляется вместе с допуском

**Контекст.** Одно только свежее чтение отзыва оставляло короткое окно: пакет
мог быть отозван после проверки, но до записи нового Run, Attempt или Check.
Это был бы новый допуск по уже устаревшему решению.

**Решение.** Обычный Run command получил дополнительную read-only authority
привязку. Хранилище сверяет её в той же transaction, что и изменение Run;
сменившееся package state отклоняет команду до reducer. Привязка не меняет
смысл самого запроса, поэтому повтор уже записанной команды возвращает прежнюю
квитанцию, а не становится конфликтом из-за позднего отзыва.

**Не выбрано.** Не превращать эту точечную commit-time защиту в глобальную
перезагрузку реестра и не расширять её до external action delivery. Отдельные
границы dispatch, receipt и recovery требуют квалифицированного executor
договора.

## 2026-08-31 — Отзыв пакета повторно проверяется перед ActionAdmission

**Контекст.** ActionIntent удерживает точный ToolDescriptor как историческое
предложение, но его подтверждение расходует человеческое approval и является
новым допуском. Отзыв пакета после proposal не должен позволить превратить
старую запись в новое разрешённое действие.

**Решение.** Admission читает точный ToolRef из сохранённого ActionIntent и
проверяет текущее package authority state. Его версия закрепляется в той же
SQLite transaction, что и расход approval и запись ActionAdmission. Только
прямой security revocation останавливает уже сохранённое предложение;
quarantine и removal остаются отдельными состояниями lifecycle.

**Не выбрано.** Не удалять historical ActionIntent и не запрещать proposal
задним числом: они не являются доставкой или внешним effect. Не расширять
изменение до dispatch/recovery — эти границы требуют отдельного executor
договора.

## 2026-08-31 — ActionIntent проверяет arguments по sealed ToolDescriptor

**Контекст.** Closed protocol ActionIntent гарантировал форму JSON и exact
ссылку на схему, но сам по себе не доказывал, что переданные arguments
соответствуют schema, названной ToolDescriptor. Такая запись могла стать
намерением с параметрами, которых этот инструмент не объявлял.

**Решение.** До durable proposal runtime берёт schema ref из найденного exact
ToolDescriptor и валидирует arguments через тот же sealed registry. Ошибка
возвращается до Run transaction; user-provided schema ref всё ещё сравнивается
с descriptor внутри существующей проверки контракта.

**Не выбрано.** Не принимать validation другой schema, указанной только в
ActionIntent, и не откладывать проверку до ActionAdmission: к тому моменту
неверное намерение уже оказалось бы в durable ledger.

## 2026-08-31 — ActionIntent удерживает только существующие input artifacts

**Контекст.** Input artifact refs становятся частью exact operation. Если
сохранить ссылку на отсутствующие bytes, будущая доставка либо прочитает не те
данные, либо уже после approval столкнётся с невозможной операцией.

**Решение.** Proposal перечитывает каждый named ArtifactRef до записи. Обычное
отсутствие bytes становится понятным отказом; нарушение целостности не
маскируется под отсутствие и остаётся ошибкой integrity.

**Не выбрано.** Не загружать новый input по имени, не пересоздавать artifact и
не переносить проверку к будущему executor: exact input должен быть известен
до человеческого решения.

## 2026-08-31 — Новый ActionIntent требует доступного сейчас пакета

**Контекст.** Открытый runtime хранит снимок sealed definitions. После отзыва
пакета такой снимок ещё может найти его ToolDescriptor, хотя создание нового
ActionIntent уже означает новое использование этого пакета.

**Решение.** Proposal проверяет current package authority с требованием
resolvable и закрепляет версию этого authority state в той же transaction, где
сохраняется ActionIntent. Поэтому отзыв, quarantine или removal не проскочат
между проверкой и записью. Ранний путь exact command receipt не меняется:
повтор уже записанной команды возвращает прежнюю квитанцию.

**Не выбрано.** Не удалять старые ActionIntent и не менять правило их
ActionAdmission: это разные стадии жизненного цикла. Не добавлять отдельный
кэш или глобальную перезагрузку definitions, поскольку точечная authority
привязка уже закрывает границу новой записи.

## 2026-08-31 — Неизменяемый кандидат пилота до формального RC

**Контекст.** Владелец запросил зафиксировать текущую сборку и проверить на
ней настоящую небольшую задачу с AI Factory. Однако единственный обязательный
hardware gate RC-0 — физический suspend/resume целевого Mac — ещё не выполнен.

**Решение.** Сохранить clean build в GitLab как неизменяемый AIF pilot
candidate, а не называть его квалифицированным release candidate. Реальная
задача и все найденные исправления идут поверх этой точки; RC обозначается
только после evidence RC-0 и единственного полного RC-4 gate.

**Не выбрано.** Не менять формальные статусы, не переписывать F1 evidence и
не присваивать RC label одной только успешной scripted или assisted задаче.

## 2026-08-31 — Предупреждение о доступности CDN оставлено заметкой пилота

**Контекст.** Реальная статическая страница использует Anime.js по публичному
CDN. Локальная проверка подтвердила разметку, подключение библиотеки и fallback
для reduced motion, но намеренно не обращалась в сеть.

**Решение.** В пилоте не добавлять сетевую проверку и не называть её
выполненной. Предупреждение сохраняется в результате AIF; работа продолжается
и фиксируется, потому что отсутствие такой проверки не блокирует создание
статической страницы.

**Не выбрано.** Не подменять библиотеку самодельной анимацией и не загружать
сторонний файл ради одной демонстрации. Проверку доступности CDN добавлять
только в задаче, где это является требованием результата.

## 2026-08-31 — Suspend evidence измеряет весь заявленный интервал

**Контекст.** Первая физическая попытка вернула `passed`, хотя разница между
calendar и excludes-suspend monotonic временем составила около 12 секунд при
заявленном минимуме 20 секунд. Это было расхождением проверяющей программы, а
не основанием уменьшить требование.

**Решение.** Проверка теперь требует наблюдённую разницу не меньше полного
заявленного минимума. Первая попытка не сохранена как evidence; повторная
проверка наблюдала 132822 ms и сохранена отдельным immutable JSON.

**Не выбрано.** Не считать слова оператора заменой измерения, не ослаблять
порог допуском «половины сна» и не переписывать прежнее F1 evidence, в котором
аппаратная проверка была честно помечена partial.

## 2026-08-31 — Выпускная проверка использует текущий формат session Run

**Контекст.** Полный RC-прогон показал шесть тестов publication: они ожидали
старые форматы state 13–17, хотя общий session Run после добавления durable
prepared delivery уже получает текущий state 21. Две тестовые записи также не
содержали обязательный format, который реальная публикация записывает всегда.

**Решение.** Обновить только ожидания и искусственные записи тестов до current
public contract. Проверяемые закрытие публикаций, cursor, new-only cut и
terminal failure не меняются; целевые шесть проверок проходят.

**Не выбрано.** Не откатывать state 21, не ослаблять проверку типа публикации
и не менять runtime: они правильно отражают текущий договор.

## 2026-08-31 — Deadline-тест ждёт завершение после срока процесса

**Контекст.** В полном RC-прогоне deadline-тест иногда ждал результат ровно
столько же, сколько разрешал самому process. Остановка process group и durable
settlement происходят уже после истечения этого срока, поэтому проверка могла
упасть на здоровой реализации при загруженной машине.

**Решение.** Для deadline-варианта оставить production deadline 10 секунд, но
дать тесту 20 секунд на полный shutdown и settlement. Cancel-вариант сохраняет
прежний предел 10 секунд.

**Не выбрано.** Не увеличивать срок работы checker и не игнорировать отсутствие
settlement: тест по-прежнему требует доказанный остановленный process и durable
итог.

## 2026-08-31 — Первый RC ограничен проверенным local AIF профилем

**Контекст.** Есть physical suspend/resume evidence, полный release gate,
примеры и новый clean-install scenario на одном exact candidate binary. Это
достаточно для обещанного owner-host сценария, но не для полного F2.

**Решение.** Зафиксировать первый RC как `core-local AIF`: человек выполняет
assisted steps, выбирает улучшения и предупреждения, а runtime сохраняет
состояние и границы управления. Полную roadmap продолжать отдельными срезами.

**Не выбрано.** Не объявлять готовыми remote execution, credentials, внешние
effects, managed isolation, scheduler или весь F2. Не загружать GitLab archive
без отдельного разрешения: архив содержит исходники и документацию.

## 2026-08-31 — Launcher выбирает явный Project launch

**Контекст.** Первый host skill был жёстко привязан к AI Factory task recipe.
Это превращало GitLab/chat TaskInput в ложное обязательное начало любого
сценария и не давало команде запускать свой versioned workflow с его входами.

**Решение.** Хранить в tracked `project.yaml` компактный `launches` catalog и
читать его через `project workflows`. Каждый launch имеет ID, описание, тип и
один source внутри `.prifly/`; host всегда просит точный ID. `task_recipe`
создаёт TaskInput, а `workflow` раскрывает declared inputs своего YAML/JSON
сценария. Каталог read-only и не предполагает, что selected source уже
установлен как package.

**Не выбрано.** Не искать все файлы workflows автоматически, не назначать
первый launch default и не создавать plugin framework или tracker adapter:
автоматическое discovery показало бы внутренний либо незакреплённый сценарий,
а источник задачи остаётся вариантом входа, а не свойством launcher.

## 2026-08-31 — YAML становится единственной правдой о project workflow

**Контекст.** AI Factory recipe уже выпускал читаемый YAML, но большая часть
его nested graph всё ещё была Python dictionaries. Разработчику приходилось
искать правду в двух файлах, а добавление lint/QA означало правку программы,
хотя это обычное изменение процесса проекта.

**Решение.** DCL-01 делает `steps.yaml`, workflow YAML и `extensions.yaml`
единственными authoring sources. Compiler только собирает skills, разрешает
exact refs, проверяет graph/ports и seal-ит новую revision. Первый простой
extension имеет строгую форму `between.from → between.to`; он не скрывает
изменение control flow и отказывает при неоднозначности.

**Не выбрано.** Не оставить Python вторым graph source, не искать stage по
похожему имени и не строить универсальный visual editor/plugin framework.
Repeat/parallel/map composition остаётся full YAML, потому что её безопасный
смысл нельзя вывести из короткой вставки.

## 2026-08-31 — Простая extension не угадывает входные данные

**Контекст.** Короткая запись `between` может безопасно изменить ровно один
маршрут, но не может корректно вывести binding для входов добавленного шага.
Угадывание сделало бы graph внешне декларативным, но не проверяемым.

**Решение.** Первый удобный путь ограничен no-input step: compiler проверяет
его contract и вставляет между двумя явно названными стадиями. Lint/QA, которым
нужна claimed working copy, объявляет поддерживаемый `workspace_write` effect.
Для входов, repeat/parallel/map и нескольких routes разработчик редактирует
full YAML graph.

**Не выбрано.** Не добавлять скрытый `input_bindings`, auto-wiring или новый
мини-язык extension. Это можно расширить только вместе с явным contract и
тестом для нового случая.

## 2026-08-31 — YAML охватывает и настройки AI Factory recipe

**Контекст.** После первого DCL-01 среза Python уже не содержал graph, но всё
ещё хранил схемы step data, тексты вопросов, лимиты, параллелизм и package
version. Разработчик всё равно должен был искать часть смысла в программе,
поэтому «единственный источник правды» был выполнен лишь частично.

**Решение.** В profile добавлены `schemas.yaml.tmpl`,
`questions.yaml.tmpl` и `settings.yaml`. Последний явно называет package
identity, параллелизм и каждый project limit вместе с его workflow input,
ceiling и default; Python читает ограниченную документированную форму без
новой внешней зависимости и не держит соответствия этих имён. Questions
остаются YAML authoring source, но при sealing становятся text/markdown
context: именно такой media type принимает core для инструкции исполнителю.
Package version повышена до `1.0.1`, поскольку новая sealed content не может
честно выдавать себя за прежнюю revision.

**Не выбрано.** Не добавлять PyYAML как скрытую зависимость launcher и не
расширять core новым generic parser/registry ради одного recipe. Не разрешать
сценарию автоматически заменить trusted package или его components под тем же
identity: история Run должна оставаться проверяемой.

## 2026-08-31 — Компактная папка YAML не создаёт второй язык сценариев

**Контекст.** Даже после переноса graph в YAML один сценарий требовал Python
recipe, package manifest и `.yaml.tmpl`. Разработчик видел несколько мест,
где могла находиться «правда», а структура большого workflow не помогала
читать nested components.

**Решение.** Ввести generic `prifly-project-workflow/1`: обязательный
`workflow.yaml` одновременно называет package и содержит root graph; optional
`extend.yaml` и recursive component directories остаются обычными YAML files.
Folder path не имеет исполняемой семантики. Compiler сам строит прежний
`prifly-package-source/1`, проверяет exact refs и seal-ит immutable package.
Raw context может объявить source только из `.prifly/` или versioned
`.codex/skills/`, поэтому byte skill закрепляется без Python copy.

**Не выбрано.** Не добавлять AI Factory terms, skills или graph templates в
core; не строить plugin framework, implicit discovery или automatic import/run.
Старый explicit package source сохраняется для совместимости, а сложные
изменения repeat/parallel по-прежнему редактируются явно в graph YAML.

## 2026-09-01 — Текущий авторский путь не смешивается с ранними Python-записями

**Контекст.** Журнал сохраняет промежуточные решения DCL-01 и DCL-02, где
упоминаются `aif-cycle.py`, `.yaml.tmpl`, `steps.yaml` и `extensions.yaml`.
После DCL-03/DCL-04 эти записи начали выглядеть как параллельная действующая
инструкция, хотя текущий AIF example и project compiler уже используют другой
путь.

**Решение.** Для нового project workflow единственный authoring source —
компактная YAML-папка: `workflow.yaml`, optional `extend.yaml` и отдельные
components в `steps/`, `schemas/`, `contexts/`, `workflows/`; шаги используют
`prifly-step/1`. Go binary выполняет compile/validation/sealing. Python может
быть пользовательским executable или test helper, но не содержит graph,
transitions, settings или package definition. Legacy explicit package source и
`project extend` сохраняются для совместимости, но не являются рекомендуемым
путём.

**Не выбрано.** Не переписывать исторические evidence и не удалять
совместимую legacy форму лишь ради чистоты документации. Актуальная инструкция
живёт в `docs/workflow-yaml.md`; журнал поясняет, почему старые записи не
следует использовать как новый API.
