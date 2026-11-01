## Context

Мотивация и границы поставки описаны в [proposal.md](proposal.md). Требования находятся в четырёх delta specs этого change.

Текущий `project_extension_call.go` разрешает call только среди документов выбранной folder. `readProjectWorkflowFolder` читает package и `project/` subtree. `extend.yaml` добавляет шаги или local calls, но не импортирует sibling package.

В smsplace classic/profiled уже содержат `tests`, `merge-request`, `heavy` и проектный gate. Их текущие точки вставки — `review → commit` и `commit → done`. UI/UX установлен отдельно. Его `workflows/quality.yaml` объединяет capture, среду, UI, UX, evaluate, cleanup и finalize.

Часть старого project README smsplace описывает прежние пути и порядок шагов. Основание этого design — actual YAML и definitions, а не тот README.

## Goals / Non-Goals

**Goals:**

- Дать проекту короткий читаемый рецепт вместо копии чужого графа.
- Объединять несколько производных workflows, выполняя их общий маршрут один раз.
- Явно объединять дополнения последовательно, параллельно или через развилку.
- Переиспользовать публичный workflow или step через typed contract.
- Сохранить одну реализацию UI/UX и обязательную оценку применимости в будущем пакете.

**Non-Goals:**

- Не добавлять второй runtime-язык или новое управление внешними actors.
- Не делать произвольный patch YAML или автоматический deep merge несвязанных графов.
- Не создавать отдельный hook DSL, dependency service, автоматический package updater или UI-эвристику в Core.
- Не переписывать examples из smsplace в рабочие файлы этого проекта.

## Decisions

### 1. Минимальная поверхность authoring

Используем явные действия: выбрать parents, перечислить layers и вызвать public components. `extends` принимает один parent либо список. `layers` задаёт порядок локальных настроек. `imports` связывает alias с объявленным package. `arrange` задаёт объединение пересекающихся дополнений родителей.

Один parent остаётся короткой записью для обычного случая. Несколько parents включают объединение по общей exact базе. Это целевой контракт текущего change, а не исключённая будущая возможность.

Новые markers: `prifly-project-workflow/2` для exports/derived folder и `prifly-extension/3` для imports/layers. Поля ниже — предлагаемый контракт, пока не действующий синтаксис CLI.

Самый короткий путь не требует производной folder. Автор может добавить import в `/3` extend существующего classic. Отдельный derived workflow нужен, когда проект хочет собственное имя и повторно используемый вариант маршрута.

Альтернатива — один универсальный `overrides` с произвольными paths. Она скрывает смысл изменений и делает обновление базы хрупким.

### 2. Экспорт — публичный вход в workflow, а не доступ к его internals

Provider добавляет `exports` в корень folder `/2`:

```yaml
exports:
  workflows:
    review-for-change: "{{workflow_review-for-change}}"
```

Это будущий пример экспорта UI/UX, не обещание существующего пакета. `workflows` и `steps` — единственные direct export kinds первого среза. Значение использует существующий local component alias и проходит kind validation.

Export включает reachable schemas, checks, contexts, steps, nested workflows и execution files. Consumer не перечисляет их отдельно. Автоматический re-export чужих imports в первом срезе не нужен.

Folder `/2` также может объявить `imports` той же формы для зависимостей собственных source exports. Они читаются при library import. Imports из provider extend не читаются: это конфигурация его standalone запуска. Так nested library calls разрешаются без наследования чужих проектных настроек.

Квалифицированный export в source `refs` разрешается до native validation. Например, `refs: {quality: ui.review-for-change}` даёт обычный `workflow_ref: quality` у Call. Kind проверяется в месте использования; export не становится строкой runtime lookup.

Импортированный workflow получает публичные inputs и declared project-scoped configuration. Его собственный standalone `extend.yaml`, launch preflight и локальные overrides не применяются. Поэтому один consumer не получает настройки другого запуска молча.

Step export используется через `step`, workflow export — через `call`. Замена provider program binding, policy или private stage запрещена. Нужную конфигурацию provider должен объявить публично.

### 3. Короткий consumer и явные общие слои

Предлагаемый `extend.yaml` производного smsplace-classic:

```yaml
authoring: prifly-extension/3
imports:
  ui: aif-ui-ux
layers:
  - ../../layers/delivery.yaml
extensions:
  - id: interface-review
    workflow: root
    between: {from: tests, to: commit}
    call: ui.review-for-change
    input_bindings:
      task: $inputs.task
      implementation: $stages.review.implementation
    on: {succeeded: commit, partial: abandoned, rejected: abandoned}
```

