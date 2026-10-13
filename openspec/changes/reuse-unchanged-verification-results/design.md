## Context

Мотивация — [proposal.md](proposal.md). Контракты — delta
[quality-and-acceptance](specs/quality-and-acceptance/spec.md) и
[release-distribution](specs/release-distribution/spec.md).

Сегодня `ci-check` выполняет unit/integration, static, vulnerability, schema,
release CI и script guards. GitHub `verify` выполняет этот состав параллельными
jobs, `qualify` отдельно выполняет race/e2e. Release workflow уже ограничен
сборкой, native smoke и публикацией. `tag-release.py` ищет successful runs по
display name на exact SHA, но не проверяет workflow identity и полноту jobs.
`extend.yaml` проекта объявляет `ci-check`, полный e2e, OpenSpec и diff check;
verify bridge исполняет это как единственный состав. Исправление нужно в
проектных standing answers, а не в общем runner или внешнем AIF package.

## Goals / Non-Goals

**Goals:** повторные обращения дают проверяемое решение execute/reuse/wait,
а не ещё один дорогой batch. Raw Makefile gates остаются доступны для
диагностики и принудительного исполнения.

**Non-Goals:** универсальный runtime cache, новый вид Artifact/StepResult,
межмашинное локальное доверие, перенос CI evidence между SHA, объединение
push и PR с разными tested SHA, отмена semantic verify/review, автоматическая
публикация, изменения отложенной Claude UI-приёмки.

## Decisions

### 1. Небольшой helper для локального ci-check

Добавить `scripts/ensure-ci-check.py --go PATH [--force]` и Makefile target
`ci-check-once`; существующий `ci-check` всегда выполняет полный состав.
Первый вызов helper исполняет raw `ci-check`, следующий сравнивает evidence с
текущими входами. Подходящий успех позволяет использовать детерминированную
часть, но `vuln-check` исполняется свежим при каждом повторном обращении:
база уязвимостей меняется независимо от исходников. Его failure аннулирует
общий успех. OpenSpec validation и `git diff --check` остаются дешёвыми
отдельными командами и исполняются каждый раз.

Private evidence и logs находятся в ignored `.cache/verification/` данного
worktree. Это локальная история проверок доверенного разработчика, не подпись
и не release qualification. Не переносить её из соседнего checkout/claim.
Завершённый успешный process записывает команды, exit codes, время, logs с
digest, версию helper/policy и fingerprint входов до/после. Запись атомарна;
per-worktree lock предотвращает два параллельных helper batch. Failure,
interrupt, несовпадение снимков или утрата logs не оставляют пригодный успех.
По умолчанию evidence действует не более 24 часов; `--force` обходит reuse.

Fingerprint учитывает bytes и modes всех tracked рабочих файлов и nonignored
untracked файлов, удаления и symlink targets, repository/worktree identity,
HEAD, точные команды/параметры, Go и прочие реально используемые tools,
platform, значимый environment и контекст разрешений исполнения. Не исключать
OpenSpec, README, fixtures, schemas или scripts как «только документацию».
Полный audit фактических входов `ci-check` определяет необходимые ignored и
external inputs: версии/hash toolchain из `.tools` и внешнего Go тоже входят
в контекст. Build/module cache и собственные logs не входят в source snapshot.
При неизвестном input, environment override или невозможности подтвердить
контекст reuse отключается; helper не угадывает эквивалентность. Caller явно
называет permission context через CHECK_CONTEXT/--context; пустое значение
означает execute-only. Label — cooperative assertion host, не новое разрешение
и не доказательство изоляции. При reuse свежий go mod verify проверяет integrity
module cache; custom compiler flags/external workspace или GOFLAGS отключают reuse. В evidence
не писать значения secrets из environment.

Перед commit индекс должен соответствовать проверенным bytes публикуемых
файлов; результат рабочего дерева не доказывает иной staged tree. Изменение
только staging при тех же bytes не требует ещё одного batch. После commit
смена HEAD консервативно инвалидирует локальное evidence. Это намеренная
граница первой версии, а не сравнение только `.go` файлов.

Альтернатива «помнить зелёный вывод в чате» теряет snapshot и toolchain.
Обычный Go build/test cache не доказывает завершение всего состава.

### 2. Проект использует helper вместо повторного полного состава

В текущем `.prifly/workflows/aif-classic/extend.yaml` standing `gate_checks`
становится `make ci-check-once GO=...`, OpenSpec validation и diff check.
Убрать полный `make e2e` из обычного состава; explicit scenario tests из плана
остаются обязательными. Обновить AGENTS.md, README, Codex/Claude PROJECT.md:
implement выполняет затронутые проверки; итоговый снимок получает полный
локальный gate перед commit, verify/commit используют helper. Изменение при
repair требует новых checks. Проверка полноты плана и review продолжаются.

Не редактировать installed upstream bridge: это drift импортированного
package. Новые instructions не подменяют его routing, gate artifact и
accepted StepResult. Standing answers действуют для новых Runs; уже sealed
Runs и packages не переписываются. Чужие workflows задают свой состав.

### 3. Одна процедура получения CI gates на exact SHA

