## MODIFIED Requirements

### Requirement: YAML authoring имеет локальный editor contract
Repository MUST публиковать versioned local JSON Schema documents и manifest
для поддержанных Project profiles `/2` и `/3`, workflow folder root,
execution bindings, extension list, workflow, step и context YAML. Contract
MUST называть kind, version marker, known fields и portable local schema
association. Он MUST работать без сети, AI Factory, credentials или обязательного
editor dependency, не принимать новые поля как старый contract.

#### Scenario: Автор подключает local schema
- **WHEN** автор открывает YAML source или reference в совместимом editor
- **THEN** published local schema даёт completion и diagnostics до compiler

### Requirement: Project authoring имеет один YAML route
Project execution profile SHALL принимать текущий нейтральный
`prifly-project-profile/3` и опубликованный `/2`. Каждый declared package MUST
ссылаться только на directory `.prifly/workflows/NAME/` с root `workflow.yaml`,
а Project launch MUST быть `workflow`, ссылающимся на такой root. Profile v1,
его отдельные source roots, `task_recipe`, direct machine workflow и file source
`prifly-package-source/1` MUST быть отклонены до sealing с понятной diagnostic.
Для этих дорелизных forms не создаётся migration obligation. Поддержка `/2`
MUST сохранять его опубликованный смысл; переход на `/3` MUST быть явной
правкой shared profile, а не побочным эффектом start или workflow add/update.

#### Scenario: Старый authoring source подан compiler
- **WHEN** profile содержит v1, `task_recipe`, direct machine workflow или
  file package source
- **THEN** Pri-Fly отказывает до output package, authority mutation или Run

#### Scenario: YAML folder подан compiler
- **WHEN** поддержанный profile называет допустимую workflow folder и launch
- **THEN** listing объявляет inputs, а compile выпускает sealed package

#### Scenario: Опубликованный Project profile используется после обновления
- **WHEN** пользователь открывает существующий `/2` profile новым binary
- **THEN** profile читается без неявного переписывания tracked YAML или Runs

### Requirement: Project source компилируется из declared files
Tracked `.prifly/` source MUST использовать поддержанный versioned Project
profile и declared workflow folders. Root `workflow.yaml` MUST объявлять
авторскую package identity, external refs, graph и known component directories;
compiler рекурсивно читает только YAML из этих declared locations.
Placeholder MUST заменять только whole YAML scalar exact ref или explicit
value; environment, shell, tags, anchors и prose interpolation запрещены.
`project compile` MUST создать sealed package без import, authority mutation
или Run. В `/3` разрешение paths и compilation сами по себе MUST NOT требовать Git
или host, если выбранный source от них не зависит.

#### Scenario: Placeholder не найден
- **WHEN** YAML ссылается на undeclared component
- **THEN** compile отказывает без угадывания ref или изменения authority

#### Scenario: Компиляция обычного сценария
- **WHEN** profile `/3` находится в папке без Git и не читает host skills
- **THEN** compile работает без host, не создаёт repository и не читает AI roots

### Requirement: Project context resolves the selected host skills root
Profile `/2` SHALL сохранять объявление стандартных roots для `codex-cli`,
`codex-app` и `claude-code`. Profile `/3` SHALL позволять отсутствие hosts или
подмножество поддержанных hosts. Context source с
`{root: host_skills, path: PATH}` MUST требовать явно выбранный declared host;
компиляция `/3` без такого source MUST NOT требовать host; `/2` сохраняет
explicit host. Host MUST NOT выводиться
из наличия папки. Compiler MUST отвергать неизвестный host, absolute path,
traversal, symlink escape, отсутствующий file или source вне `.prifly` и
declared skills root до sealing. Exact bytes закрепляются; host identity
MUST NOT сама менять YAML graph или давать полномочия.

#### Scenario: Claude Code skill закрепляется
- **WHEN** compiler получает `claude-code`, а context называет
  `aif-plan/SKILL.md` относительно выбранного skills root
