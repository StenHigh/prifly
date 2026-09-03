## Context

`docs/spec/02-domain-execution.md` содержит 26 legacy requirements о
предметной семантике и protocol исполнения Pri-Fly. `docs/task-input.md`
описывает единый source-neutral вход выбранной внешней работы. Связь legacy
requirements с acceptance cases хранится в
`docs/roadmap/requirements-map.csv`.

По `openspec/SOURCE-OF-TRUTH.md` эти два файла остаются единственным current
source set до завершения change; `domain-execution` станет source of truth
только после cutover. Product-model сохраняет высокоуровневое правило, что
работа имеет RunBrief до выполнения; exact contract `TaskInput/1` после
cutover принадлежит domain-execution и не получает второго schema-like
описания.

## Goals / Non-Goals

**Goals:**

- Перенести каждую из 26 смысловых границ главы и один контракт intake в
  descriptive OpenSpec requirements со scenario.
- Сохранить проверяемую связь legacy requirements и acceptance cases только в
  archive change, без старых идентификаторов в постоянном spec.
- Сделать cutover обратимым до final cleanup: legacy source остаётся в дереве
  без изменения bytes.

**Non-Goals:**

- Не менять Go runtime, CLI, YAML frontend, JSON wire contracts, schemas,
  acceptance evidence, roadmap status или поведение task readers.
- Не удалять legacy chapters, CSV maps, manifests либо historical evidence.
- Не делать TaskInput обязательным входом workflows, которые не принимают
  внешнюю задачу, и не добавлять GitLab/GitHub/Jira integration.

## Decisions

### Одна смысловая граница на legacy requirement

Постоянный spec содержит 27 descriptive requirements: по одному на каждую
границу execution chapter и один на TaskInput/1. Это сохраняет reviewable
granularity без прежней системы номеров. Объединение нескольких boundary в
широкий абзац отвергнуто: оно лишило бы crosswalk проверяемости.

### Exact TaskInput contract живёт вместе с execution semantics

`TaskInput/1` описывает immutable pre-Run resource, confirmation и
source-neutral reader boundary, поэтому его detailed contract переносится в
`domain-execution`. Высокоуровневое product rule о RunBrief не становится
альтернативной схемой intake; изменения полей, preparation или source reader
должны менять только domain-execution.

### Legacy IDs живут только в этом crosswalk

Таблица ниже — единственное место change с `DOM-*` и `AC-*`. После archive
она остаётся проверяемой историей перехода; `openspec/specs/domain-execution`
использует только понятные headings. Альтернатива — оставить IDs в постоянном
spec — отвергнута по решению владельца как сохранение custom tracking format.

### Cutover происходит после content review и strict validation

Apply сверяет каждую строку crosswalk с source set и permanent spec,
проверяет links acceptance cases, затем переключает ровно одну строку source
map. Legacy source не редактируется и не удаляется. До final cleanup rollback
означает возврат одной map row, а не восстановление historical bytes.

### Legacy coverage crosswalk

