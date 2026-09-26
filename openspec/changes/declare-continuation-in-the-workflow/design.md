## Context

См. proposal.md. Знание пакета AI Factory в коде движка (проверено 2026-09-26):

| Место | Что зашито |
|---|---|
| `internal/runtime/continuation.go:25-66` | исходный workflow `aif:workflow/classic`, `aif-profiled:workflow/classic`; исходы `partial`/`rejected`; стадии `warmup`, `implement` с вердиктом `pass`; порты `task`, `handoff`, `plan`, `implementation` |
| `internal/runtime/start.go:724-747`, `:881-890` | целевой workflow `aif-continuation:…/classic-continuation`; перенос `task`/`handoff`/`plan` по именам |
| `cmd/prifly/project_start.go:346` | launch допустим только с тем же ID |
| `cmd/prifly/project_continuation.go` | чтение `base_commit`/`head_commit` из содержимого `implementation`, предки commit, `changed_files` из `git diff`, вход `implementation` |
| `internal/runtime/recovery.go:150-162`, `cmd/prifly/project_recovery.go:30-45` | стадия `review`, выход `implementation`, поле `head_commit`, равенство commit claim |
| `cmd/prifly/main.go:2427` | справка «classic Run» |

Потеря работы: `releaseSettledClaim` (`internal/runtime/claim_run.go:208`)
при каждом новом claim того же репозитория освобождает claims всех
завершённых Runs; для `worktree` это `git worktree remove` и удаление ветки.

Нейтральные части: `run fork` и `ForkProvenance`, перенос префикса в
восстановлении по эффективному контракту, входам и решениям, проверка
активного продолжения по `fork.reason`.

## Goals / Non-Goals

**Goals:**

- Продолжение и восстановление работают для любого workflow, который их
  объявил; в `internal/` и `cmd/` не остаётся имён пакета.
- Workflow говорит, что сохранить (checkpoint) и что проверить (свой первый
  шаг); движок хранит, передаёт и исполняет.
- Незакоммиченная работа завершённого не-успешно Run не удаляется
  автоматически и переходит к продолжению вместе с деревом.
- Сохранённые Runs продолжения и восстановления читаются и доводятся.

**Non-Goals:**

- Движок не снимает состояние дерева сам (git, snapshot) и не читает
  checkpoint.
- Работа, которую упавший шаг не успел ни сообщить, ни оставить в дереве, не
  восстанавливается.
- Не менять смысл `run fork`, `run reopen`, `run resume`; не вводить
  продолжение с середины графа.
- Не переносить capability `aif-classic-workflow` из `openspec/specs/`.

## Decisions

### Форма авторинга, ревизия 7

```yaml
checkpoint: {schema_ref: schema_checkpoint}
stages:
  implement:
    kind: step
    checkpoint: state            # выход шага, сообщающий checkpoint
continuation:
  from_workflows: [aif:workflow/classic, aif-profiled:workflow/classic]
  from_outcomes: [partial, rejected]
  inputs:
    task: {source_input: task}
    handoff: {stage: warmup, output: handoff, verdict: pass}
    plan: {stage: implement, output: plan, verdict: pass}
    resume_from: {checkpoint: true}
```

Для стадии `call` вместо `verdict` пишется `outcome`. Отменённый Run исхода
не имеет, поэтому его продолжение — отдельное объявление `from_cancelled:
true`, а не шестой исход в `from_outcomes`; нужно хотя бы одно из двух.
Отменённый Run, держащий неразрешённую execution, не продолжается
(`continuation_source_unsettled`): сначала `run resolve`. Всё запечатано в
WorkflowRevision и входит в его digest, поэтому review digest prepare и
повторная проверка при start покрывают объявления без нового механизма.
Опубликованные схемы заморожены: поля получает только новая
`workflow-revision-v7.schema.json`; лестница авторинга поднимает ревизию по
наличию полей, как для `blocked` в ревизии 6.

Продолжение объявляет целевой workflow: он знает, какие входы ему нужны, а
исходный не должен знать о продолжателях. Альтернатива — объявление в
манифесте пакета или launch — отвергнута: оно не входило бы в digest, и одна
ревизия workflow продолжала бы по-разному в разных проектах.

### Checkpoint выводится из принятых результатов, не хранится отдельно

