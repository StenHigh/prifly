# Повторный resume блокирует продолжение SMSPlace #163

Дата: 2026-10-02. Отчёт создан по запросу владельца проекта SMSPlace для разработчика Pri-Fly.

## Краткий результат

Первое возобновление `aif-classic` успешно перенесло принятые этапы и рабочее дерево. Возобновлённый Run выполнил verify → fix → verify и штатно завершился `partial`, ожидая разрешения владельца на небольшую правку вне исходной задачи. Разрешение получено, но второе возобновление отказывается с `resume_prefix_empty`.

Вернуться к первоначальному Run тоже нельзя: `recover_workspace_missing`. В результате работа сохранена, но штатный путь к следующим этапам workflow недоступен. Нужно продолжить последнего владельца дерева, не повторяя планирование и реализацию и не теряя незакоммиченные изменения.

Это не сообщение о потере файлов и не доказанная неисправность SQLite. Ни ручная правка authority, ни raw `run fork`, ни новый Run для обхода отказа не выполнялись.

## Окружение и идентификаторы

- Фактически исполняемый бинарь: `/Users/sh/.local/bin/prifly`.
- Версия, повторно проверенная при подготовке отчёта: `prifly 0.13.74`.
- Host: `codex-app`.
- Launch: `aif-classic`, workflow ID `aif:workflow/classic`.
- Исходный пакет в запуске: classic 1.47.0; текущий questionnaire также сообщает 1.47.0.
- Authority: `/Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority`.
- Первичный checkout: `/Users/sh/PhpstormProjects/SMSPlace/backend`.
- Задача: https://gitlab.com/smsplace/backend/-/work_items/163, дочерняя к #155.
- Run A: `run:9b2a1ba7bd33f72246e446d7bff7daf7b5d0dc5eb5c0f790617f3111f519f6da`.
- Run B, успешный первый resume A: `run:80e270db507eab9ba8faad23205852d2be95ade93e16f41be94819c6e6c20d1e`.
- Сохранённое дерево: `/Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority/.prifly/work/claims/df98f72e4297c34fa926562ddc831d998e8bb4de58c2234b37e18747e961a62e`.
- Ветка дерева: `prifly/df98f72e4297c34fa926562ddc831d998e8bb4de58c2234b37e18747e961a62e`.
- База: `d8f872e8cd0020d8b449cb3ef7b658b76406210a`.
- HEAD дерева: `793df111e028a59e52563be58fd447d2d7d56f93`.

Исходники Pri-Fly, прочитанные только для локализации: этот репозиторий, HEAD `ac8d42b5847bbd437e6d9e6f438617ecd3cdba5b`. Их соответствие установленному бинарю не проверено; выводы по исходникам ниже являются гипотезой, а не установленной причиной поведения бинаря.

Важная неоднозначность метаданных SMSPlace: `.prifly/project.yaml` указывает origin ref `v1.50.0`, тогда как questionnaire сообщает package 1.47.0. Этот отчёт не устанавливает причину расхождения и не утверждает, что оно вызвало отказ. При диагностике ориентироваться на запечатанные контракты Runs и prepare, не обновлять workflow автоматически.

## Хронология и причины остановок

### 1. Run A: реализация готова, verify не смог поднять нужное окружение

Приняты warmup, plan, improve, implement. Реализация QA pricing lab находится в двух коммитах:

- `3e0a8b6f3` — реализация двух рынков для fallback anchor;
- `793df111e` — уточнение bounded pricing browser replay.

Браузерные QA-сценарии выполнены в приватном окружении: два сценария прошли, fixture verification — без расхождений. После этого verify потребовал профиль PHP вместо прежнего browser-профиля. `worktree-env ensure` отказал с `release_required`, поскольку изменились dependency fingerprints активного окружения. Гейт завершился кодом 2 до проверок, не кодом 1 с дефектами кода.

