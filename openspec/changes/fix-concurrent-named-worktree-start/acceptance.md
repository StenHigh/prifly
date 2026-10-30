# Диагностика concurrent start и настройка имён

Причина исходного `invalid_input` установлена: два процесса сталкиваются при публикации одного package directory.
Исправление публикации пакета реализовано. Целевая регрессия и полный staged gate прошли. Отдельный отказ подготовки worktree исправлен последующим patch, описанным ниже.

## Читаемые имена проекта

По отдельному запросу владельца изменён только `.prifly/project.yaml` текущего claim worktree:

```yaml
schema_version: prifly-project-profile/4
title: Pri-Fly
worktree_naming:
  template: "{project_name}-{workflow}"
  bindings:
    project_name:
      value: prifly
    workflow:
      value: aif-classic
```

Фактический prepare preview:

```text
каталог: .prifly/work/claims/prifly-aif-classic
ветка:   prifly/prifly-aif-classic
policy:  first-free/1
reserved: false
```

При занятом имени действуют `-02`, `-03` и следующие suffix по существующей policy.
Заголовок Run по-прежнему читается из `task.title`; служебный Run ID сохраняется.
Нынешний Run и его claim не переименовывались.

Использована постоянная основа workflow: она подходит существующему `aif-classic` без изменения его input schema.
Это имя проекта и workflow, а не индивидуальный ключ задачи.
Внешний package и сохранённый `extend.yaml` не изменены.

`project compile` и `project questionnaire --prepare` прошли на stable Pri-Fly 0.13.81.
Для prepare создана временная authority вне claim. После проверки её monitor registration удалена.
Новые Runs и claims не создавались. Временный ignored `local.yaml` удалён.
[Evidence настройки](evidence/project-naming.json) содержит preview и hash профиля.
Настройка пока находится в claim branch; основной checkout `main` не изменён.

## Исходная ошибка теста

Baseline: `6de187d22435447c464b9b41269cd8b2aec50d2e`, Go 1.27.3, macOS arm64.
Диагностика выполнена на отдельной копии `git archive`, без production edits в Pri-Fly.
Перед `makeProblem` в копии добавлен вывод исходной cause только для существующего named CLI helper.
После опыта эта временная правка удалена; исходный файл побайтно восстановлен.

Команда использовала private `/tmp`, `GOMAXPROCS=2`, `GOTOOLCHAIN=local` и существующие Go caches:

```sh
go test ./cmd/prifly \
  -run '^TestNamedWorktreeConcurrentCLIProcessesFromPublishedExample$' -count=20
```

Batch завершился с exit 1: два падения из 20 повторов.
Оба падения показывают `*os.LinkError` при одной операции:

```text
rename <package>.pending-package:<own-id> <package>: file exists
```

[Полное evidence](evidence/root-cause.json) содержит обе исходные ошибки, hashes и описание диагностической вставки.
Полный лог сохранён в `.cache/authoring-readability/naming-raw-cause.log`.
Это наблюдение для поиска причины, а не измерение частоты дефекта в production.

## Место и механизм

`ImportPackage` вызывает `materializePackage` до регистрации `package.trust` и создания Run.
В `internal/runtime/packages.go:586` функция проверяет существующий manifest и сравнивает его digest.
Если каталога ещё нет, она пишет свой staging directory.
В строке 622 она вызывает `os.Rename(staging, final)`.
После коллизии эта ветка сразу возвращает error; повторной проверки уже опубликованного пакета нет.

```text
Процесс A                         Процесс B
manifest отсутствует              manifest отсутствует
пишет staging A                   пишет staging B
rename A → final: успех
                                  rename B → final: file exists
                                  cleanup своего claim
                                  CLI Problem: invalid_input
```

Оба процесса используют одну package identity, поэтому их `final` одинаков.
Разные имена worktree не разделяют этот каталог пакета.
Внешний Docker-инструмент ещё не вызван и не участвует в этой гонке.
`makeProblem` сохраняет cleanup context, но обычная файловая ошибка получает общий код `invalid_input`.