`ui` — локальное имя импорта. `aif-ui-ux` — имя установленного package из `project.yaml`. Версия и source pin берутся оттуда; пользователь не повторяет commit/digest в каждом вызове.

`delivery.yaml` содержит вынесенные проектом tests/MR/resource/settings. Общие tests/MR definitions можно держать в одном local provider и импортировать из слоя. Поэтому shared layer не требует копий worker files в classic и profiled.

`workflow: root` адресует effective root независимо от собственного ID производного workflow. Legacy selectors базы применяются в исходном scope до переименования derived root. В `/3` явный extension `id` именует stage независимо от qualified target, включая imported step.

В текущем smsplace база уже содержит tests/MR. Перед применением такого shared layer проект должен отдельно вынести прежние записи из обоих extend. Пример предполагает эту миграцию, а не повторное добавление tests/MR. Альтернатива без миграции — сохранить нынешние вставки и добавить только импорт UI/UX в existing classic extend.

Bindings этого примера описывают желаемый будущий export. Реальный input contract, критерии и browser configuration выпускает пакет после Core. Если ему нужны дополнительные inputs, consumer явно связывает их; Core ничего не генерирует по имени `review-for-change`.

В примере `partial/rejected → abandoned` — явная остановка, а не автоматическая починка. Сохраняются child evidence и исходный результат. Новый repair loop не входит в Core change.

Слой ищется относительно объявляющего документа. `..` допустим только внутри canonical repository root. Symlinks и выход из repository запрещены. Supporting file paths разрешаются относительно объявившего их layer; source provenance хранит фактический путь. Старый resolver сохраняет прежние folder boundaries.

### 4. Производная folder наследует общую основу нескольких родителей

Пример `workflow.yaml` нового пользовательского варианта:

```yaml
authoring: prifly-project-workflow/2
package:
  id: smsplace:package/delivery
  version: 1.0.0
  description: Classic с общими проверками smsplace
  requires_core_protocol: "1"
extends: aif-classic
id: smsplace:workflow/delivery
version: 1.0.0
title: Разработка smsplace
```

`extends` называет зарегистрированные package folders, а не remote URL и не installed Registry editions. Compiler читает их source ancestry и declared additions. Выбор parents явно наследует эту конфигурацию; обычный import этого не делает.

Derived root получает собственную identity. Ports, decision catalog, refs, features, execution bindings и ресурсы общей базы переносятся с source provenance. Собственные limits можно объявить явно. Они не увеличиваются автоматически для child или дополнительных родителей.

Derived root не переписывает произвольно `stages`, ports и inherited policies. Он добавляет contributions через extensions и явно объединяет их через `arrange`. Contribution — исходное добавление с точкой подключения и входами/исходами. Оно может быть step/Call или вызовом child graph с обычной развилкой.

Пример наследования без повторения classic:

```text
                 aif-classic
                      ↓
           smsplace-base: tests и MR
                 /           \
        classic-ui         classic-audit
        + UI/UX            + аудит
                 \           /
                 smsplace-delivery
```

В `workflow.yaml` последнего варианта достаточно заменить scalar:

```yaml
extends: [classic-ui, classic-audit]
```

Resolver строит ancestry DAG — граф родителей без циклов. Он находит единственного ближайшего общего ancestor с одинаковой exact source identity. Общая основа и её contributions включаются один раз. Поэтому tests, commit и MR не удваиваются из-за двух путей наследования.

Один source contribution определяется его origin identity и local extension ID. Повторные пути объединяются только при одинаковом effective contract. Разные bindings или настройки одного inherited contribution требуют явного разрешения конфликта. Два независимых добавления с одинаковым именем не становятся одним contribution.

Разные редакции общего ancestor, несвязанные основы или несколько несовместимых ближайших основ дают отказ. Группа, уже объявленная parent, наследуется как структурированный contribution. Consumer не разбирает её внутренние обязательные проверки молча.

Альтернатива — запускать оба полных родительских workflow через Call. Она повторяет classic и не решает запрос пользователя. Автоматический выбор «первого родителя» также отвергнут: он теряет настройки или дополнения.

### 4.1. Дополнения объединяются явно в своей точке маршрута

Непересекающиеся contributions сохраняют места в общем графе. Если оба родителя изменяют один переход, compiler требует `arrange`. В наследуемом recipe сохраняется исходный anchor, поэтому второй parent не теряет место после применения первого.

Предлагаемый пример в `/3` extend конечного consumer:

