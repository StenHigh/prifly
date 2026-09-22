## Context

См. proposal.md. `run reopen` сохраняет пройденные стадии, но обязан использовать старый lock. `project continue` закрепляет новый пакет, но создаёт Run с entry quality-tail; это повторяет тяжёлые `verify` и `review`. В source `run:ae20…` процесс tests settled с exit 0 и журнал содержит `attempt.result_candidate` с immutable evidence ref, затем приёмка отказала на `extra_filters`; файл output в scratch сам по себе не является принятым ArtifactRevision. Текущий новый continuation `run:67c669…` ждёт хоста на verify и не является восстановлением этой точки.

Source set: `openspec/specs/runtime-resources/spec.md`, `domain-execution/spec.md`, `cli-protocol/spec.md`, `local-run-monitor/spec.md`; OpenSpec change не входит в runtime. См. соответствующие delta specs. Прежние state/read/event версии и исторические Runs не переписываются.

## Goals / Non-Goals

**Goals:**

- Отдать оператору до запуска точную стоимость восстановления: что reuse, что revalidate, что придётся исполнить.
- Сохранить старую историю и начать новый связанный Run на первом недоказанном действии.
- На реальном случае пройти от технически failed tests к terminal outcome без повторного запуска навыков verify/review; tests повторять только если сохранённых output bytes не хватает.

**Non-Goals:**

- Не мигрировать lock или state старого Run in-place.
- Не объявлять похожие названия стадий эквивалентностью и не переносить approvals, grants либо внешние effects.
- Не делать произвольный replay любого graph: неподдерживаемый topology/контракт отвергается до запуска.
- Не превращать успешное завершение процесса в accepted StepResult без проверки bytes и schema.

## Decisions

### Новый связанный Run, не смена пакета у старого

Использовать новый versioned recovery command с source Run/version и target package, сохраняя неизменность source и общую модель `ForkProvenance`. Добавить отдельное provenance о карте reused activation/result/candidate; не перегружать существующий reason как единственный носитель смысла. Новый Run получает собственный lock, ids, budgets и admission. Альтернатива in-place migration потребовала бы подменить исполняемый контракт исторического Run и сделать старые события зависимыми от нового interpreter; она противоречит текущему default правилу в `domain-execution`.

### Планировать префикс по фактам, не по позиции в YAML

Read-only planner строит ordered executed trace source до первой технически failed StageActivation, включая дочерние WorkflowInvocations и control decisions. Он сопоставляет этот trace с target graph по effective definitions, bindings и фактическим data subjects; совпадения строковых stage ID недостаточно. При несовпадении до frontier — явный refusal с первой причиной. Поддержать в первой версии реально используемый sequential quality-tail с вложенными verify/review loops; parallel/map/wait и unknown control paths не притворяются поддержанными. Принятые StepResults и их ArtifactRefs остаются source evidence; новый Run записывает отдельные reuse decisions/provenance, а не фиктивные новые Attempts. Альтернатива создать готовые StepInstances как будто они исполнялись в новом Run отвергнута как ложная история.

### Проверять предмет качества отдельно от package identity

Compiled package `/3` меняет refs всех owned components при изменении одного файла. Поэтому сравнивать только exact ref означало бы всегда повторять всё. Planner сравнивает canonical effective contract префикса после разрешения refs и отдельно exact subject: входные ArtifactRefs, дерево Git, executor/tool bytes, context resources, checks и решения. Для gate, который измерял целое дерево, нужен тот же tree hash; изменение только authoring package в primary checkout не означает, что claimed code tree можно молча заменить. Recovery может взять проверенный commit исходного claim как code subject, пока он доступен и новый WorktreeClaim создаётся честно. Если нового такого tree нет, reuse gate отказывает. Дедупликация артефактов не переносит старые полномочия.

### Candidate: revalidate прежде повторного exec

Planner читает `attempt.result_candidate` по sealed EvidenceRef и проверяет process settlement. Он отдельно проверяет сохранённость каждого declared output ArtifactRevision; scratch paths не допускаются. Если все bytes доступны, новый Run записывает повторную оценку этих же bytes новым result/port schema и checks, без новой execution Attempt. Если кандидат есть, но output не sealed, план указывает `execute failed stage`; перед admission перепроверяются absence unresolved effects и retry/effect policy. Сохранённый exit 0 — только свидетельство процесса, не вердикт. Это позволяет нынешнему случаю честно показать, можно ли сохранить 17-минутный tests, ещё до запуска.

### Одна граница prepare/start и совместимая проекция

`project recover --source-run … --launch … --prepare` компилирует target package и возвращает digest плана, но не import/claim/Run. Start повторно проверяет source version, package digest, Git tree, trust/current access и resource state, затем атомарно создаёт linked Run с replayed evidence map либо отказывает. Клиенту отдаются новые versioned DTO/events; прежние формы не получают изменённый смысл. Monitor читает provenance из authority, а не восстанавливает его эвристикой по графу.

## Risks / Trade-offs

- [Ошибочная эквивалентность дорогого gate] → строгий effective-contract/subject check и тест, где одна правка дерева или decision запрещает reuse.
- [Candidate есть, output не sealed] → не читать scratch как evidence; показать причину и повторить только failed stage, если безопасно.
- [Вложенный route расходится при реконструкции] → fail closed до Run creation; в первой версии ограничить поддерживаемый topology и покрыть реальный quality-tail fixture.
- [Source worktree исчез или занято другое владение] → опираться на подтверждённый Git commit/tree, создать новый claim по текущим правилам; отсутствие exact tree даёт отказ.
- [Новый package вводит более строгую проверку на reused output] → валидировать reuse под новым контрактом или отказать; прежний pass не наследует новую проверку автоматически.
- [Стоимость feature выше одного повторного прогона] → мерить число реально пропущенных дорогих стадий и время на приёмочном сценарии, но не ослаблять проверку ради метрики.

## Migration Plan

1. Ввести новые state/read/protocol editions только для recovered Runs; старые readers и сохранённые bundles оставить совместимыми, unknown edition отвергать явно.
2. Добавить deterministic fixtures: schema-invalid tests после принятого verify/review, candidate с/без sealed outputs, changed Git tree, stale prepare, unresolved effect и crash после create/replay.
3. Собрать Pri-Fly, обновить managed binary и project runner после проверок, затем применить `project recover --prepare` к `run:ae20…`; показать фактический план reuse до запуска.
4. Запустить восстановление только если план подтверждает, что verify/review не выполняются повторно. Source Run останется failed; новый Run довести по выдаваемому `run next` до outcome. Rollback — прекратить новые recoveries; уже созданный связанный Run остаётся читаемой историей, старый Run не меняется.