Run A штатно завершился `partial`; verify attempt имел verdict `blocked`. Владелец затем разрешил пересоздать только приватное окружение. Старое окружение освобождено, новое PHP-окружение создано; исходники и отчёты сохранены.

Снимок A по `run status`: `status=completed outcome=partial version=61`, `driver_live=false`, `unresolved=false`, pending decision отсутствует. `run next` указывает terminal, `from_stage_id=verify` и допускает `project.continue`.

### 2. Run B: первый resume сработал

Через `project continue --launch aif-classic --source-run A --host codex-app` создан B. Prepare перенёс принятые warmup/plan/improve/implement и то же рабочее дерево; старт возобновил verify.

Первый verify B обнаружил:

1. Требуемые механические изменения Rector.
2. Два новых QA case ID отсутствовали в учитываемой части QA registry.
3. Существующий в базе тест comment hygiene выявил ссылки на задачи/MR в двух других тестах.

Гейт передал исправимые findings в штатный fix. Fix был принят и исправил первые два пункта: форматирование, runtime postconditions registry и соответствующие проверки. Код этого fix сохранён незакоммиченным в том же дереве — это существенно для повторного resume.

Повторный verify B:

- Pint, Rector, PHPStan — зелёные;
- pins — 809 тестов, 27112 assertions, зелёные;
- admin — 242 теста, 14846 assertions, одно падение comment hygiene;
- QA registry audits — зелёные.

Единственный оставшийся блокер был вне первоначального scope #163. На момент отчёта verify разрешение владельца ещё не пришло. Gate artifact правильно сохранил `blocking=true`, `blocking_owner_only=true`, а workflow завершился `partial`, вместо самовольной правки baseline.

Последний gate artifact B:

- ID: `artifact:83b85877f91f8f4678c5ebc0bc3eeb1f46969efb473362f9a1f04beb27066735`;
- revision: 1;
- digest: `sha256:8523bcb5a74b7d961ac672a93311887ba16c338d7f5c67765db0d17b68919d9c`.

Снимок B: `status=completed outcome=partial version=36`, `driver_live=false`, `unresolved=false`, pending decision отсутствует. Три completed steps с verdict pass: verify, fix, verify. Gate failures передаются внутри typed output, поэтому pass шага здесь не означает зелёный гейт. `run next` указывает terminal, `from_stage_id=verify` и допускает `project.continue`.

### 3. Разрешение владельца получено, но resume B недоступен

Владелец разрешил удалить только номера задач/MR в комментариях:

- `tests/Unit/Pricing/PricingModelCorpusManifestTest.php`;
- `tests/Unit/Traceability/Ratchet/RatchetPolicyTest.php`.

Причина: ссылки давали историческую навигацию к решениям, но нарушали source comment hygiene. Даты, пояснения, разделы ТЗ и ID требований должны остаться. Assertions, хэши, frozen debt и known-red не должны меняться.

Эти две правки ещё НЕ выполнены: host хотел получить штатный шаг с workspace-write effect. Повторный resume отказывается до выдачи Attempt. Завершение проверок, security/review, tests, commit и MR ещё впереди; MR не создан.

## Точное воспроизведение на существующей authority

Все команды ниже диагностические. `--prepare` не создаёт Run и не выполняет шаги. Не заменять prepare стартом, пока план восстановления не проверен.

### Основной отказ: resume B

```sh
/Users/sh/.local/bin/prifly \
  --project /Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority \
  project continue --launch aif-classic \
  --source-run run:80e270db507eab9ba8faad23205852d2be95ade93e16f41be94819c6e6c20d1e \
  --host codex-app --prepare
```

Повторно воспроизведено при подготовке отчёта, exit 3:

```json
{
  "schema_version": "1",
  "code": "resume_prefix_empty",
  "message": "nothing was accepted before verify, so there is nothing to carry; start the workflow anew",
  "retryable": false,
  "correlation_id": "correlation:ba1fb862c085ad69cc1c7e4f9a09c832",
  "violations": [],
  "safe_next_actions": ["doctor", "run.status"]
}
```

