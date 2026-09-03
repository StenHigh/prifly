## Context

См. [proposal.md](proposal.md). Сейчас `package.profiles` уже меняет bytes
компилируемого package, но `extend.yaml.profile` выбирает его как tracked
настройку. Поэтому `aif-classic` не может безопасно спросить Fast / Full /
Ultra для отдельной задачи: после такого вопроса выбранный skill mode может не
совпасть с уже sealed формой выходного plan artifact.

У Core уже есть versioned Project launch, package sealing, assisted session
handoff, CAS state и recovery. Есть также генерируемые runners Codex и Claude,
но они умеют спросить только стартовые детали и не хранят ответ как часть Run.
Upstream AI Factory skills прямо используют native question tool; Pri-Fly не
может надёжно перехватить произвольный вызов такого tool из чужого skill.

## Goals / Non-Goals

**Goals:**

- Один непрерывный предзапусковой диалог: выбор launch/workspace, Fast / Full
  / Ultra и все известные package вопросы. Profile идёт первым лишь как
  условие следующих страниц, а не как второй пользовательский процесс.
- Одна неизменяемая запись всех choices в конкретном Run и честный final
  ledger, одинаковый для CLI, Codex и Claude.
- Возможность безопасно оставить Run: только явно разрешённые package choices
  получают automatic value; остальное становится persisted waiting state.
- Generic Core: в нём нет имён AI Factory, моделей или prompt-парсинга.
- Один универсальный Decision Bridge принимает protocol любого совместимого
  executor; package не получает отдельный Core adapter из-за имени skill.

**Non-Goals:**

- Не извлекать вопросы из Markdown skill, transcript или model reasoning.
- Не считать `HANDOFF_MODE=1` AI Factory заменой решения Pri-Fly: это другой
  contract с собственными defaults и branch ownership.
- Не обещать autonomous execution для пакета, чей executor не объявил
  поддержку универсального versioned Decision Bridge. Такой пакет может
  пользоваться анкетой, но неизвестный вопрос остаётся обычной attended host
  interaction до адаптации самого package/skill к общему protocol.
- Не изменять старые package bytes, saved Runs, их schemas или evidence.

## Decisions

### 1. Один каталог, явные документы и один UI flow

`workflow.yaml` получает optional `decision_catalog` с exact
repository-relative путями. Каждый путь указывает на один YAML document
`prifly-run-decision/1`; документы могут лежать в `decisions/` на любой глубине
для удобства чтения. Путь сам не имеет semantic meaning, не сканируется по
glob и confined внутри package folder. Это оставляет маленький workflow
читабельным, но исключает скрытый вопрос из случайного файла.

`DecisionDefinition` содержит: stable id, title/description, phase
(`preflight` или `runtime`), answer schema/finite choices, requiredness,
recommendation/default, `automatic` policy, sensitivity (`ordinary`,
`scope-changing`, `approval-like`) и declared destination. Optional `when`
может проверять выбранный profile и exact typed answer уже объявленного
предыдущего preflight decision; все predicates должны быть true. Так после
`roadmap_linkage=link` появляется `roadmap_milestone`, но predicate не создаёт
stage, route или право и не допускает циклическую зависимость. Допустимы только
три destination: `package_profile`, declared launch/workflow input и named
assisted-session context value. Так catalog не может создавать graph, effect
или право. Full machine form остаётся доступной; краткая форма опускает только
текстовые presentation defaults.

Host runner сначала читает read-only launch questionnaire. Он показывает один
диалог: profile выбирается первым, затем вычисляются profile- и
already-answered-dependent questions в sealed declaration order, затем
показывается summary and confirmation. После подтверждения
runner передаёт все typed answers одним launch request. Поэтому пользователь
не редактирует `extend.yaml`, но compiler получает profile до compilation.

Альтернатива — отдельные `--profile` и затем ручной запуск — отвергнута: она
не решает проблему UX и легко создаёт несовместимую комбинацию choices.

### 2. Profile является per-Run override с явной precedence

`--package-profile NAME` добавляется к `project start` (и к explicit
`project compile` для reproducible preview). Его precedence: explicit value
конкретного Run, затем reviewed `extend.yaml.profile`, затем
`package.profiles.default`. Значение проверяется до создания temporary package
directory, Worktree claim, package import и Run. Выбранный profile и источник
попадают в launch receipt, package manifest and decision ledger.

`extend.yaml.profile` сохраняется как обратносуместимый team default, а не
удаляется и не переформатируется. Это минимальная migration: уже reviewed
project продолжает иметь тот же default, а новый Run может выбрать другой
profile без git diff.

Альтернатива — переместить profile из `extend.yaml` в RunBrief — отвергнута:
profile определяет compilation и capture contract, а RunBrief не является
настройкой package.

### 3. Sealed Decision Sheet отделяет ответы от skill prose

После validation compiler создаёт canonical `DecisionSheet/1`: catalog digest,
profile, all preflight answers, their source (`actor`, `project_default`,
`package_default` or `autonomous_policy`), and allowed destinations. Sheet
становится immutable Run input. Для assisted step Core передаёт only the
declared named session-context values and their source in versioned session
task; renderer может показать human-readable sheet, но worker не получает
authority изменить catalog или answers.