Добавить `scripts/ensure-release-checks.py --sha SHA --ref REF [--wait]
[--force]`. SHA обязателен и фиксируется до любых запросов; helper не создаёт
tag, не push-ит и не approves environment. Без `--wait` возвращает текущее
состояние и URLs, с ним наблюдает bounded интервал; timeout остаётся pending.
Polling не держит клиент без возможности показать прогресс дольше 60 секунд.

Общий selector с `tag-release.py` читает все страницы GitHub runs/jobs,
проверяет repository, workflow ID/path, exact `head_sha`, latest attempt и
полноту обязательных jobs. Success другого workflow с тем же display name,
local evidence, skipped jobs и результат другого SHA не подходят. PR merge
SHA — отдельный subject. Поздний реальный failed/cancelled batch не скрывать
более ранним успехом; явный retry должен завершиться успешно. Шардирование
`verify` и полный race/e2e состав остаются обязательными.

Для каждого workflow: пригодный success → reuse с URL; queued/in-progress →
wait; отсутствующий → один dispatch; failure → остановка с причиной и
командой явного retry. `--force` означает намеренный новый batch; активный
batch сначала должен завершиться. При недоступном API нельзя заявить pass.
Ref проверяется против SHA до dispatch. Dispatch передаёт optional
`expected_sha`, а workflow проверяет его до тяжёлых jobs; движение branch
между запросами не позволяет silently проверить другой candidate.

### 4. Конкурентный dispatch не запускает два дорогих batch

В `verify` и `qualify` добавить serialization по repository/workflow и
фактически tested SHA с `cancel-in-progress: false`, затем дешёвый preflight,
который повторно читает результаты. Первый batch выполняет текущие jobs;
повторный после него может пропустить их только при проверенном полном
успешном source run на том же SHA. Вывод preflight и job summary называют
исходный run. Не строить цепочки reuse: источник — непосредственно
исполненный полный batch. Selector tag проверяет этот источник, а не зелёную
оболочку, где jobs skipped. Force явно включает полное исполнение.

Preflight исключает собственный run и pending дубликаты из поиска source
evidence: после serialization он оценивает завершённые исходные batch, а не
ждёт самого себя. Ошибка API отключает reuse; обычное исполнение остаётся
возможным, но tag без проверяемого полного CI evidence запрещён.

Serialization сама по себе не доказывает успех и не устраняет последовательный
повтор. Текущие [GitHub concurrency rules](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency)
допускают замену pending run; cancelled очередь не становится gate evidence.
Не группировать разные commits по branch: это отменило бы нужную candidate
проверку. У [pull_request](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request)
tested SHA обычно относится к merge commit, поэтому push/PR не считаются
одинаковыми по одному head branch ID.

В первой версии CI reuse также ограничен 24 часами и текущей policy.
Изменённый workflow/toolchain contract, известное изменение внешнего
контекста, expiry либо force требуют свежего batch. `tag-release.py` проверяет
те же условия непосредственно перед tag; проверка состояния helper не выдаёт
долгоживущего разрешения. Raw GitHub rerun остаётся явным операторским действием.

### 5. Qualification и публикация сохраняют прежние границы

Локальное evidence никогда не заменяет CI. Каждый новый release SHA получает
полный verify и race/e2e qualify. Release workflow не принимает local receipt,
не повторяет эти suites и сохраняет native build/smoke, signed manifests,
protected tag, job-scoped credentials и approval environment. CLI/schema
версии Pri-Fly не меняются; helper interfaces — tooling репозитория.

## Risks / Trade-offs

- Неполный fingerprint → audit входов и негативные cases до подключения helper; неизвестное означает execute, не reuse.
- Переход между sandbox/context → учитывать контекст; не использовать failure либо результат с неподтверждённой совместимостью.
- Полный snapshot инвалидируется при изменении документа → намеренный консервативный выбор; selective/cross-commit reuse вне scope.
- Зелёный workflow с пропущенными jobs → проверять непосредственно исполненный source batch, negative API fixtures и реальную CI приёмку.
- Fork/PR permissions и synthetic merge → read-only API permission, отдельная SHA identity; невозможность reuse оставляет обычное исполнение, а не bypass.
- Новая CVE после старой проверки → свежий local vuln-check, ограниченная актуальность CI и явный force; известный отозванный результат не подходит.
- Проверка режима запуска CI меняет свой состав → release CI guard продолжает доказывать покрытие всех обязательных targets, включая путь без reuse.

## Migration Plan

Сначала helpers и negative tests, затем Makefile/standing answers и CI guard.
Первый вызов без evidence выполняет проверки полностью. Для реальной CI
приёмки push создаёт единственный полный verify; qualify запускается один раз
на зафиксированном SHA, повторные запросы проверяют reuse без повторных suites.
Если candidate меняется, новый SHA снова получает полные ворота. Результаты
не объявляют новый release либо закрытие roadmap/Claude acceptance.

Rollback возвращает standing composition к raw `ci-check` и отключает CI
preflight reuse, сохраняя полный состав и exact-SHA tag gate. Private cache
можно удалить без миграции Authority или historical records.