### Попытка возобновить A

Та же команда, но `--source-run run:9b2a1ba7bd33f72246e446d7bff7daf7b5d0dc5eb5c0f790617f3111f519f6da`:

```json
{
  "schema_version": "1",
  "code": "recover_workspace_missing",
  "message": "the source Run holds no tree, and this workflow needs one to run again",
  "retryable": false,
  "correlation_id": "correlation:27a63f23165f53385aeaa8844be18747",
  "violations": [],
  "safe_next_actions": ["doctor", "run.status"]
}
```

Exit 2. Это согласуется с передачей дерева из A в B, а не с удалением самого дерева.

### Попытка выбрать fix через `--from-stage`

Та же команда для B с `--from-stage fix --prepare`:

```json
{
  "schema_version": "1",
  "code": "resume_from_stage_invalid",
  "message": "stage fix is not an accepted stage of the source's root workflow run before verify",
  "retryable": false,
  "correlation_id": "correlation:65d26104ad365ba449142e24abfe431d",
  "violations": [],
  "safe_next_actions": ["doctor", "run.status"]
}
```

Exit 3. Наличие completed fix в B не означает, что он root stage перед frontier: fix выполнялся внутри вложенного quality workflow. Host не пытался обходить ограничение другим именем этапа.

`doctor` выполнен: authority читается, SQLite storage version 9, WAL, foreign keys включены, CLI заявляет поддержку `workflow_resume` и `workflow_continuation`. Это не полноценный аудит целостности всей истории, но отдельного технического отказа doctor не сообщил.

## Возможная причина: требуется подтвердить разработчику

В прочитанном локальном `internal/runtime/recovery.go`:

- `PlanRecovery` загружает source Run, его plan и события, вычисляет point, затем вызывает `recoveryTrace`.
- `recoveryTrace` строит список из `source.Activations`, сортирует по sequence и берёт только activations до cutoff.
- Если список пуст, возвращает `resume_prefix_empty`.
- В `RecoveryState` уже существуют `Reused` и `RootOutputs`; invariant требует непустой `Reused`.

Рабочая гипотеза: B начинает фактическое исполнение с verify, а принятый ранее префикс A хранится как recovery evidence, не как новые activations B. При повторном resume учитываются только activations B до его verify frontier; унаследованный префикс не включается. Тогда фраза «nothing was accepted» верна лишь для локальной части B, но не для полной цепочки выполнения задачи.

Нужно проверить, как создаются activation sequences, какой verify выбран как frontier в B, как используется `source.Recovery`, и есть ли другой предусмотренный публичный маршрут. Отчёт не предписывает просто удалить guard `len(reused)==0`: это может нарушить invariant и оставить входные bindings без доказательств.

Точки входа для чтения:

- `internal/runtime/recovery.go`: `PlanRecovery`, `recoveryTrace`, `recoveryPointOf`, `recoveryClaim`, `recoveryInvariant`;
- `cmd/prifly/project_recovery.go`: sealed inputs, source workspace и source answers;
- `cmd/prifly/project_continuation.go`: CLI prepare/start и передача claim;
- `internal/runtime/resume_test.go`, `internal/runtime/recovery_test.go`;
- `cmd/prifly/project_continuation_test.go`, `cmd/prifly/project_recover_test.go`.

## Что должно помочь продолжить задачу

Минимально полезный результат — публичный prepare/start второго resume B, который:

1. Доказывает доступность и совместимость принятого префикса, включая ранее унаследованные этапы.
2. Использует текущего владельца claim, не забирает дерево назад у A и не создаёт competing owner.
3. Берёт то же дерево как оставлено, включая незакоммиченный fix; не откатывает его к HEAD или старому artifact.
4. Сохраняет исходные inputs, sealed answers и provenance принятого префикса, без фиктивных новых результатов этапов.
5. Выдаёт read-only verify; затем текущий оставшийся blocker, уже разрешённый владельцем, может штатно уйти в workspace-write fix.
6. После зелёного verify проходит оставшийся объявленный workflow, без пропуска security/review/tests/commit/MR.

