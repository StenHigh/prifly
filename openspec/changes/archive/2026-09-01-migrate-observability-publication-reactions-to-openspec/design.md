## Context

См. [proposal.md](proposal.md). `docs/roadmap/state-and-telemetry.md` — один
нормативный source set: 24 правила наблюдаемости, 6 правил публикаций и 14
правил реакций, а также 45 + 14 + 29 самостоятельных acceptance cases. Он
сейчас ошибочно включён в строку delivery roadmap в
`openspec/SOURCE-OF-TRUTH.md`, хотя порядок поставки и нормы поведения — разные
вещи.

## Goals / Non-Goals

**Goals:**

- Перенести 44 правила в один постоянный OpenSpec capability с понятными
  названиями и без legacy authoring IDs.
- Сохранить все 88 Given/When/Then cases, их owner stage, текущий declared
  execution status, requirement links и границу evidence.
- Отделить ownership будущего product contract от delivery/status документов.

**Non-Goals:**

- Не реализовывать F2, telemetry query, hooks, artifact subscriptions, guards
  или их JSON Schema.
- Не менять current status, evidence, roadmap phase, Go code, schemas,
  manifests либо historical release records.
- Не переносить delivery roadmap, F2 progress или опубликованные contracts в
  этом change.

## Decisions

### Постоянная спецификация говорит поведением, архив хранит старые ключи

Постоянный `observability-publication-reactions/spec.md` использует 44
семантических requirement headings. Legacy `OBS`, `PUB`, `REA` и case IDs
живут только в `archived-crosswalk.md`: там остаются ID, owner/completion
stage, exact legacy requirement links и ссылки на понятные permanent headings.
Это соответствует уже перенесённым capabilities и не создаёт второй вечный
язык редактирования.

| Historical rule family | Count | Permanent section |
|---|---:|---|
| `OBS-001` … `OBS-024` | 24 | измерения, read views, catalog, query, collection и analysis |
| `PUB-001` … `PUB-006` | 6 | contracts hooks, publication, artifacts, subscriptions и phases |
| `REA-001` … `REA-014` | 14 | planner, bindings, watch, guards, races и limits |

### Перенос требует полный acceptance corpus, а не representative examples

Candidate уже фиксирует все 44 поведенческие границы. До sync он будет
расширен из legacy source до ровно 88 individually named scenarios: 45 OBS,
14 PUB и 29 REA. Нельзя подменять этот corpus одним широким «проверить
телеметрию» или принять document checker за выполненный runtime test. Каждый
сценарий сохраняет Given/When/Then, owner stage и declared status. В legacy
карте есть как `passed` F1 cases, так и `specified_not_executed` future cases:
перенос сохраняет это фактическое различие, но не подменяет историческое
evidence новой квалификацией. Permanent имена читаемы, архив сопоставляет их с
прежними ID.

### Ownership split происходит одной картой после coverage check

После полного candidate coverage `SOURCE-OF-TRUTH.md` получает отдельную
строку `Наблюдаемость, публикации и реакции` с current source set
`docs/roadmap/state-and-telemetry.md` и final path
`observability-publication-reactions`. Строка delivery roadmap больше не
называет этот файл, но продолжает указывать только delivery/status source set.
Legacy source остаётся byte-identical до единой final cleanup; rollback —
обратное переключение одной строки карты.

### Legacy map сохраняется как evidence migration, а не runtime evidence

Apply извлекает из `docs/roadmap/acceptance-map.csv` точные 88 строк OBS/PUB/
REA в архив change и сверяет их с archived crosswalk. Это фиксирует прежнюю
traceability, но не меняет status и не объявляет сценарий выполненным.

## Risks / Trade-offs

- [Один case потерян при удалении ID] → count 45/14/29, individual crosswalk и
  проверка каждого archived case ID до sync.
- [Future F2 rule ошибочно объявлен реализованным] → сохранить фазу, declared
  status и явный `specified_not_implemented` boundary в permanent scenarios.
- [Delivery документы снова дублируют product semantics] → отдельная source-map
  строка и запрет включать `state-and-telemetry.md` в delivery migration.
- [Историческое evidence меняется ради новой структуры] → защищённый diff для
  `docs/evidence/**`, обоих manifests, schemas и source документа.

## Migration Plan

1. Расширить candidate 44 requirements до полного 88-scenario corpus и
   построить archived crosswalk 44 rule rows + 88 case rows.
2. Скопировать точные 88 OBS/PUB/REA entries из acceptance map в archive,
   проверить count, stage/status/links и отсутствие legacy IDs в permanent
   candidate.
3. Sync capability в main specs, заменить единственную source-map строку и
   проверить byte-identical legacy source/evidence/contracts.
4. Архивировать change, выполнить strict OpenSpec validation и оставить
   physical removal legacy sources только для final cleanup всего проекта.
