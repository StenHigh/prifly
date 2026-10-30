## Context

Мотивация — [proposal.md](proposal.md). Baseline — Pri-Fly `6de187d22435447c464b9b41269cd8b2aec50d2e`, Go 1.27.3, macOS arm64.
[Evidence](../improve-workflow-authoring-readability/evidence/worktree-gate-failure.json) получено до создания этого плана.

| Проверенный факт | Наблюдение |
|---|---|
| Полные staged ворота | Exit 2; падает `TestNamedWorktreeConcurrentCLIProcessesFromPublishedExample` |
| Чистый baseline | Тот же тест: 2/10 падений |
| Readability claim | Тот же тест: 1/10 падений |
| Видимый отказ | `invalid_input`; cleanup сообщает отсутствие Run admission и release своего claim |
| Точный источник исходной ошибки | materializePackage: rename staging → final получает EEXIST после публикации другим процессом |

Тест запускает два отдельных CLI-процесса с разными command IDs и одной основой имени.
Затем он отдельно проверяет concurrent duplicate command ID.
Исходный сбой возникает в первой паре, на проверке ответов около `project_worktree_naming_test.go:179`.

Область поиска ограничена существующим start protocol:

```text
reviewed inputs → claim reservation → worktree creation → package import/refresh
                                                            ↓
                                                   Engine.Start → Run → first drive
                                                            ↓ ошибка
                                                   reconcile → fenced cleanup
```

`project_start.go` передаёт все ошибки после claim через `projectStartCleanup`.
Поэтому видимый cleanup suffix не доказывает, какая предыдущая операция отказала.
`invalid_input` также не доказывает ошибку пользовательского task JSON.
Нельзя выбрать исправление только по этому публичному коду.

`Engine.Start` уже имеет восемь bounded повторов для `ErrAdmissionSnapshotStale` у named claims.
Отдельный CLI helper повторяет некоторые typed conflicts первого drive после создания Run.
Наблюдаемый сбой происходит до admission, поэтому retry первого drive его не объясняет.

## Goals / Non-Goals

**Goals:**

- Найти исходную ошибку и конкретное пересечение операций до изменения production code.
- Восстановить два допустимых независимых запуска с разными claim/path/branch/Run.
- Сохранить один claim и Run для duplicate semantic command.
- Сохранить current authorization, generation fences, capacity и безопасный cleanup.
- Доказать исправление детерминированной регрессией и публичным multi-process тестом.

**Non-Goals:**

- Менять naming policy, suffix sequence, CLI flags, published schemas или saved Run contracts.
- Последовательно исполнять все project launches через новый repository-wide lock.
- Увеличивать retry counts, добавлять sleeps либо повторять весь `project start` до успеха.
- Повторять любой `invalid_input`, storage error или неизвестный effect как заведомо безопасный конфликт.
- Исправлять AI Factory `fail/no_work` routing или расширять readability candidate.
- Вводить новый lifecycle, executor, внешний coordinator или dependency.

## Decisions

### 1. Сначала воспроизводимый RED и исходная причина

Повторить существующий тест на pinned baseline с теми же Go, TMPDIR и GOMAXPROCS.
Для диагностики сохранять исходную cause на границе helper/engine, до преобразования в Problem envelope.
Не расширять публичный JSON ради временной диагностики.

Сопоставить две команды по фазам: claim, Git worktree, package import/refresh, Start и cleanup.
Проверить свежесть состояния package в каждом Engine и admission observations.
Это направления проверки, не установленные причины.

После локализации задать нужное пересечение операций test barrier либо существующим test seam.
Test barrier — управляемая остановка теста перед выбранной операцией, которая позволяет повторить нужный порядок.
Новый публичный hook или флаг для этого не нужен.
Регрессия должна падать до исправления на конкретном нарушенном invariant.
Простой `-count=100` дополняет регрессию, но не заменяет её.

### 2. Исправить общий участок один раз

Реализован `publishPackageDirectory`: только коллизия существующего каталога
разрешает повторную проверку manifest и всех payload files через существующий sealPackagePayload.
Совпадение продолжает обычный trust; конфликтующие bytes и прочие filesystem errors сохраняют отказ.
Публикация остаётся до write-транзакции. Новая общая блокировка и retry loop не нужны.
Регрессия начинает проверку непосредственно в точке публикации после выигрыша другого процесса.


Минимальная правка должна находиться там, где возникает неверное наблюдение или нарушается command ownership.
До этой правки найти все callers и проверить direct start, duplicate start и cleanup.

| Участок | Почему нужно проверить |
|---|---|
| `cmd/prifly/project_start.go` | Порядок claim/import/refresh/Start и передача исходной ошибки |
| `cmd/prifly/project_worktree_naming.go` | Существующая классификация повторяемых conflicts |
| `cmd/prifly/project_cleanup.go` | Run reconciliation и ownership package/claim при rollback |
| `internal/runtime/start.go` | Bounded повтор только подтверждённого stale observation |
| `internal/runtime/packages.go` | Process-local package state после concurrent import |
| `internal/local/admission_observation*` | Сравнение observation без filesystem effects внутри transaction |