Точный дизайн исправления и необходимость нормативного OpenSpec change решает разработчик Pri-Fly по правилам этого репозитория. Данный документ — отчёт об инциденте, не новая спецификация и не разрешение менять SMSPlace workflow.

## Предлагаемый регрессионный сценарий

Нужен тест именно цепочки resume, не только одиночного восстановления:

1. Run A принимает ранние этапы, затем останавливается partial на quality gate.
2. Resume A создаёт B, переносит prefix evidence и claim.
3. B выполняет gate → вложенный fix → gate и снова завершает partial на том же root frontier.
4. В workspace остаются tracked незакоммиченные изменения fix.
5. Prepare resume B должен получить непустой валидный inherited prefix и текущий claim без мутаций authority.
6. Start создаёт C, корректно передаёт дерево B → C, неизменённые inputs/answers и сведения о происхождении результатов.
7. Старые этапы не исполняются заново; дерево, включая dirty diff, не меняется от передачи.
8. C может повторно остановиться и снова возобновиться: поведение не ограничено глубиной 2.

Отрицательные проверки должны сохранить отказы при изменении effective contract принятого префикса, недоступном artifact, действительно отсутствующем/released claim, несовместимых bindings и активном child. Не выключать эти guards ради зелёного пилота. Также проверить семантику `--from-stage` для унаследованных root stages либо явно объяснить её ограничение в отказе.

## Состояние сохранённой работы и безопасная граница

При подготовке отчёта read-only git-проверками подтверждено:

- Дерево существует, HEAD содержит два implementation-коммита.
- В нём 11 tracked незакоммиченных файлов fix; суммарный diff от базы затрагивает 23 файла.
- `git diff --check` от базы — без ошибок.
- В первичном SMSPlace checkout `development`, локальные HEAD и `origin/development` совпадают с указанной базой. Это локальные refs, не свежая проверка сервера.
- Новые незатреканные файлы не показаны `git status --porcelain --untracked-files=all`; ignored QA reports могут существовать и не входят в этот вывод.

Незакоммиченные файлы, которые нельзя потерять:

```text
app/Console/Commands/AdminQaRegressFixtureCommand.php
app/Services/QaConsole/Pricing/Dto/QaPricingScenarioInput.php
app/Services/QaConsole/Pricing/QaPricingBackdatedSeeder.php
app/Services/QaConsole/Pricing/QaPricingScenarioResolver.php
app/Support/AdminQaCoverage/AdminMoonshineP9CoverageRegistry.php
app/Support/AdminQaCoverage/BusinessJourneyMatrix.php
tests/Feature/AdminQaCoverage/AdminP3AuthPagesCoverageTest.php
tests/Feature/AdminQaCoverage/AdminP7ActionCoverageTest.php
tests/Feature/AdminQaCoverage/AdminQaRegressFixtureCommandTest.php
tests/Feature/QaConsole/QaPricingScenarioControllerGateTest.php
tests/Feature/QaConsole/QaPricingScenarioEndToEndTest.php
```

Приватное PHP-окружение, оставленное для verify/tests: project `we_0fda77d0d953_d65ebf32`, app `74f1a8a62360`, DB `5cf0d815c445`, Redis `b5b1b1a6a029`. Это последние известные идентификаторы; перед использованием проверить актуальную живость и mount. Не пользоваться контейнером первичного checkout для проверки linked tree. Окружение в рамках отчёта не освобождалось.

Не делать ради восстановления:

- ручные UPDATE authority/SQLite/Run JSON;
- `git reset --hard`, очистку или release claim;
- копирование реализации в primary development;
- изменение `known-red`, frozen debt или тестовых assertions для подавления baseline failure;
- обновление shared workflow либо изменение эффектов read-only gate;
- создание MR до штатных оставшихся этапов.

