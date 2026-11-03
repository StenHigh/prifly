## Context

См. [proposal](proposal.md) и [issue #19](https://github.com/StenHigh/prifly/issues/19).

`projectResolveWorktreeNaming` проверяет binding `input` через `plan.Workflow.Inputs`. Closed profile `/4` не допускает отдельного объявления. Extension `/5` меняет композицию и points, но не добавляет root input. Локально установленный официальный `aif-classic v1.57.0` объявляет закрытую task schema без source/ID.

Prepare/start уже имеют review digest, naming preview и stale check. `WorktreeNamingRequest` хранит только base/policy. `WorktreeNameAllocation` хранит base/policy/ordinal/phase в `authority-claims/4`. Для сохранения полных исходных naming values требуется отдельная версия claim metadata.

## Goals / Non-Goals

**Goals:** минимальный отдельный контракт потребителя без изменения native graph; полная freshness до effects; продолжение по сохранённому snapshot.

**Non-Goals:** schema registry, новые workflow ports, произвольные runtime metadata, tracker lookup, автоматический выбор launch и изменение allocation policy.

## Decisions

### 1. Profile five owns naming declarations

Определяем новый profile `/5`. Project хранит schema рядом со своей конфигурацией, вне upstream inventory:

```yaml
schema_version: prifly-project-profile/5
worktree_naming:
  inputs:
    identity:
      schema: .prifly/naming/identity.schema.json
  template: '{project}-{source}-{item}'
  bindings:
    project: {value: demo}
    source: {naming_input: identity, pointer: /source}
    item: {naming_input: identity, pointer: /id}
# packages/launches/hosts сохраняют прежнюю форму.
```

Schema — обычный самодостаточный JSON Schema 2020-12. Разрешаем только fragment `$ref` внутри документа. Первый вариант не имеет defaults и schema imports. Имена inputs используют существующие ограничения Project port names. В первой редакции предел — 16 naming inputs; каждый должен использоваться хотя бы одним binding.

Разделение namespace сохраняет прежнее значение `input`. Альтернатива с добавлением workflow port расширяет graph и усложняет обновление upstream. Inline schema раздувает общий profile и хуже показывает ownership файла. Оба варианта отклонены.

### 2. One explicit value flag is sufficient

Добавляем повторяемый `--naming-input NAME=FILE` для questionnaire prepare/start. Обычный questionnaire показывает schema и условную применимость, не требует значения. Compile не требует runtime values.

JSON reader и validator используют существующие механизмы и ограничения размера JSON inputs. Дополнительно весь persisted snapshot ограничен 128 KiB canonical UTF-8 JSON (131072 bytes), включая declaration, schemas, values, digests и остальные поля snapshot. Это общий предел, а не предел одного input. Reader не следует symlinks и не разрешает network references. Неизвестный/повторный NAME, отсутствие обязательного value, schema mismatch и invalid pointer получают типизированные `runtime.Fault` в существующем naming семействе. Новые коды добавляются только при отдельном значении отказа.

При checkout или отсутствии Workspace Core не требует naming values. Если caller всё-таки передал их, CLI отказывает как неприменимому аргументу. Так CLI не создаёт видимость сохранённой metadata.

`--naming-input-ref` не нужен для минимального контракта. Он потребовал бы Artifact/schema identity и дополнительного lifecycle. Уже существующие `--input`/`--input-ref` относятся только к workflow.

### 3. Review covers the complete snapshot

CLI читает declaration, schema и JSON values до authority mutation. Canonical JSON определяет schema/value digests. Изменение только whitespace не меняет canonical value; изменение любого JSON field меняет digest.

```text
project profile + schema files + --naming-input files
                       |
                       v
              typed validated snapshot
                       |
                       v
          prepare summary + review_digest
                       |
                 reread and compare
                       |
                       v
           durable claim -> worktree -> Run
```

Summary `/8` показывает schema paths/digests, value sources, canonical values/digests и обычный naming review. Источники и snapshot входят в review digest; availability preview не входит. Проверка freshness располагается до preflight и любых start effects, включая QueueReserve/QueueDispatch. Сейчас non-queue preflight выполняется раньше compilation; новый `/5` путь переносит snapshot validation/freshness перед этим effect. Повторное чтение перед reservation закрывает изменение файлов во время подготовки start. Legacy порядок сохраняется.

Данные reservation берутся из проверенного snapshot, а не перечитываются внутри transform. Read-only prepare не импортирует schemas/values как artifacts. Naming declaration не входит в compiled workflow closure, поэтому не меняет refs upstream.

### 4. Persist snapshot in claim metadata

Добавляем snapshot в новые Go/JSON claim types и `authority-claims/5`. Snapshot содержит declaration, самодостаточные schemas и полные canonical values с digests. Его общий предел 128 KiB согласован владельцем 2026-11-03. Отдельный registry/blob lifecycle не нужен.

Новая запись идёт через обычный typed claim command payload. Snapshot участвует в semantic idempotency digest. Старые claims читаются без синтеза snapshot. Snapshot хранится отдельным полем claim, чтобы allocation сохранил сравнимость; копирование и сравнение snapshot проверяют содержимое. Изменение старого allocation struct под прежним wire version запрещено.

Authority command/result ограничен 1 MiB. Payload phase update содержит before/after; start result повторяет snapshot, а queue использует base64 для summary/result. До effects проверяем общий snapshot limit и фактические либо консервативно ограниченные serialized payload/result carriers. Переполнение snapshot даёт typed `project_worktree_snapshot_too_large`. Global command/result limits не повышаются.

Новый `/5` claims record читается bounded state decoder с existing ceiling 16 MiB и strict unknown-field checks. Legacy readers/versions сохраняют прежнюю границу. Полный prospective record проверяется перед reservation; transform повторяет проверку текущего snapshot до mutation. Это закрывает скрытую прежнюю границу `decode → flow.Parse` в 2 MiB без увеличения storage budget.

Handover копирует snapshot без чтения текущих schema/value files. Continue/resume/recover используют source claim при штатной проверке ancestry/ownership. Если текущий continuation требует новый claim от commit, allocator использует сохранённую основу и прежний snapshot. Naming override в continuation отказывается. Независимый `project start` создаёт новый snapshot по явным значениям.

Исходные workflow inputs продолжают передаваться существующим механизмом. Naming metadata не становится входом шага и не расширяет effects.

### 5. Publish every affected closed view

Публикуем profile `/5`, questionnaire `/5`, launch summary/start `/8`, authority claims `/5` и новый claims read contract. Перед кодом составляем список nested claim carriers в continuation/recovery и Project outputs. Каждый расширяемый closed carrier получает новую версию; старые definitions не меняются.

Новый DTO включает queue и composition fields. Version selection учитывает source snapshot даже без текущей policy. Legacy пути без snapshot сохраняют прежнюю минимальную версию.

### 6. Separate source checks from released acceptance

Source tests воспроизводят positive/negative cases. Integration fixture проверяет чистую consumer-конфигурацию и upstream inventory без сети.

Официальный package проверяется pinned release `v1.57.0` и реальной установкой/обновлением с recorded origins. Launch использует его прежние task/security inputs. Проверка prepare/start не требует прохождения всех AI стадий: после проверки claim Run останавливается штатно с явным итогом.

Generic native CLI probe использует минимальный fixture с repository-writing assisted contract и synthetic session reports. Он не вызывает LLM и не содержит приложение. Он проверяет обычный session protocol; отдельный runner не создаётся. Existing managed program scratch не объявляется claimed Workspace ради probe.

Probe получает exact executable path. Source-built binary и официальный released binary имеют разные evidence. Пункт приёмки released Core остаётся открытым до опубликованного совместимого релиза и запуска probe на нём.

## Risks / Trade-offs

- Хранение полного JSON увеличивает claim record → общий snapshot limit 128 KiB, bounded full record и предварительная проверка serialized command/result carriers.
- Новые поля могут протечь в старые nested views → inventory carriers, closed schema tests и legacy output regression.
- Проверка только выбранных pointers пропустит изменение metadata → digest покрывает полный JSON и schema.
- Текущий profile может утратить naming files → продолжение использует сохранённый snapshot, а не filesystem.
- Синтетический probe можно принять за AI qualification → отчёт явно фиксирует protocol scope и released build.

## Migration Plan

1. После согласования proposal закоммитить planning artifacts и запустить `aif-classic` по задачам этого change.
2. Добавить новые readers, schemas и tests без изменения profiles `/2`–`/4` и historical evidence.
3. Опубликовать Core с новыми contracts; выполнить отдельный probe на exact released executable.
4. Потребитель явно переводит shared profile на `/5` и добавляет собственную schema. Upstream package не меняется.
5. При откате конфигурации убрать новый opt-in только для будущих запусков. Authority с `/5` требует совместимый binary; старый reader отказывает.