```yaml
arrange:
  - id: extra-checks
    between: {from: tests, to: commit}
    parallel:
      - classic-ui.interface-review
      - classic-audit.test-audit
    on: {succeeded: commit, rejected: abandoned}
```

Selector называет package родителя и его contribution ID. Для последовательного выполнения автор использует `sequence` с теми же selectors. Условия задаются `choice` с native predicate AST, selection, default и on_unknown. Группы допускают ограниченное вложение, например mandatory UI/UX рядом с выбором дополнительного аудита.

`sequence` требует успеха всех элементов в указанном порядке. `parallel` требует успеха всех элементов и завершения всех ветвей. Его lowering использует native join `mode: all`, `selection: all`, `remainder: wait`. Это безопасный простой default первого среза; quorum/race здесь не добавляются.

Группа имеет обычные control outcomes `succeeded/rejected`. Успех означает выполнение объявленного условия всей группы. Остальные child outcomes, включая partial, ведут в rejected с сохранёнными исходными child evidence; technical error использует ordinary error route. Нужна другая продуктовая агрегация — автор объявляет typed child workflow и подключает его как contribution.

Compiler разворачивает явно заданную группу в ordinary workflows/stages, не создавая новые schemas или предметные checks. Для step leaf он сохраняет соответствие verdict/outcome в control wrapper. Branch labels и source mapping определяются origin contribution и group position, без случайных IDs.

Каждый contribution точки должен присутствовать в группе ровно один раз. Choice структурно объявляет все альтернативы, хотя исполняет выбранную. Обязательные дополнения размещаются в sequence/parallel вне альтернативной ветви. В основном smsplace-примере applicability UI/UX никогда не оказывается пропущенной альтернативой.

Outputs и bindings проверяются после lowering. Compiler не переносит молча root reference внутрь sibling branch. Если группировка меняет scope читаемого output, требуется явно объявленный typed wrapper с подходящими ports либо named refusal. Это не разрешение автоматически изобретать output schema.

Параллельность разрешается только при допустимых claims, effects и resource limits. Join не освобождает путь после одного PASS и не скрывает живую либо неизвестную ветвь. Недостаточная capacity не превращается в молчаливое sequential исполнение.

```text
Общая база с её extend — один раз
    ↓
Уникальные additions родителей + явный arrange
    ↓
Упорядоченные layers consumer
    ↓
Собственный extend производной folder
    ↓
Разрешённые параметры запуска
    ↓
Проверенный обычный граф → sealed package → Run
```

Граф `extends` и imports может ветвиться, но не содержит cycles. Начальные фиксированные пределы: глубина 8 parents/groups, 32 уникальных layer files и 32 source folders во всей композиции. Общие ancestors учитываются однократно. Existing file/byte и graph limits действуют дополнительно; новые настройки потолков не добавляются.

### 5. Порядок и конфликты понятны без правил общего YAML merge

Layers перечисляются в одном месте и не включают другие layers. Поэтому список чтения виден без рекурсивного обхода preset-файлов. Общие ancestors разворачиваются однократно до уникальных parent deltas. Profile substitutions выполняются после выбора итоговых settings, до native compilation; промежуточные варианты не seal-ятся.

Разные значения одного setting в независимых parents — конфликт до явного решения consumer. Список `extends` не является скрытым last-wins порядком. Совпадающие effective значения допустимы с сохранением обоих origins. Внутри одной ancestry ветви остаётся обычный порядок слоёв.

Правила объединения ограничены:

| Поле | Правило |
|---|---|
| `profile` | Последний явно объявленный профиль, проверенный схемой базы |
| `settings`, `answers` | Замена значения по конкретному ключу; структурные maps собираются по ключам |
| `exclude` | Полная замена списка; граф использует авторский bypass |
| `model_profiles` | Замена целой записи конкретного host/profile, без слияния opaque значений |
| `resources` | Последняя полная запись по имени; limits остаются под действующей policy |
| `references` | Уникальное имя; конфликт с другой целью запрещён |
| `imports` | Уникальный alias; повтор с другой целью запрещён |
| `execution_bindings` | Existing ownership rule; provider-bound программу переопределять нельзя |
| `extensions` | Последовательное применение; повтор stage ID запрещён |
| `arrange` | Явное объединение parent contributions в указанной точке; пересечение без группы запрещено |

Значение configuration input, даже если оно object, заменяется целиком. Списки не конкатенируются автоматически. Неизвестные keys отказываются. Настройки импортированного workflow используют qualified ключ, например `settings.ui.review-for-change`; kind и configuration scope проверяются по export contract. В YAML `ui.review-for-change` является одним ключом map `settings`.