Новый полный Run с переносом diff в его implement возможен как согласованный fallback, но повторяет ранние этапы и теряет удобство штатного resume. Он ещё не создан. При доступном исправлении цепочки resume предпочитаем продолжить B.

## После исправления

Сначала выполнить основной `project continue ... --source-run B --prepare`, проверить показанные prefix provenance, frontier и текущий claim. Только затем штатно запустить с возвращённым review digest и разрешёнными executable effects. Host продолжит задачу через выданные Attempts, удалит согласованные ссылки в шаге с workspace-write effect и выполнит оставшиеся проверки. Отдельного разрешения редактировать GitLab issue не дано.

## Дополнение разработчика: причина подтверждена (2026-10-02)

Первоначальная хронология выше сохраняется. Последующая диагностика подтвердила гипотезу:

- Installed `/Users/sh/.local/bin/prifly --version` сообщает `0.13.74`; с явным `--repository /Users/sh/PhpstormProjects/SMSPlace/backend` prepare от B вновь дал `resume_prefix_empty`, а от A — `recover_workspace_missing`.
- Публичное JSON-чтение B показывает `recovery/2`, source A/version 61, 13 записей `reused` и root outputs warmup/plan/improve/implement. Собственные root activations B — только verify (`call`) и fix-after-verify (`finish`).
- Claim `df98f72e4297c34fa926562ddc831d998e8bb4de58c2234b37e18747e961a62e` активен, принадлежит B, generation=2. Повторная Git-проверка показала те же 11 tracked modified files. Это наблюдение на момент диагностики, не гарантия будущего состояния.
- На текущем HEAD Pri-Fly `ac8d42b` общий `recoveryTrace` рассматривает только `source.Activations`. Унаследованные `Recovery.Reused`/`RootOutputs` не участвуют в следующем планировании. Guard видит пустую локальную часть B, хотя полный доказанный префикс непуст.
- Независимый CLI regression probe на временном программном workflow без ИИ воспроизвёл A(partial) → resume B(partial) → prepare C: B содержит одну унаследованную стадию, второй prepare возвращает `resume_prefix_empty`. Probe запускался через временный Go overlay вне репозитория; исходники не изменялись. Первый запуск был ограничен sandbox (`local_socket_unavailable`), повтор с разрешённым Unix socket воспроизвёл нужный отказ. Три существующих runtime tests одиночного resume/provenance прошли.

Installed binary не содержит VCS revision в Go build metadata, поэтому его byte-for-byte соответствие HEAD не установлено. Наблюдаемое поведение binary и независимого fixture совпадает с найденным дефектом planner. Расхождение origin `v1.50.0` и author package `1.47.0` не нужно для воспроизведения этого дефекта и не должно исправляться обновлением пакета в ходе восстановления.

`run next.continuations` — индекс объявлений установленных workflow, не обещание успешного admission конкретного launch. Текущая runner-инструкция ошибочно помогает попасть в тупик: трактует `resume_prefix_empty` как «start anew», не учитывая этот дефект. Агент не должен обходить отказ authority или начинать полный Run без согласованного плана переноса работы.

Исправление планируется в [fix-chained-workflow-resume](openspec/changes/fix-chained-workflow-resume/proposal.md); [задачи](openspec/changes/fix-chained-workflow-resume/tasks.md) включают цепочку до D, inherited bindings, dirty tree, отрицательные проверки и поставку. **Создание change не означает, что фикс реализован или установлен.**

## Инструкция агенту SMSPlace после поставки

### 1. Проверить, что фикс доступен и работа сохранилась

Работать с SMSPlace, не с checkout Pri-Fly. Прочитать актуальные `AGENTS.md`, project runner и `.prifly/local.yaml` SMSPlace; проверить реальные authority/executable paths. Команды ниже используют пути инцидента. Если пути изменились, сверить их с конфигурацией и сообщить расхождение до запуска.