Это список мест для чтения, а не обязательный список правок.
Предпочтительны существующие package refresh, typed stale marker и command receipts.
Blanket retry скрывает постоянные ошибки и может повторить effects; он не принимается.
Новая общая блокировка снижает concurrency и меняет обещанную независимость worktree; она не принимается.

Если безопасный retry действительно нужен, он сохраняет command ID, reviewed values и claim identity.
Он перечитывает необходимые observations, учитывает context cancellation и имеет конечный предел.
Только типизированная доказанная причина разрешает retry.
Исчерпание и реальные отказы сохраняют предметный отказ, а не становятся успехом.

### 3. Cleanup проверяется вместе со start

При отказе сначала определить durable outcome команды через существующий reconciliation.
Созданный Run сохраняет свой claim/package. Неизвестный outcome сохраняет evidence.
Подтверждённое отсутствие Run допускает cleanup только собственного нового claim с правильной generation.
Нельзя снять package trust или удалить дерево, которое уже использует другой успешный запуск.

Проверить сохранённые refs и bytes второго Run до и после cleanup первого.
Отдельно проверить duplicate command: повтор не расходует следующий suffix и не создаёт второй Run.
Payload conflict не превращается в duplicate success.

### 4. Приёмка проверяет результат, а не удачный повтор

| Проверка | Критерий |
|---|---|
| Управляемая регрессия | RED до fix; GREEN после; no sleeps |
| Разные command IDs, два процесса | Два Run, разные фактические claims/paths/refs; оба доходят до первого handoff |
| Один duplicate command ID | Один Run/claim; ответы согласованы; нет дополнительного suffix |
| Отказ одного запуска | Другой Run, его package, дерево и claim остаются пригодными |
| Review digest | Разрешён следующий suffix; изменение reviewed declaration по-прежнему отвергается |
| Стабильность public теста | Существующий multi-process тест проходит фиксированные 50 повторов без dropped failures |
| Итоговые ворота | `make test-changed`, OpenSpec strict validation, diff check и один staged `ci-check-once` |

50 повторов — дополнительная стресс-проверка, а не доказательство отсутствия любой гонки.
Linux повторит публичную регрессию в CI; локальный macOS результат не выдаётся за Linux evidence.
Новые tests получают `t.Parallel()`, если их окружение и hooks позволяют это по правилам `AGENTS.md`.
Все созданные тестом Runs, claims и monitor registrations очищает сам fixture.

## Risks / Trade-offs

- [Причина не в observation] → сначала установить cause; не внедрять выбранный заранее retry.
- [Диагностика меняет timing] → после локализации использовать test barrier и сохранить отдельный multi-process test.
- [Retry ломает duplicate identity] → проверить stable semantic payload, command receipt и отсутствие нового suffix.
- [Rollback затрагивает другого caller] → проверить второго владельца после ошибки первого; сохранить unknown outcome.
- [Нужен новый контракт] → остановить реализацию и пересогласовать scope; `skip_specs` тогда пересматривается.
- [Случайно зелёный стресс] → требовать детерминированную RED/GREEN пару и хранить все неудачные batches.

## Migration Plan

1. Подтвердить причину и подготовить failing regression.
2. Применить минимальное исправление в общем участке.
3. Проверить concurrency, duplicate identity, cleanup и существующие refusals.
4. Выполнить staged ворота и сохранить exact evidence.
5. Вернуться к readability Run; человеческая приёмка остаётся отдельной.

Миграция состояния и version bump контракта не планируются.
Rollback возвращает code patch, не переписывая уже созданные Runs или claims.
План реализации сохранён вместе с [фактическими результатами](acceptance.md).
Production package fix и регрессия выполнены. Последующее исправление Git registry описано ниже; пункт 3.1 закрыт после 50/50.


## Follow-up: Git worktree registry

После package fix отдельная прямая Git-проба воспроизвела отказ чтения `commondir`.
Один `worktree add` читает запись другого до завершения её заполнения.
[Evidence](evidence/registry-race.json) хранит stderr, версии и границу вывода: исходный публичный отказ не содержал Git stderr.

Общий `Engine.git` держит OS flock на `<git-common-dir>/prifly-worktrees.lock` только во время одной команды `git worktree`.
Все существующие named и legacy add/remove/prune проходят через этот участок.
Другие Git-команды и выполнение workflow остаются параллельными.
Это блокировка операции регистрации, а не repository-wide exclusion claims или всего Run.

Ожидание учитывает context cancellation и прежний общий deadline `gitTimeout`.
Неправильный тип lock-файла и symlink получают отказ. Файл не удаляется после команды.
Lock снимается закрытием descriptor при успехе и ошибке Git.
Он координирует Pri-Fly callers разных authorities, использующих один Git common directory.
Внешний `git worktree` этот lock не соблюдает. Checkout входит в защищённый subprocess.

FIFO-barrier останавливает тестовый Git внутри команды; отдельный descriptor проверяет удержание lock.
Это управляемый RED/GREEN без ожидания случайного interleaving.
Дополнительные тесты проверяют общий каталог linked worktree, независимость репозиториев, cancellation и небезопасные файлы.
[Публичный неизменённый тест](evidence/concurrency-registry-green.json) прошёл 50/50 на окончательном варианте.
Предыдущие отрицательные batches сохранены.
