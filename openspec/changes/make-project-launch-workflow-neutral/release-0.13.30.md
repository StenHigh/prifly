# Выпуск Pri-Fly 0.13.30

Четыре починки, найденные за один день, когда движок стал потребителем
собственного сценария (`aif-classic` 1.37.0 в этом репозитории) и когда по
всему модулю прошли `staticcheck` и ревью. Тег на `a2f0699`.

## Что было

- **Discovery над `~/.prifly`.** `projectRoot` шла вверх и принимала любой
  `.prifly` предка за профиль — включая user dir движка (`~/.prifly`: монитор,
  стенды, authorities), который лежит над каждым репозиторием под HOME. Первый
  `project init` в таком репозитории отказывал `unsafe_authority_root`, а с
  явным `--state-root` записал бы `project.yaml` и runner'ы в HOME. Свойство с
  0.13.16 (появление `~/.prifly`), пакетчик воспроизвёл на 0.13.28 и 0.13.29;
  пилот не попадал, потому что его `.prifly` появился раньше `~/.prifly`.
- **Standing-ответ вне вопросов профиля.** `answers.preflight.plan_docs`
  (условие `profiles: [full, ultra]`) в `extend.yaml` при
  `--package-profile fast` давал `project_start_unknown_decision … (from
  extend.yaml) does not apply` — один `extend.yaml` не мог служить двум
  профилям.
- **Standing runtime-ответ не применялся мостом.** `sealedDecisionAnswer`
  знал только source `actor` (флаг); `answers.runtime.improve_apply` из
  `extend.yaml` показывался запечатанным в сводке запуска, но шаг спрашивал
  заново, autonomous-заход встал бы в `waiting_decision`, а
  `autonomy_unanswered` ложно называл его. Найдено пилотом на боевом заходе.
- **`run fork` с некомпилируемым workflow.** Ошибка `flow.CompileCore`
  присваивалась затенённому `err`, `plan` оставался `nil`, `planRef`
  падал nil-паникой. Найдено `staticcheck` (SA4006) при ревью.

## Что стало

- Голый `.prifly` — профиль только в стартовом каталоге; выше считается
  только `project.yaml`. Тест `TestCLIProjectInitBelowBareUserDirectory`.
- Standing-ответ к вопросу, который выбранный профиль или прежний ответ не
  задаёт, отбрасывается (до неподвижной точки — снятие одного может снять
  условие другого); флаг к такому вопросу — по-прежнему отказ. Тест
  `TestProjectStandingAnswerOutsideThisRunIsDropped`.
- Запечатанным считается ответ source `actor` или `project_default`; ledger
  называет реальный источник. Тест `TestDecisionBridgeAppliesTheOwnersSealedAnswer`
  на оба источника; `DecisionsAutonomyCannotTake` не перечисляет standing.
- Ветка компиляции fork вынесена в `compileForkPlan` с одним путём ошибки.
  Тест `TestCompileForkPlanReportsACompileFailure`.
- `staticcheck` (v0.8.1) и `govulncheck` (v1.8.0) — в `make check` и
  `make ci-check`, пришпилены, печатают число прочитанных пакетов; их первые
  11 находок закрыты; `golang.org/x/text` → v0.39.0 (GO-2026-5970, косвенная).

## Ворота

`make ci-check` (в том числе staticcheck 9 пакетов без находок, vuln-check без
уязвимостей, fmt-check 289, refusal-check 158, все публичные схемы совпали),
`make e2e` (6 наборов), `make race` (`cmd/prifly` 166 с, `internal/runtime`
752 с, гонок нет) — всё на `a2f0699`; CI `verify` на push; run выпуска
35217483068, все три job'а success.

## Проверено на опубликованном бинаре

После `prifly update` → `0.13.30`: `project init` в свежем репозитории под HOME
при существующем `~/.prifly` создаёт профиль в репозитории; анкета
`aif-classic` с `--package-profile fast` при standing `plan_docs`/
`plan_logging` — `inactive`, без отказа. Стенд пакетчика — после его
пересборки (он ждал именно этот тег).