Владелец/разработчик должен назвать фактическую версию релиза, содержащую фикс, и его проверенное evidence. Сравнить её с installed version. `0.13.74` дефект содержит; номер будущего фиксирующего релиза здесь намеренно не придуман. Автоматически менять workflow package, project.yaml или runner источники ради прохождения prepare нельзя.

```sh
PRIFLY_BIN=/Users/sh/.local/bin/prifly
SMS_AUTHORITY=/Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority
SMS_REPOSITORY=/Users/sh/PhpstormProjects/SMSPlace/backend
SMS_SOURCE_RUN=run:80e270db507eab9ba8faad23205852d2be95ade93e16f41be94819c6e6c20d1e
SMS_TREE=/Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority/.prifly/work/claims/df98f72e4297c34fa926562ddc831d998e8bb4de58c2234b37e18747e961a62e

"$PRIFLY_BIN" --version
"$PRIFLY_BIN" --project "$SMS_AUTHORITY" run status "$SMS_SOURCE_RUN" --json
"$PRIFLY_BIN" --project "$SMS_AUTHORITY" run next "$SMS_SOURCE_RUN" --json
"$PRIFLY_BIN" --project "$SMS_AUTHORITY" claim list --json
git -C "$SMS_TREE" rev-parse HEAD
git -C "$SMS_TREE" status --porcelain --untracked-files=all
git -C "$SMS_TREE" diff --check
```

Перед первым mutating start сохранить вне дерева binary diff tracked изменений, список untracked файлов и необходимые ignored QA reports по правилам проекта. Diff не сохраняет untracked/ignored bytes: их нужно сохранить отдельно, если появились. Это копия рабочих файлов, не ручное редактирование authority. Не делать commit незавершённого fix ради одной передачи claim.

Ожидается source B `completed/partial`, version 36, без live driver, pending decision и unresolved execution; active claim принадлежит B и имеет generation 2; HEAD `793df111e028a59e52563be58fd447d2d7d56f93`, 11 modified files из списка выше. Если состояние другое, выяснить текущего владельца и наличие уже созданного continuation; не запускать ещё один Run вслепую. Номера версий/файлов — контрольная точка инцидента, не основание откатывать изменившуюся работу.

### 2. Получить и проверить read-only prepare

```sh
"$PRIFLY_BIN" --project "$SMS_AUTHORITY" project continue \
  --repository "$SMS_REPOSITORY" \
  --launch aif-classic --source-run "$SMS_SOURCE_RUN" \
  --host codex-app --prepare --json
```

`--repository` обязателен в этой инструкции: authority path не выбирает авторский checkout автоматически; выполнение из Pri-Fly без него может компилировать другой launch.

Сохранить полный prepare и проверить:

- Source — B с актуальной версией, workflow — `aif:workflow/classic`, host — codex-app.
- Frontier — root verify, действие — execute. Весь его вложенный quality workflow выполняется заново; inherited warmup/plan/improve/implement не назначены к повтору.
- Префикс непуст, ранние результаты доказуемо происходят из A через B; в плане доступны нужные root outputs/bindings.
- Передаваемый claim — тот же ID/path, текущий owner B и актуальная generation. Не новое дерево от primary checkout и не попытка вернуть ownership A.
- Inputs, sealed answers, profile/policy и значимые контракты совпадают с source. Не передавать новые `--input`, `--from-stage`, questionnaire answers или override workspace ради обхода отказа.
- Prepare не изменил Run versions, claim owner/generation, Registry или рабочие файлы. Значимые source/target/evidence изменения требуют нового prepare.

Если prepare снова отказывает или план не соответствует этим условиям: сохранить JSON refusal и свежие status/next/claim facts, передать разработчику Pri-Fly, остановить зависимый start. Не выпускать claim, не менять SQLite, не использовать raw fork, `--allow-duplicate-continuation` или полный Run как автоматический fallback.

### 3. Запустить ровно просмотренный план