- **THEN** он закрепляет exact bytes этого файла и не читает Codex root

#### Scenario: Context выходит за свой root
- **WHEN** source использует `..` или выходит за `.prifly` и selected skills root
- **THEN** compiler отказывает до output, authority mutation или Run

#### Scenario: Два host используют один Project workflow
- **WHEN** Codex и Claude компилируют одну folder в `/3` с разными context bytes
- **THEN** авторские package identity и YAML graph сохраняются, а exact
  сборки различаются и могут сосуществовать в одной authority

### Requirement: Project launch является единственной исполнимой точкой входа
Public Project launch MUST принимать exact ID объявленного workflow launch и
typed значения его inputs, compile/seal/register exact package до Run и
закреплять выбранные source/context bytes и inputs. Launch MUST NOT выбирать
сценарий по тексту задачи, default launch или наличию файлов.
Для `/3` host, Git Workspace и RunBrief MUST требоваться только по объявленному
контракту выбранного сценария. Обычный command launch MUST обходиться без них.
Interactive host MUST получить explicit `worktree` или `checkout` до запуска
работы, требующей Git Workspace; отсутствие ответа означает ожидание.
Без такой работы вопрос и claim MUST отсутствовать. CLI `/3` MUST требовать
явный workspace mode для Git-записи; default `/2` сохраняется для совместимости.
Запуск MUST NOT неявно запускать model/provider или расширять права host.

#### Scenario: Объявленный launch запускается
- **WHEN** пользователь назвал launch, required inputs и нужные ему ресурсы
- **THEN** Run использует только sealed revision этого launch и сообщает
  выбранный Workspace, если он требуется

#### Scenario: Launch не объявлен
- **WHEN** пользователь называет отсутствующий или не-workflow launch
- **THEN** система отказывает до compilation, registration, claim или Run

#### Scenario: Host не получил выбор Workspace
- **WHEN** launch требует Git-запись, а worktree/checkout не выбран
- **THEN** host спрашивает и не создаёт package, claim или Run до ответа

#### Scenario: Обычная папка содержит command workflow
- **WHEN** launch `/3` не требует Git или assisted execution
- **THEN** он исполняется без host, Git claim и фиктивного RunBrief

### Requirement: Project init prepares a context-capable authority
`project init` MUST создавать отдельную authority с current Core context
configuration для selected skills и других context resources, в том числе
для `/3` без host. Launch MUST отвергать incompatible authority до package,
claim или Run и не переинтерпретировать прежние Runs. Если copied/cloned
Project уже имеет valid profile и exact runners объявленных hosts, init MUST
создавать только отсутствующую local authority configuration без перезаписи
shared YAML/runners. Отсутствие hosts в `/3` MUST быть допустимым состоянием.

#### Scenario: Старый Core authority выбран для Project launch
- **WHEN** authority не имеет required current context configuration
- **THEN** CLI возвращает incompatibility diagnostic без package, claim или Run

#### Scenario: Clone получает свою authority
- **WHEN** valid profile и declared runners есть, а local configuration отсутствует
- **THEN** init создаёт только local configuration

#### Scenario: Project без host позже получает mixed workflow
- **WHEN** authority создана нейтральным init без hosts
- **THEN** она уже поддерживает current context pinning без пересоздания history

### Requirement: Update сохраняет exact identity и правки команды
`project workflows update NAME` MUST требовать origin, пересчитать digest
folder без `extend.yaml` и при расхождении отказать с изменёнными paths без
перезаписи. Неизменный remote commit при совпадающем digest MUST давать read-only
успех. Иначе команда MUST получить новую folder по тому же path, проверить её
как при add, перенести локальный `extend.yaml` byte-for-byte, атомарно заменить
folder и origin. Результат MUST сообщать изменение upstream `extend.yaml` и
сохранение авторской `package.version`. Sealed packages, locks, Runs и evidence
MUST NOT меняться. В `/3` новые source bytes при прежней авторской версии MUST
давать новую exact сборку. `/2` сохраняет legacy identity conflict и явное
сообщение о необходимости миграции для вариантов. Подмена bytes уже sealed
identity MUST оставаться отказом в обоих путях.