Последний принятый checkpoint Run — это выход `checkpoint`-порта последнего
по settlement принятого результата стадии, объявившей checkpoint, во всех
вызовах Run. Всё это уже есть в состоянии: принятые результаты, их выходы,
порядок settlement и запечатанный план. Поэтому отдельное поле состояния и
новая редакция состояния не нужны; представление Run вычисляет его при
чтении. Единая схема для Run обеспечивается проверкой при компиляции
вызовов.

Checkpoint упавшего без результата шага не существует — действует
предыдущий принятый. Отвергнутый результат (кандидат не прошёл приёмку) не
сообщает checkpoint.

### Передача claim вместо снимка

Claim — ресурс Core; его каталог физически хранит всё, что шаг оставил, в
том числе незакоммиченное. Передать его связанному Run дешевле и точнее
любого снимка и не требует git от движка. Передача происходит в транзакции
создания Run, по тому же пину claims, что и сегодняшняя привязка claim при
start; generation увеличивается. Нового поля в записи claims нет: claim
привязан к новому Run, а его fork provenance называет исходный, поэтому
редакция `authority-claims/3` не меняется. Условия: исходный Run завершён по `claimRunFinished`, claim привязан к
нему, inode каталога совпадает.

`--workspace-commit` (бывший `--implementation-head`) оставляет исходный
claim на месте и создаёт новый от указанного commit: это нужно, когда
работа доделана вне дерева Run. Связь commit с checkpoint проверяет первый
шаг workflow продолжения.

### Автоосвобождение по исходу

`releaseSettledClaim` снимает claim `worktree` только при исходе
`succeeded`, `completed_with_waivers` или `no_work`. Это исходы Core, а не
пакета. Остальные деревья живут до передачи или явного `claim release`.
Несколько worktree одного репозитория допустимы одновременно, поэтому новые
запуски не блокируются. Для `checkout` поведение прежнее: освобождение не
удаляет файлы, а оставленный checkout-claim блокировал бы весь репозиторий.

### Восстановление без предмета проверки

Восстановление получает claim исходного Run передачей и исполняет
отказавшую стадию в том же дереве, как `run reopen`. Условие «claim на
commit, который видел `review`» заменено физическим тождеством дерева.
Сохранённое `recovery/1` требует `SubjectCommit`; новые восстановления пишут
`recovery/2` без него. Читатели принимают обе редакции.

### Проверки — шагом workflow

Движок не знает, что `implementation` — Git-диапазон. Workflow продолжения
AI Factory получает checkpoint и прежний `implementation` и первой стадией
ставит программный шаг, который проверяет дерево по checkpoint, считает
`changed_files` и выдаёт новый `implementation`. Отказ этого шага записан в
истории Run как результат шага, а не скрытая логика CLI. Проверка чистого
checkout в режиме `checkout` остаётся в CLI: она про claim проекта.

### Guard против возврата имён

Тест сканирует не-тестовые `.go` файлы `internal/` и `cmd/` и отказывает на
строковом литерале `<namespace>:(workflow|package|step)/…` с namespace,
отличным от `core`. Имена стадий так не поймать; их закрывает то, что после
изменения их не на что использовать. Guard печатает число просмотренных
файлов, чтобы пустой обход не выглядел чистым.

## Risks / Trade-offs

- [Деревья незавершённой работы копятся] → `claim list` их показывает;
  освобождение — `claim release`. Это цена не терять работу.
- [Пилот теряет продолжение между релизами движка и пакета] → выпуск подряд;
  `project_continue_undeclared` называет причину; уже созданные Runs
  доводятся.
- [Отказ проверки дерева приходит первым шагом Run, а не до его создания] →
  отказ записан в истории; шаг дешёвый.
- [Шаг, убитый без результата, не сообщил checkpoint] → действует
  предыдущий checkpoint, а дерево передаётся целиком, так что оставленные
  файлы не теряются.
- [Guard ложно срабатывает] → разрешённые namespace перечислены явно; первый
  прогон печатает все совпадения.

## Migration Plan

1. Движок: ревизия 7, checkpoint и продолжение, передача claim,
   автоосвобождение по исходу, нейтральное восстановление, `recovery/2`,
   guard; удалить литералы пакета.
2. Пакет `prifly-aif-workflows`: checkpoint на шагах, меняющих дерево;
   `continuation` и первый шаг проверки в workflow продолжения. Новая
   редакция.
3. Пилот: обновить движок и пакет; довести новый Run до partial и до
   технического отказа; `project continue --prepare` и
   `project recover --prepare` показывают переданный claim и checkpoint.
4. Откат: предыдущий бинарник с предыдущей редакцией пакета; Runs нового
   движка остаются читаемой историей.