После проверки prepare использовать его верхнеуровневый launch `review_digest`, не вложенный `recovery.review_digest`. Start допустим при действующем разрешении владельца продолжить работу и выполнять перечисленные программы; ранее данное разрешение не нужно запрашивать повторно, если scope и эффекты совпадают. Сам этот отчёт не добавляет разрешения на новые внешние действия.

```sh
# Подставить точный верхнеуровневый review_digest последнего проверенного prepare.
SMS_LAUNCH_DIGEST='<review_digest из prepare>'
"$PRIFLY_BIN" --project "$SMS_AUTHORITY" project continue \
  --repository "$SMS_REPOSITORY" \
  --launch aif-classic --source-run "$SMS_SOURCE_RUN" \
  --host codex-app --expected-launch-digest "$SMS_LAUNCH_DIGEST" \
  --allow-execution --json
```

Флаг `--allow-execution` используется только в рамках указанного разрешения. Записать возвращённый новый Run ID C, затем читать `run status C`, `run next C` и claims. Claim должен принадлежать C с новым поколением, путь — сохранённый; ранние стадии не должны получить новые Attempts. До первого workspace-write шага проверить сохранность HEAD и dirty файлов. При потере ответа сначала найти уже созданный child публичным чтением; повтор команды с новым command identity может создать другой Run, поэтому не повторять start вслепую.

### 4. Завершать через выданные Attempts

Следовать `run next C`, `session task`, `session submit` и текущей runner-инструкции. Gate verify read-only: две разрешённые правки source выполняются только в выданном шаге с `workspace_write`, не между Attempts и не внутри verify.

В новом verify сохранить фактический baseline finding и актуальное решение владельца: разрешено удалить только номера задач/MR из комментариев двух тестов ниже. Разрешение сообщается в предусмотренном отчёте/контексте шага, не через подмену sealed inputs/answers и не через зелёный verdict при падающей проверке.

- `tests/Unit/Pricing/PricingModelCorpusManifestTest.php`;
- `tests/Unit/Traceability/Ratchet/RatchetPolicyTest.php`.

В fix оставить даты, объяснения, разделы ТЗ, requirement IDs, assertions, hashes, frozen debt и known-red. Проверить diff этих файлов, затем повторить verify на том же claimed дереве. Перед командами проверки подтвердить актуальную живость приватного PHP-окружения и его mount на `$SMS_TREE`; исторические container IDs выше не являются гарантией. Если снова нужен release/recreate окружения, действовать по текущим полномочиям и правилам `worktree-env`, сохраняя работу.

Если существующий workflow при актуальном разрешении всё равно не выдаёт fix и снова заканчивается partial, сохранить gate/route evidence и сообщить отдельный блокер. Не менять authoring YAML и не превращать owner-only finding в warning для продолжения. Следующий resume готовится от последнего Run, владеющего claim, после выяснения причины.

После зелёного verify выполнить все оставшиеся объявленные stages по графу и tasks: security/review, дополнительные tests, commit и delivery/MR там, где они предусмотрены и разрешены. Само завершение verify не является завершением задачи. MR не создавать в обход выданного external-write шага/полномочий; GitLab issue не редактировать без отдельного действующего разрешения.

### 5. Сохранить итог и не спутать его с фиксом движка

Дополнить этот отчёт фактической версией fixed binary, prepare/launch digests, C и дальнейшими Run IDs, claim transfer и сохранностью dirty work, результатами gates, implementation/fix commits, ссылкой MR при его создании и точным terminal status/outcome. Отдельно указать, были ли ранние стадии исполнены повторно, какие проверки остались и кто сейчас владеет деревом. Даже terminal partial нужно назвать partial, не успешным завершением.

Закрытие change Pri-Fly, прохождение unit/CLI tests и выполнение SMSPlace #163 — отдельные факты. До выполнения описанных действий SMSPlace остаётся в сохранённом B; инструкция является планом, не evidence состоявшегося восстановления.