## Граница исправления, установленная при диагностике

Исправлять нужно совместную публикацию одинакового пакета, сохраняя проверку identity и файлов.
Постоянные filesystem errors и конфликтующие bytes не должны превращаться в успех.
Следующая задача — управляемая RED-регрессия для пересечения initial read и rename.
Увеличение retry counts или изменение naming policy для этого не требуется.

На этом диагностическом снимке был завершён только пункт 1.1.
Итоговое выполнение описано ниже; первоначальный отрицательный результат сохраняется.
Человеческая приёмка readability остаётся отложенной.


## Выполнение: исправление публикации

`materializePackage` передаёт финальную публикацию private helper `publishPackageDirectory`.
При `EEXIST`/`ENOTEMPTY` helper читает manifest победившего процесса и проверяет все его payload files.
Точное совпадение позволяет продолжить исходную процедуру trust. Остальные ошибки сохраняют отказ.
Новые locks, retry loops, schema/state versions и dependencies не добавлены.
Own staging cleanup остаётся в прежнем defer вызывающей функции.

`TestPackageDirectoryPublicationCollision` начинает выполнение после публикации победителя.
Этот порядок задан fixture, без sleeps или глобального hook.
Перед исправлением идентичная публикация дала ожидаемый FAIL после поведения-сохраняющего выделения helper.
После исправления прошли пять случаев: identical, manifest conflict, payload drift, undeclared file и missing staging.
В каждом случае bytes победителя остаются неизменными.
[RED evidence](evidence/publication-red.json), [GREEN evidence](evidence/publication-green.json).

Тесты повторного trust, identity conflict, undeclared content и byte drift тоже прошли.
Существующий cleanup test прошёл для committed, unknown и failed cleanup outcomes.
`make test-changed` прошёл: все Go packages, поскольку меняется project profile.

Первая stress-проверка из 50 повторов нашла один другой отказ `claim_preparation_incomplete`.
Она не считается успешной; [batch evidence](evidence/concurrency-first-batch.json) сохраняет failure.
Второй отказ исследуется отдельно; его нельзя скрывать повтором до случайного зелёного результата.


## Итоговые проверки

- `TestPackageDirectoryPublicationCollision` — PASS по пяти случаям после ожидаемого RED.
- Повторный trust, identity conflicts, sealed bytes и undeclared content — PASS.
- `TestProjectStartCleanupPreservesOutcomeAndOriginalProblem` — PASS по девяти сценариям.
- `make test-changed` — PASS, все Go packages.
- `make ci-check-once GO=/opt/homebrew/opt/go/libexec/bin/go CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1` — PASS.

Полный gate включил Go tests, vet Linux/Darwin, gofmt, typed-refusal guard,
staticcheck, govulncheck, schemas, release contract checks, scripts и shard completeness.
Проверка модулей прошла. [Evidence первого полного batch](evidence/full-gate-first.json).
Последующее обновление journal проходит повторный staged batch; лог — `.cache/authoring-readability/full-fixed-final.log`.

Protected historical evidence, archives, schemas, flow contracts и installed workflow files не менялись.
Профиль `.prifly/project.yaml` совпадает с ранее проверенным отдельным поручением владельца по именам.
Первый production fix ограничивался `internal/runtime/packages.go`; регрессия — `internal/runtime/package_publication_test.go`.
Новые capabilities отсутствуют, поэтому authoring index и source ownership map не менялись.

## Первоначально открытая проверка стабильности worktree

Первый обычный batch из 50 повторов имел один `claim_preparation_incomplete`.
Это иной отказ до публикации пакета. Он не объявлен исправленным этим patch.
Две дополнительные диагностические копии по 100 повторов прошли с Git stderr tracing.
[Evidence](evidence/git-followup.json) сохраняет оба положительных batch и исходный отрицательный.
На том этапе production worktree code не изменялся; временная instrumentation полностью удалена.

