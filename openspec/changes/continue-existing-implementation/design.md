## Context

См. proposal.md. `run reopen` намеренно применим только к технически
сломавшемуся Run; `run fork` создаёт новый Run из sealed refs, но начинает
workflow с его entry. Ни один механизм не может безопасно превратить accepted
`partial` в незавершённую стадию.

Source Run уже содержит sealed task, warmup handoff, plan и прежнюю
Implementation. После terminal outcome владелец может вне Run внести исправление
и закоммитить его; continuation обязан принять именно этот новый Git факт, а
не притвориться, что его создал старый Attempt.

## Goals / Non-Goals

**Goals:**

- Новый Run использует source evidence и новую committed Implementation.
- Новый graph начинает quality tail, поэтому implement не может дать `no_work`.
- Оператор получает одну Project-команду и checked summary, а не ручной JSON.

**Non-Goals:**

- Не менять terminal lifecycle или evidence source Run.
- Не переносить grants, approvals, claims, attempts и model provenance source
  Run.
- Не делать continuation универсальным переходом к произвольной стадии и не
  повторять технический recovery, который уже покрывает `run reopen`.

## Decisions

### Отдельный continuation launch, не ветка normal `aif-classic`

`aif-classic-continuation` будет отдельным derived Project workflow folder с
root inputs `task`, `handoff`, `plan` и `implementation`. Его graph начинается
с verify batch и содержит только verify/security/review/commit и их declared
fix loops. Это сохраняет обычному `aif-classic` неизменный input contract и
делает отсутствие implement структурно невозможным.

Folder генерируется из canonical `aif-classic` тем же проверяемым способом,
что и `aif-profiled`: shared schemas/contexts остаются byte-identical, а root
и только затронутые components получают собственные IDs/versions. Package
также объявит сохранение project `extend.yaml` и project subtree при update.

Альтернатива — optional continuation inputs и choice в classic root — отвергнута:
она усложняет доказательство required ports на двух путях и меняет contract
обычного launch, где отсутствие implementation сегодня корректно.

### CLI создаёт current Implementation из Git, не из host prose

Новая команда `project continue` принимает `--source-run`, declared continuation
launch, selected host/workspace и те же policy/input options, что `project
start`. Она читает source Run через runtime API, выбирает только declared
source task/handoff/plan и последний accepted implementation output, затем до
claim проверяет чистый repository HEAD: source base должен быть его предком.
Она вычисляет `changed_files` через Git между source base и current HEAD и
seal-ит полученный `implementation` как imported input с provenance continuation
command. Поэтому post-terminal commit виден как новый факт нового Run, а не
выдуманный output прежнего implement Attempt.

`--prepare` возвращает source Run ID, все exact refs, derived Implementation,
workspace и review digest. Start требует этот digest и заново сверяет source
run/version, Git HEAD и refs перед созданием claim/Run.

Альтернатива — заставить host вручную написать Implementation JSON — отвергнута:
она допускает неверный head/changed_files и заставляет модель разбирать state.

### Continuation — специализированный fork, а provenance остаётся читаемым

Source Run должен быть terminal в той же authority и иметь ровно требуемые
sealed artifacts; подходящие outcomes — `partial` и `rejected`, чтобы
continuation закрывала не только один route, но не становилась повторным
запуском successful Run. Runtime переиспользует существующий `ForkProvenance`:
новый Run хранит source Run ID/version и exact refs. `Start` помещает
проверенные bytes internal artifacts в типизированные входы нового package;
их ArtifactRevision содержит provenance исходной ревизии, поэтому старые
refs не выдаются за входы с новой schema. Отдельный continuation entry point
валидирует эту связь, а raw `run fork` сохраняет правило reuse только root
outputs. Claim подготовленного workspace привязывается к новому Run атомарно
с `run.created`: первый read-only verify может проверять материализованное
дерево без фиктивного пишущего шага. Monitor показывает связь как
«continuation от Run», не как stage source Run.

Альтернатива — отдельный continuation lifecycle — отвергнута: уже существующий
fork даёт ровно нужные новый Run и immutable source provenance. Произвольный
raw `run fork` остаётся недостаточным: он не вправе извлекать internal
artifacts и не доказывает post-terminal commit.

## Risks / Trade-offs

- [Исходная Implementation не предок текущего HEAD] → отказ с точным
  diagnostic; владелец переносит нужный commit, не получает silent review чужой
  ветки.
- [Continuation package расходится с classic tail] → generator, byte-sync test
  и package version gate; hand edits derived folder запрещены.
- [Source Run содержит несколько implement rounds] → launcher выбирает
  последний accepted implementation output по sealed order и показывает его в
  summary; другой выбор требует нового command input, не эвристики.
- [Пакетный update ещё не установлен в проект] → command отказывает, если
  declared launch отсутствует; он не правит shared profile автоматически.

## Migration Plan

1. Выпустить Pri-Fly с `project continue` и continuation provenance reader.
2. Выпустить `aif-classic-continuation` и derived profiled counterpart в
   `prifly-aif-workflows`.
3. Установить/update folders в SMSPlace, добавить explicit continuation launch
   и прогнать текущий source Run как acceptance scenario после переноса
   `239adf986` в clean development.
4. Rollback — не запускать новый continuation launch и оставить source Run;
   уже созданный continuation Run остаётся самостоятельным historical record.