#### Scenario: Папка изменена локально
- **WHEN** digest folder без `extend.yaml` отличается от origin
- **THEN** update перечисляет изменённые paths и ничего не переписывает

#### Scenario: Удалённый commit не изменился
- **WHEN** ref указывает на тот же commit, а digest совпадает
- **THEN** update сообщает актуальность и ничего не записывает

#### Scenario: Upstream не поднял версию package
- **WHEN** новая folder отличается bytes, но авторская `package.version` та же
- **THEN** update явно сообщает этот факт; `/3` создаст другую exact сборку,
  а `/2` предупреждает о конфликте при существующей sealed identity

## ADDED Requirements

### Requirement: Сборки одного авторского package сосуществуют
Compiler `/3` SHALL детерминированно различать авторскую identity и identity exact
сборки для package и всего принадлежащего ему closure. Profile, effective
settings/exclude/extensions, explicit values, source/context/supporting-file
bytes, manifest metadata, decision catalog и external exact refs MUST участвовать
в определении сборки. Все поля generated provenance MUST быть детерминированы
этим входом. Внутренние refs MUST разрешаться
в соответствующие compiled components, внешние refs MUST не переписываться.
Повтор одинакового входа MUST давать одинаковые refs независимо от времени,
абсолютного пути проекта и порядка установки. Результат MUST сохранять
однозначное соответствие author root → compiled root и происхождение сборки
в schema-validated inert file, объявленном manifest. Mapping MUST совпадать с
фактическими exports до выбора root. `/2` сохраняет legacy compilation;
external consumers новой сборки MUST использовать compiled exact ref.
Доверие MUST относиться к exact manifest, не наследоваться от соседнего
варианта; checks collision/revocation и pinned история MUST сохраняться.

#### Scenario: Разные варианты запускаются в одной authority
- **WHEN** пользователь последовательно компилирует, импортирует и запускает
  варианты A, B, A одного package в одном project и authority
- **THEN** A и B сосуществуют, повтор A переиспользует exact сборку, а старый
  Run после restart сохраняет прежние definitions и context bytes

#### Scenario: Настройка меняет сборку
- **WHEN** команда меняет только extend setting, exclude или вставку шага
- **THEN** новая сборка устанавливается рядом без ручного переименования package

#### Scenario: Bytes sealed identity подменены
- **WHEN** тот же sealed id/version подан с другими bytes
- **THEN** import отказывает; повторная compilation revoked сборки не снимает отзыв

#### Scenario: Изменился только вопрос или описание
- **WHEN** author меняет только decision catalog либо manifest description
- **THEN** новая `/3` сборка получает другую identity и импортируется рядом

### Requirement: Project объявляет локальные команды без нового исполнителя
Project YAML SHALL описывать переносимую привязку объявленных steps/checks к
локальным программам: логическое имя executable, argv и declared supporting
files. Пути установленной программы и machine-specific environment MUST
оставаться локальными; секреты MUST NOT включаться в tracked YAML или отчёт.
Владелец MUST явно допустить binding до первого исполнения. Launch MUST
проверять полный набор нужных bindings до dispatch и закреплять effective
configuration для Run, не перезаписывая настройки других packages/Runs.
Само add/compile MUST NOT исполнять команды или выдавать им полномочия.
Работа без Git MUST использовать существующую Attempt workspace и typed
artifacts; произвольная запись в shared directory этим не разрешается.

#### Scenario: Коллега запускает общий command workflow
- **WHEN** YAML получен вместе с проектом, а executable установлен в другом месте
- **THEN** достаточно локальной привязки executable; graph и shared argv не меняются

#### Scenario: Binding отсутствует или пытается заменить чужой
- **WHEN** нужная программа не разрешена либо binding выходит за выбранный closure
- **THEN** launch отказывает до исполнения и не изменяет чужую конфигурацию