Layer допускает блоки настроек и extensions с marker `/3`, но не `extends`, `arrange` или вложенный `layers`. Parents и их объединение выбирает конечный workflow. Inserted components объявляются existing project-owned declarations или imports. Новый универсальный язык inline steps не вводится.

Для first slice сохраняем `between`, а не вводим ещё один механизм точек расширения. Изменившийся upstream route вызывает понятный отказ. Стабильные public hooks можно обсуждать отдельно при доказанной необходимости.

### 6. Composition builder работает до native compilation

Добавить bounded read-only resolver рядом с существующим Project compiler:

1. Прочитать project, ancestry DAG и explicit layers; найти общую exact основу.
2. Разрешить aliases и source contributions; проверить origin, paths, kinds и cycles.
3. Учитывать ancestors однократно; разрешить parent conflicts явными настройками consumer.
4. Применить arrange и extensions; проверить native bindings, outcomes и joins.
5. Закрепить reachable dependency closure и сформировать build provenance.
6. Выполнить existing compile, seal и reviewed-start проверки.

```text
project + source folders
    → проверка путей и зависимостей
    → общий ancestor и уникальные contributions
    → arrange и ordered layers
    → native sequence/parallel/choice и typed validation
    → closure и provenance
    → обычная compilation/sealing
```

Каждая source folder сохраняет свой namespace разрешения local aliases. Одинаковые short names разных providers не конфликтуют до квалифицированного выбора. Одинаковая exact identity с разными bytes отказывает, а не переименовывается ради сборки.

Schema refs переносятся как часть exact dependency. Consumer не должен копировать provider schema для удовлетворения typed input. Сквозной fixture обязательно связывает такие ports без локальной копии YAML.

Provider source с локальным drift относительно declared origin отказывает обычным правилом установки. Local package без origin допустим как явный project source. Полный source inventory может консервативно инвалидировать review; нет обещания incremental compilation первого среза.

Sealing может копировать bytes в immutable хранилище. Это штатная фиксация Run, а не вторая редактируемая реализация. Effective build identity учитывает все реально использованные источники и mappings. Source update не переписывает pins предыдущего Run.

### 7. Применимость UI/UX принадлежит будущему package workflow

Core reference использует небольшой synthetic provider. В нём workflow всегда вызывает applicability step, затем обычный Choice. Это доказывает возможность композиции без реализации UI/UX-решений в Core.

```text
Classic: реализация → verify/review с исправлениями → tests
                               ↓
               imported review-for-change
                               ↓
                 applicability для задачи и candidate
                    ├─ required → UI/UX → оценка
                    ├─ not-required → причина skip
                    └─ unknown → diagnostic partial
                               ↓ только succeeded
                       commit → Draft MR
```

Будущий package contract должен связывать применимость с исходной задачей, фактическими изменениями и актуальным candidate. Путь файла — сигнал, а не доказательство отсутствия UI-изменений. `app/MoonShine/**` в smsplace показывает, почему проверять только JS/CSS недостаточно.

На неинтерфейсной задаче applicability выполняется, browser reader не создаётся. При unknown или недоступном обязательном браузере успешная приёмка не разрешается. Caller не подменяет это входным `ui_enabled: false`.

После repair новый проход повторяет applicability и нужные readers. Сам импорт не добавляет loop: wrapper или родитель заранее объявляет его native Repeat. В минимальном smsplace-примере findings останавливают запуск для решения владельца.

UI/UX environment имеет отдельный declared lifecycle. Existing PHP `tests` освобождает свой environment, поэтому совместное использование browser/PHP lease требует отдельного проектного контракта. Core не обещает автоматическое объединение окружений. Изменивший candidate поздний шаг инвалидирует прежнюю приёмку по ordinary freshness правилам.

### 8. Обзор сначала показывает маршрут, затем подробности

Для новых форм default compile/prepare показывает parents, общую базу, layers, imports и contributions. Он показывает общий маршрут один раз и подписывает sequence/parallel/choice группы. Поля ещё не вычисленного applicability показываются как «проверяется при исполнении», не `skipped`.

`project compile --explain` даёт подробное read-only объяснение без выполнения. Это дополнительный режим существующей команды, не новый набор compose/build/plan команд. JSON добавляет versioned composition provenance; exact DTO versions выбираются до реализации writers.

Пример короткого текста для будущего consumer:

```text
База: aif-classic
Слой: .prifly/layers/delivery.yaml
Импорт: ui → aif-ui-ux / review-for-change
После tests: interface-review
Условие UI/UX: определяет обязательный child при исполнении
Маршруты: succeeded → commit; partial/rejected → abandoned
```