| Legacy source | Legacy record | Acceptance cases | Replacement requirement | Review |
|---|---|---|---|---|
| `docs/spec/02-domain-execution.md#области-и-участники` | `DOM-001` | `AC-006`, `AC-013`, `AC-090`, `AC-093` | «Области исполнения имеют независимую идентичность» | verified |
| `docs/spec/02-domain-execution.md#словарь-ресурсов` | `DOM-002` | `AC-014`, `AC-099` | «Ресурсы Run имеют разные роли и неизменяемые revisions» | verified |
| `docs/spec/02-domain-execution.md#ссылки-и-версии` | `DOM-003` | `AC-017`, `AC-050` | «Вложенное исполнение сохраняет единый Run» | verified |
| `docs/spec/02-domain-execution.md#замороженный-состав-запуска` | `DOM-004` | `AC-019`, `AC-020` | «Run закрепляет полный состав исполнения» | verified |
| `docs/spec/02-domain-execution.md#форматы-и-канонизация` | `DOM-005` | `AC-056` | «Машинные definitions имеют однозначную форму» | verified |
| `docs/spec/02-domain-execution.md#inputs-и-outputs` | `DOM-006` | `AC-033`, `AC-034`, `AC-067` | «Связи данных объявляются точными портами» | verified |
| `docs/spec/02-domain-execution.md#артефакт-и-публикация` | `DOM-007` | `AC-014`, `AC-055`, `AC-075` | «Artifact принимается только после sealing» | verified |
| `docs/spec/02-domain-execution.md#утверждения-и-evidence` | `DOM-008` | `AC-068`, `AC-111` | «Evidence подтверждает конкретное утверждение» | verified |
| `docs/spec/02-domain-execution.md#повторное-использование` | `DOM-009` | `AC-027`, `AC-069`, `AC-138` | «Reuse требует совместимых оснований» | verified |
| `docs/spec/02-domain-execution.md#роль-ядра` | `DOM-010` | `AC-002` | «Core детерминированно управляет control loop» | verified |
| `docs/spec/02-domain-execution.md#ход-исполнения` | `DOM-011` | `AC-071` | «Исполнение проходит через проверяемые границы control loop» | verified |
| `docs/spec/02-domain-execution.md#executionenvelope` | `DOM-012` | `AC-057`, `AC-058` | «ExecutionEnvelope ограничивает одну допущенную попытку» | verified |
| `docs/spec/02-domain-execution.md#конкурентность-и-принятие-результата` | `DOM-013` | `AC-072`, `AC-077` | «Конкурентные команды принимаются атомарно» | verified |
| `docs/spec/02-domain-execution.md#время` | `DOM-014` | `AC-120` | «Время приходит от доверенного источника» | verified |
| `docs/spec/02-domain-execution.md#run-lifecycle` | `DOM-015` | `AC-088` | «Run завершает только доказанный lifecycle outcome» | verified |
| `docs/spec/02-domain-execution.md#step-и-attempt-lifecycle` | `DOM-016` | `AC-032`, `AC-078`, `AC-080`, `AC-089` | «Step и Attempt различают предметный и технический исход» | verified |
| `docs/spec/02-domain-execution.md#skip-waiver-и-no-work` | `DOM-017` | `AC-009`, `AC-036`, `AC-105` | «Skip, waiver и no_work не равны pass» | verified |
| `docs/spec/02-domain-execution.md#actionintent-до-согласования` | `DOM-018` | `AC-096` | «Intent фиксирует effect до approval» | verified |
| `docs/spec/02-domain-execution.md#approval-и-grant` | `DOM-019` | `AC-095`, `AC-096`, `AC-097`, `AC-104` | «Approval и Grant имеют ограниченную силу» | verified |
| `docs/spec/02-domain-execution.md#известный-и-неизвестный-эффект` | `DOM-020` | `AC-082`, `AC-083`, `AC-085`, `AC-086` | «Неизвестный внешний effect блокирует обычную работу» | verified |
| `docs/spec/02-domain-execution.md#ручное-вмешательство` | `DOM-021` | `AC-114` | «Ручной результат сохраняет evidence и границы policy» | verified |
| `docs/spec/02-domain-execution.md#условия-и-ветви` | `DOM-022` | `AC-037`, `AC-038`, `AC-040` | «Условия графа используют закрытую типизированную семантику» | verified |
| `docs/spec/02-domain-execution.md#циклы-fan-out-и-subworkflow` | `DOM-023` | `AC-045`, `AC-048`, `AC-050` | «Композиция workflow ограничена и учитывается целиком» | verified |
| `docs/spec/02-domain-execution.md#изменение-определения-во-время-run` | `DOM-024` | `AC-005` | «Изменение definition создаёт новую совместимую работу» | verified |
| `docs/spec/02-domain-execution.md#события-и-запуск-по-расписанию` | `DOM-025` | `AC-015`, `AC-051`, `AC-052`, `AC-054`, `AC-121` | «Event и schedule имеют явные границы автоматизации» | verified |
| `docs/spec/02-domain-execution.md#контроль-изменений-самого-pri-fly` | `DOM-026` | `AC-116`, `AC-128` | «Эволюция протокола не переписывает историю Run» | verified |
| `docs/task-input.md` | `TaskInput/1` | нет отдельного legacy ID | «Внешняя задача проходит через нейтральный immutable intake» | verified |

## Risks / Trade-offs

- [Перефразирование изменит protocol boundary] → reviewer сравнивает каждую
  строку crosswalk с source heading и назначенными acceptance cases.
- [TaskInput будет иметь две редактируемые схемы] → detailed field contract
  есть только в domain-execution; product-model сохраняет лишь pre-Run goal.
- [Временная параллель документов станет второй правдой] → source map остаётся
  на legacy до завершённой проверки, а replacement spec не меняется отдельно
  от этого change.
- [Документальная проверка будет выдана за release evidence] → tasks явно
  отделяют OpenSpec validation от существующих product gates.

## Migration Plan

1. Проверить 26 legacy headings, TaskInput contract, 27 crosswalk rows и
   acceptance links.
2. Проверить strict OpenSpec validation для change и будущего main spec.
3. После content review отметить строки crosswalk `verified`, переключить
   строку `Исполнение и вход задачи` source map на
   `openspec/specs/domain-execution` и убедиться, что product-model не
   становится второй intake schema.
4. Архивировать change; legacy source set и CSV maps оставить неизменёнными
   до final release cleanup.
