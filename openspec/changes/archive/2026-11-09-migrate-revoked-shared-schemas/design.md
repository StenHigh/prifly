## Context

Причина и scope описаны в `proposal.md`. Источник требования — issue [#21](https://github.com/StenHigh/prifly/issues/21).

`packageAdmissionGate` в `internal/runtime/package_lifecycle.go` сравнивает exact component refs со всеми сохранёнными packages. Совпадение с `PackageRevoked` немедленно запрещает admission. Параметр `requireResolvable` различает новый Start и работу сохранённого Run, но не различает поставщиков одинакового schema Ref.

`revokedPin` использует тот же gate для последующих admissions. `reuseTrustPin` отдельно проверяет доверие источникам результатов. Поэтому исключение только в Start не решает задачу. Глобальное разрешение Ref также затронет старые Runs.

`authority-packages/1` хранит status, status reason и observation последнего изменения. Он не хранит отдельную generation каждого package lifecycle. Повторный revoke с новым command id допускается. Timestamp или текст reason не дают надёжного различия двух отзывов.

Store уже предоставляет `ApplyAuthority`, `ExpectedCut`, `RunPins` и `AuthorityGuards`. Эти средства позволяют проверять preconditions в той же write-транзакции. Runtime и CLI не должны получать типы SQLite driver.

Нормативные источники перечислены в proposal; ownership остаётся прежним. Delta specs описывают будущий контракт. Текущие schemas и код пока его не реализуют.

## Goals / Non-Goals

**Goals:** один reviewed путь для новой работы в той же authority. Решение покрывает schemas из точного consumer package и его объявленных зависимостей. Оно сохраняет происхождение допуска в новом Run и повторно проверяется перед работой.

**Non-Goals:** изменение исполняемого контракта старого Run, trusted reuse его результатов, восстановление revoked package, автоматический выбор нового supplier. Миграция не исполняет package code и не изменяет Workspace или claims.

## Decisions

### 1. Отдельные план, решение и binding

Использовать три различных факта:

1. `SchemaMigrationPlan` — read-only снимок scope, диагностики и preconditions.
2. `SchemaMigrationDecision` — записанное решение владельца по digest плана.
3. `SchemaMigrationBinding` — происхождение допуска, закреплённое конкретным новым Run.

```text
exact consumer + выбранные schema suppliers
                     |
                 prepare
                     |
         полный план + review digest
                     |
             owner apply + reason
                     |
          сохранённое решение миграции
                     |
       явный Start --schema-migration ID
                     |
     binding нового Run -> каждый admission
                     |
          актуальные trust/lifecycle guards
```

Альтернатива «любой trusted supplier отменяет revoke» отвергнута: она меняет действующий security contract без отдельного решения. Альтернатива автоматически применять последнее решение отвергнута: она скрывает выбор владельца и расширяет scope обычного Start.

### 2. Consumer и suppliers выбираются по exact package Ref

`consumer_ref` обозначает установленный sealed package, а не alias, authoring version или workflow display name. Его manifest и объявленный dependency closure задают верхнюю границу решения. В request владелец перечисляет schema refs и выбранные supplier package refs. Каждый supplier непосредственно объявляет нужную schema и находится в этом closure.

Prepare сам находит все revoked packages, которые когда-либо объявляли требуемые refs. Для каждого совпадения он сохраняет исходный status/reason и полный Ref. Первоначальный `Trust.Reason` показывается отдельно от `StatusReason`. Ни один revoked источник не исчезает из диагностики из-за наличия другого supplier.

Request без полного выбора разрешён для диагностики. План перечисляет кандидатов и получает `applicable=false`. Apply такого плана отказывает. Выбор вне closure, неоднозначная identity, несовпадающие bytes или executable component не создают разрешение.

Scope применимого плана покрывает весь набор заблокированных schemas consumer closure. Неполный выбор не создаёт частичного исключения. Не затронутые отзывом компоненты продолжают обычные trust checks. Для первого среза не вводится wildcard или разрешение всем будущим consumer revisions.

Альтернатива одного флага `--force` отвергнута: он не показывает проверенные refs и не закрепляет поставщиков. Бессмысленная смена schema version также не является миграцией.

### 3. Package authority получает `/2` только при первом apply

Хранить decisions в новом `authority-packages/2` вместе с lifecycle generations пакетов. Это оставляет изменение scope и его audit receipt одной атомарной authority командой. Отдельный authority record потребовал бы согласованного изменения нескольких records для generations и решения.

При первом apply `/1` переводится в `/2` в той же транзакции. Все существующие package refs, status, reasons, observations и trust decisions сохраняются. Старые entries получают начальную generation, обозначающую именно сохранённое состояние; старое событие не выдумывается. Решение сохраняет fingerprint этого состояния.

После перехода каждая новая принятая lifecycle команда увеличивает generation конкретного package. Повторный revoke увеличивает её даже при прежнем status. Повтор той же command identity возвращает receipt и не увеличивает generation. Exact reimport уже trusted bytes не меняет generation или исходное trust decision.

Decision закрепляет generations всех покрытых revoked источников, consumer, выбранных suppliers и их необходимых зависимостей. Новая запись revoked package с тем же schema Ref также делает покрытие неполным. Поэтому новый отзыв нельзя обойти старым decision.

Все package read/write handlers должны поддерживать `/1` и `/2`. Команда не должна случайно сериализовать `/2` обратно как `/1` или потерять decisions. До первого apply обычная authority сохраняет `/1`. Старый binary откажет на `/2`, как уже отказывает на неизвестной версии.

Точный Go/JSON shape создаётся при реализации вместе со словарём и schemas. Decision хранит actor, reason, command identity, review digest, consumer, suppliers, covered revocations и commit provenance. Размер state остаётся ограничен существующими budgets. Переполнение даёт именованный отказ; history не удаляется ради освобождения места.

### 4. Prepare и apply используют один deterministic planner

Planner проверяет manifests, schema bytes, dependency closure, control state и затронутые сохранённые Runs. Scan использует существующие limits и явно сообщает неполный результат. Его нельзя сокращать до первого совпадения либо только количества Runs.

План фиксирует authority identity/epoch, packages version, control version, текущий cut и версии прочитанных Runs. Cut — последовательный номер изменения authority. Review digest связывает все поля, влияющие на решение, включая полный набор coverage и последствия для Runs. Форматирование JSON и порядок неупорядоченного ввода не должны менять смысл digest. Канонический порядок выбирается по полному Ref.

Перед apply runtime заново проверяет bytes и строит факты planner. Ошибка digest данных становится `schema_migration_integrity`; различие state/closure становится `schema_migration_plan_stale`. Неполный scan не допускает apply.

В commit `ApplyAuthority` проверяет package version, `ExpectedCut`, `RunPins` и control guard с действующим `ControlOperationTrust`. Для первого среза глобальный cut является консервативным precondition. Даже независимое изменение authority может потребовать нового prepare. Это приемлемое ограничение, указанное в help; ослаблять свежесть молча нельзя.

Transform сравнивает только снимок и подготовленный payload. Он не читает manifests, files, blobs или процессы. Все вычисления bytes происходят до транзакции. Импортированные bytes снова проходят проверку при дальнейшем использовании; apply не обещает защиту от произвольного внешнего изменения файлов после commit.

Успешный commit переводит package state при необходимости и добавляет ровно одно решение с receipt. Stale, integrity или denied command не меняет разрешения. Store может сохранить отказ в receipt по обычному command protocol; это не частичное применение.

Проверка duplicate command идёт до попытки построить новое решение по текущему состоянию. Retry после потерянного ответа возвращает исходный receipt, даже если authority позже изменилась. Inspector при этом показывает текущую пригодность решения. Другой payload с тем же command id даёт `command_conflict`.

### 5. Новый Run явно закрепляет решение

Предлагаемая публичная поверхность:

```sh
prifly --json package schema-migration prepare --request request.json
prifly --json package schema-migration apply --plan plan.json --expected-review-digest DIGEST --reason TEXT --command-id ID
prifly --json package schema-migration inspect --id ID
prifly run start ... --schema-migration ID
prifly project questionnaire ... --prepare --schema-migration ID
prifly project start ... --schema-migration ID
```

`request.json` и `plan.json` — пользовательские файлы соответствующих новых contracts. `...` обозначает существующие аргументы Start, а не новый синтаксис. Для Project consumer сначала проходит обычные compile и import. Prepare миграции ничего не импортирует. Обычный Project Start может повторно импортировать те же trusted bytes без изменения lifecycle generation.

Start проверяет выбранное решение, точный installed consumer и принадлежность запускаемого workflow этому package. Он вычисляет настоящий workflow closure; binding содержит только применимые одобренные schemas. Если все package-level schemas не нужны данному workflow, лишние refs не расширяют его lock. Непокрытые запреты сохраняют отказ.

Решение и его проверенные suppliers закрепляются в `RunStart` атомарно с обычным admission. Packages control pin защищает от нового lifecycle решения между проверкой и commit. Отказ Start не создаёт Run и не оставляет новый claim активным.

Project questionnaire/summary включает выбор решения в review digest. Автоматический retry Start не подменяет ID. Direct `run start` использует тот же runtime handler. Незавершённая или другая compilation не получает разрешение по одному совпадающему schema Ref.

### 6. Все admissions учитывают binding нового Run

Общий package gate получает контекст проверенного binding. Сначала действуют обычные control/trust ограничения. Для schema Ref исключение допускается только при совпадении decision, consumer, конкретного supplier и всех покрытых revocations. Для остальных component kinds прежняя ветка `PackageRevoked` сохраняется.

Последующие admissions читают закреплённый binding Run и текущий package state. Они проверяют generations, разрешаемость выбранных suppliers и новые revoked совпадения. Замена supplier на удобный альтернативный запрещена. Restart использует сохранённые данные, а не память процесса.

Old Runs без binding используют прежний gate. Их defs, pins, events, cancellation, claims и audit не изменяются. `reuseTrustPin` сохраняет запрет trusted reuse старого revoked источника. При создании связанного Run нельзя копировать binding как право: новый Start заново проверяет явный выбор решения.

### 7. Новые contracts имеют достижимые opt-in версии

Добавить closed schemas для request, plan, result и inspection в принятом runtime/public layout. Идентификаторы семейства: `schema-migration-request/1`, `schema-migration-plan/1`, `schema-migration-result/1` и `schema-migration-inspection/1`. Authority state использует `authority-packages/2`.

Binding требует новой state/read boundary. При реализации выбрать следующую свободную версию после сверки `model.go`, `versions.go`, canonical protocol и active changes. Не занимать номер, прочитанный только из старого отчёта. Это техническая нумерация; scope и обязательность новой boundary уже определены.

Run с binding действительно запечатывается новой версией. Run без binding сохраняет существующую подходящую версию. Отдельные Project summary/start DTO версии нужны только для ответов с выбранной миграцией. Прежние bundles, field names и historical evidence не меняются.

Новая capability появляется в `prifly capabilities` с минимальным Core/contract requirement. `examples/README.md` и соответствующий authoring reference показывают поле read contract и границу Run, а не придумывают YAML-поле миграции. Операционные команды описываются в `examples/operations/` и delivered runner assets.

### 8. Именованные отказы и inspection

Использовать коды `schema_migration_invalid`, `schema_migration_incomplete`, `schema_migration_scope_mismatch`, `schema_migration_plan_stale`, `schema_migration_integrity` и `schema_migration_decision_invalid`. Existing `object_access_denied` и `command_conflict` сохраняют смысл. Коды создаются типизированно; текст reason не является кодом.

Inspect возвращает сохранённое решение и вычисленную текущую пригодность. Он показывает конкретный изменившийся package/generation, непокрытый revoke либо нарушенный digest. Недоступные данные обозначаются как непроверенные; они не становятся разрешением.

## Risks / Trade-offs

- [План создаёт видимость проверки всех схем] → полный bounded scan и `applicable=false` при любом неполном результате.
- [Исключение открывает старый Run] → обязательный новый binding и regression старого Run с тем же schema Ref.
- [Start проходит, следующий шаг блокируется] → общий runtime gate и native сценарий минимум с двумя admissions после Start.
- [Повторный revoke совпал по времени и reason] → отдельная lifecycle generation, а не сравнение timestamp.
- [Другой revoked supplier остался незамеченным] → полный набор revoked совпадений при prepare, Start и дальнейших admissions.
- [Global cut часто устаревает] → честный stale отказ и новый prepare; finer preconditions не входят в первый срез.
- [Старый binary игнорирует новое решение] → явный `/2`, новая Run boundary и обязательный отказ неподдерживающего reader.
- [Decision принят за перенос доверия к старому code] → whitelist только schema kind; все executable checks остаются прежними.
- [Downgrade после `/2`] → не понижать запись и не удалять decisions. Переход требует нового поддерживающего binary; восстановление snapshot не входит в миграцию.

## Migration Plan

1. Добавить native fixtures, которые до реализации подтверждают clean/remove success и revoke refusal с нулём Runs.
2. Реализовать versioned contracts, generations и pure prepare/apply; затем подключить явный Start и повторные admissions.
3. Проверить несколько schemas, старые Runs, повторный revoke, двух revoked suppliers, concurrency, duplicate commands и restart.
4. Обновить словарь, source map, capability index, operation guide, help и embedded runner assets вместе с реализацией.
5. Выполнить focused checks и один итоговый `make ci-check-once CHECK_CONTEXT=issue-21-schema-migration` для фактического implementation snapshot.
6. Выполнить release gates на фиксированном release SHA по правилам репозитория; затем проверить generic fixture на released binary.

При отклонении плана ничего не меняется. После apply история решения сохраняется. Новый revoke или утрата доверия делают решение непригодным. Нельзя откатывать миграцию удалением audit либо возвращением record `/1`.

Source qualification и released generic acceptance являются разными задачами. Приёмка рабочей authority проводится отдельно с её владельцем. Ни generic PASS, ни `openspec validate` не закрывают её автоматически.

## Acceptance Mapping

| Issue | Проверяемый результат | Задачи |
|---|---|---|
| R1 | Все exact refs, lifecycle причины, suppliers и влияющие Runs доступны без SQLite | 1.1, 2.1, 2.2, 4.1 |
| R2 | Отдельный owner consent, неизменность revoke и прежнего audit | 2.3, 2.4, 4.2 |
| R3 | Schema-only scope, old Run refusal, все revoked источники и новый revoke | 3.1–3.4, 4.3 |
| R4 | Atomic stale/integrity, control и Run changes, duplicate commands | 2.4, 4.4, 4.5 |
| R5 | Несколько shared schemas, реальный Start и следующие admissions в той же authority | 1.1, 3.2, 4.1, 6.2 |
| R6 | Clean/remove/quarantine/dependencies, history, restart и legacy contracts | 1.2, 4.3, 4.6, 5.1 |
| R7 | Public CLI/help, released binary evidence, separate private acceptance | 5.2–5.4, 6.1–6.3 |