Для `aif-classic` package будет создана полная, pinned questionnaire map:

- `aif-plan`: Fast/Full/Ultra, tests, logging detail, documentation, roadmap
  linkage/milestone and explicit constraints;
- `aif-implement`: treatment of pre-existing uncommitted work, checkpoint
  commits, documentation checkpoint, roadmap completion and cleanup/merge
  choice where the installed skill exposes it;
- `aif-commit`: commit grouping/message confirmation and push choice.

Conditional entries appear only when their declared condition is true (for
example, a milestone after roadmap linkage). The map is checked against one
pinned upstream AI Factory skill revision. It does not silently claim that a
later upstream revision asks the same questions.

The package's host-facing context will receive the resolved sheet and invoke
the plan skill with the selected mode. Known catalog answers are instructions
to use, not questions to repeat. This uses the same universal Decision Bridge
contract as every package, not an AI Factory Core special case or a bespoke
adapter per skill.

### 4. Attended, autonomous and unattended have intentionally different meanings

`attended` is the normal default: every required preflight question without a
valid default receives a human answer. `autonomous` is an explicit launch
policy, not a synonym for “agent decides”: it may use only a catalog entry
marked automatic and only when its sensitivity is `ordinary`. Every such
choice is recorded as `autonomous_policy` with its recommendation.

`unattended` is a stronger label, reserved for a sealed catalog/policy that
cannot need a fresh human choice. An unknown decision, scope change, protected
effect or expired answer pauses; it never becomes an inferred “recommended”
answer. Local commit/push/MR choices remain explicit decision entries and do
not inherit permission from autonomous planning choices.

The alternative “the model chooses every unexpected answer” is rejected: it
would let an unreviewed question alter scope or publish a change while the
owner is absent.

### 5. Dynamic decisions use one universal bridge, not native chat side effects

A supported executor declares the universal Decision Bridge capability and may
send `DecisionRequest/1` for a declared runtime catalog ID while its Attempt
is active. The bridge is part of the generic assisted-session contract: it
does not contain a package, skill, provider or model name. Core atomically
records a pending request, puts the Run in `waiting_decision`, and withholds
successor routing and retry. `run decision answer` carries run ID, request
version/digest, decision ID and typed value; CAS makes the first accepted
answer the only one.

After a valid answer, Core creates a new delivery for the *same* logical
Attempt, adding the declared answer context. It does not rerun the prior
external operation or create a new step. Restart rebuilds the pending request
from events and shows the same question to any authorized host.

This requires an assisted-session protocol revision. Hosts that only know the
current session version receive an explicit unsupported diagnostic before a
package declaring runtime decisions is admitted. A raw AI Factory skill that
calls `AskUserQuestion` directly has not emitted `DecisionRequest`; Pri-Fly
will not pretend it captured the answer. `aif-classic` may enable autonomous
mode only after its tested package integration emits this common protocol or
proves all of its possible choices were preflighted.

The alternative of saving an answer in host transcript is rejected: it cannot
be replayed safely after reconnect, is not tied to one Attempt and is invisible
to another host.

### 6. Version boundaries and deliberate runner update

New persisted records and assisted session fields receive new schema versions;
older readers refuse an authority containing an unsupported decision event.
Existing Runs remain readable through their old schemas and have an empty
ledger rather than invented history. Generated runners are tracked project
files, so `project init` continues not to overwrite them. An explicit
`project runners update` command validates an exact prior template, replaces
only known generated files atomically and refuses locally modified runners;
the developer reviews and commits that runner change normally. Direct CLI
launch stays usable without the runner update.

## Risks / Trade-offs

- [A package catalog misses an upstream skill question] → the common bridge
  supplies a visible `waiting_decision` fallback for compatible skills; do not
  advertise unattended compatibility for a skill that bypasses it.
- [Questionnaire gets long] → conditional pages, concise labels and final
  summary; no second configuration file or separate “profile wizard”.
- [Persisted state/schema rollout breaks existing authority] → additive
  versioned events, migration fixture, explicit incompatible-reader refusal
  and a restart/replay check before write migration. Qualified backup/restore
  is not a current Pri-Fly capability and is not implied by this change.
- [Automatic choice is mistaken for permission] → sensitivity restriction,
  source displayed in both preflight and final ledger, and separate Approval /
  ActionIntent rules remain unchanged.
- [A manual runner has been customized] → update command refuses it instead of
  overwriting; its owner adopts the new template deliberately.

## Migration Plan

1. Add schemas, canonical parser/compiler and profile override while retaining
   current `extend.yaml.profile` as default; test no tracked file changes for
   Fast, Full and Ultra launches.
2. Add Decision Sheet, ledger, CLI questionnaire/answer read models and new
   assisted-session version with state migration/recovery tests.
3. Update generated Codex/Claude runners and publish the explicit runner
   update path; validate one uninterrupted questionnaire in each host.
4. Add `aif-classic` decision documents and the pinned upstream question map;
   initially expose only the autonomy level actually proven by its adapter.
5. Regenerate public schemas, run full gates and execute a bounded live pilot.
   Pilot evidence must distinguish preflight compatibility from dynamic
   decision bridge qualification.