На предыдущем снимке пункт 3.1 оставался открытым. Из 9 пунктов были выполнены 8.
Детерминированная регрессия доказывает исправление package publication, но не устраняет неизвестный Git failure.
Полный release race/e2e и Linux runtime CI не запускались; local vet прочитал обе целевые платформы.

Readability получил результат полных ворот. Его человеческая приёмка и принятие upstream handoff остаются открытыми.
Текущий implement Attempt не получил `pass`. Checkpoint commit и перенос в main пока не выполнены.


## Исправление оставшегося отказа worktree

Прямая проба двух Git-процессов воспроизвела ошибку на итерации 167 из запланированных 500.
Git 2.53.0 отказал при чтении `commondir` соседней незавершённой регистрации:

```text
fatal: failed to read .git/worktrees/smsplace-trucker-default-123/commondir: Undefined error: 0
```

Проба не использовала Pri-Fly, SQLite, package import или Docker.
Порядок записи подтверждён [исходником Git](https://github.com/git/git/blob/v2.53.0/builtin/worktree.c#L414-L476).
Исходный публичный отказ не сохранил stderr; прямая проба установила механизм в этой фазе, а не восстановила прежний stderr.
[Evidence](evidence/registry-race.json) сохраняет эту границу вывода.

`internal/runtime/worktrees.go` теперь защищает одну команду `git worktree` через OS flock в общем Git-каталоге.
Все named/legacy add/remove/prune используют общий вызов. Claims и готовые worktree остаются независимыми.
Lock держится до конца subprocess, включая checkout, и учитывает cancellation/deadline.
Внешние Git-процессы этот частный lock не соблюдают.

`TestGitWorktreeRegistryLockCoversCommand` получил ожидаемый RED до исправления и GREEN после него.
FIFO-barrier подтверждает нахождение дочернего процесса внутри команды без sleeps для задания порядка.
Проверены освобождение lock после успеха/ошибки, linked worktree, независимые репозитории, cancellation, symlink и FIFO.
Тест с заменой PATH намеренно последовательный; остальные новые тесты используют `t.Parallel()`.

Окончательный вариант прошёл неизменённый публичный CLI-тест: **50/50**, exit 0.
Предшествующий вариант защиты тоже прошёл 50/50; [оба batch сохранены](evidence/concurrency-registry-green.json).
Первоначальный отрицательный batch остаётся отдельным evidence. Пункт **3.1 закрыт**.
Сценарий включён в Linux `verify/packages` через `make test-rest`; удалённый CI в этой сессии не запускался.

Полный свежий staged batch после Git fix прошёл с exit 0. [Evidence](evidence/full-gate-registry.json).
Ворота включили Go tests, vet Linux/Darwin, gofmt, refusals, staticcheck, govulncheck,
schemas, release contracts, scripts, проверку 746 runtime tests в shards и `go mod verify`.
`make test-changed` тоже прошёл. OpenSpec strict: 38/38. Пункт 4.2 закрыт; итог change — **9/9**.
После обновления этого журнала и HTML выполняется заключительная staged проверка тех же изменений.
Её receipt и log хранятся в `.cache/verification/` и `.cache/authoring-readability/full-registry-final.log`.

## Возврат к readability

На исправленном исходном коде собран отдельный CLI в cache, без установки в систему.
Он подтвердил 12 пар сборок и восемь пар маршрутов aif-classic baseline/candidate.
Все семь файлов кандидата совпадают с прежним handoff manifest.
[Evidence](../improve-workflow-authoring-readability/evidence/current-runtime.json).
Человеческая приёмка остаётся pending. Текущий implement Attempt не получил pass.


## Последующее принятие readability

Владелец ответил «Принимай» и принял три части по review без обязательных замеров.
[Решение](../improve-workflow-authoring-readability/evidence/owner-review.json) снимает человеческий блокер основного change.
Прежние записи pending выше описывают предыдущие снимки. Ручной reader pilot не объявлен выполненным.