Ошибки содержат code, consumer source/key и provider/layer context. Печать полного графа и всех settings остаётся подробным explain, а не default шумом.

### 9. Совместимость, безопасность и продолжение

Legacy `/1` folder, unmarked extend и `/2` extension сохраняют прежние parser/build paths и fixed vectors. `/3` разрешает imported targets и native root-input bindings; `/2` не получает это расширение молча.

Prepare включает effective base, layer order/content, выбранную closure, host contexts и execution bindings в review. Он не выполняет programs, не создаёт claim и не предоставляет trust. Start проверяет digest повторно до effects. Rights, requested capabilities и external-write boundary всей closure видны через existing admission.

Если sealed packaging требует explicit dependency manifests, builder формирует проверяемую замкнутую поставку через existing PackageComponent machinery. Он не создаёт TrustDecision за пользователя. Импорт сторонней policy не повышает ceiling выбранного запуска.

Обновление одного provider остаётся отдельным действием. Local layers за его пределами и consumer extend сохраняются. Совместимое обновление не требует ручной синхронизации копий. Breaking export/port/route update требует явной правки consumer и нового prepare.

Продолжение использует existing effective-prefix comparison. Изменившийся imported child внутри accepted prefix не переносится как прежний PASS. Whole-child replay допускается только действующим recovery contract. Cancellation не доказывает остановку внешнего actor или cleanup.

## Risks / Trade-offs

- [Настройки снова становятся языком программирования] → общая exact база, ограниченные группы, плоские layers и native stages.
- [Ромб наследования повторяет classic] → source provenance и однократное включение общего ancestor; отдельный runtime fixture.
- [Один parent скрывает другой] → явные groups/settings при конфликтах, без last-wins порядка parents.
- [Parallel group допускает публикацию раньше времени] → all/wait native join, проверка обеих ветвей и общий effect/resource control.
- [Наследуются неожиданные project overrides] → `extends` читает явно выбранную effective базу; `imports` читает source export без provider extend.
- [Универсальность требует копий schemas или worker files] → reachable closure и exact port refs; отдельный negative/positive fixture.
- [Обновление ломает consumer] → exact origin, явные public exports, typed validation и понятный compile refusal.
- [Imported Call превышает budgets] → existing whole-closure checks; limits повышает только автор явно в допустимых пределах.
- [UI/UX пропускается по предположению] → mandatory applicability, candidate correlation и unknown route без успешной приёмки.
- [Synthetic tests объявлены browser PASS] → Core, package release и smsplace pilot имеют отдельные evidence и статусы.
- [Расширенная source область читает лишние файлы] → explicit declarations, repository confinement, no symlinks, bounded source counts и source list в explain.

## Migration Plan

1. Реализовать imports и один parent, затем multiple parents и composition groups по tasks этого change.
2. Выпустить Core и проверить generic public CLI scenario на released binary.
3. Отдельно поручить workflows repository публичный export и mandatory applicability contract.
4. Выпустить package после собственных checks и actual host/browser qualification.
5. Отдельно подключить consumer в smsplace и пройти три исхода applicability.

Только пункты 1–2 относятся к реализации этого Core change. Пункты 3–5 — handoff, а не разрешение менять соседние repositories.

Этап с одним parent не закрывает целевую composition capability. До готовности следующих этапов multiple-parent/group syntax явно отказывается. Если промежуточный subset выпускается отдельно, он получает честный capability profile и совместимый путь последующего расширения.

Core implementation commit не ждёт ещё не опубликованного binary. Release и published replay остаются отдельными незакрытыми gates до фактического выпуска.

Rollback authoring возвращает прежний launch/extend и не удаляет Run history. Rollback binary допустим только при поддержке уже записанных native contracts. Если новый writer потребует persisted composition metadata, сначала уточняются version boundary и план migration; скрытая запись в прежнюю форму запрещена.

## Интеграция после v0.13.83

Выпущенный `/4` не является неизвестным marker. Он сохраняет отдельный point contract. Этот change не добавляет cross-package point selection. Composition reader отказывает на sidecar любого участвующего package до flattening, включая невыбранный manifest. Effective `/4` и point entries в layers также отказываются. Library import без sidecar игнорирует standalone `/4`, как требует source-export boundary. Typed code: `project_composition_extension_points_unsupported`.

Такой ограниченный срез не переносит отрицательную проверку ownership на объединённую inventory. Старый points compiler продолжает работать только с исходной folder inventory. Будущая поддержка потребует положительного origin mapping и нового совместимого контракта.
