# Сверка 2026-11-08

Baseline: main `ade4d982d7f05338760720ea2dccbbade985f393`, чистый новый checkout.
Проверены все 21 неархивированный change: счётчики взяты из фактических tasks.md;
новый change сверки не входит в свой исторический baseline inventory.
GitHub latest stable: [v0.13.88](https://github.com/StenHigh/prifly/releases/tag/v0.13.88).
[Nested points v0.13.83](https://github.com/StenHigh/prifly/releases/tag/v0.13.83)
содержит опубликованную приёмку 11 generic scenarios / 12 Runs;
[composition v0.13.85](https://github.com/StenHigh/prifly/releases/tag/v0.13.85)
подтверждает поставку imports/layers и extension /5.
Не утверждается, что незакрытые checklist автоматически приняты по release notes.

Проверки этого изменения:
- PASS: Python-сверка всех 21 строк с tasks; точные completed/total counts.
- PASS: все относительные файловые ссылки нормативного roadmap существуют.
- PASS: git diff --check; runtime, schemas и archive не изменены.
- BLOCKED: make test-changed GO=/usr/bin/go остановлен на go list до тестов.
  Штатный .tools/go/bin/go отсутствует; /usr/bin/go не является Go compiler
  (go version отвечает «Unknown option: version»). Проект требует Go 1.27.3.
- NOT RUN: strict OpenSpec validation, CLI openspec отсутствует.
- NOT RUN: full ci-check/qualify, build и release replay; на 2026-11-08 это documentation-only
  сверка без commit/push, без установки toolchain или изменения пакетов.

Issues, tasks исходных changes и release evidence не изменялись. Все live,
production, Claude UI и formal P1/P2 gates сохраняют исходные ограничения.

## Подготовка draft PR 2026-11-09

Повторный fetch main: baseline ade4d98 не изменился. Изменения сохраняются
в отдельной ветке, без merge и правок исходных issues.
`make ci-check-once CHECK_CONTEXT=cloud-roadmap-docs-2026-11-09` вызван:
BLOCKED до первого теста, отсутствует `.tools/go/bin/go` (Error 127).
Это не успешная CI-приёмка. Ограничение вынесено в draft PR; required CI
должен выполняться на точном опубликованном commit. Strict OpenSpec validation
по-прежнему не запускался из-за отсутствующего CLI.

## Обновление на main 2026-11-10

База 3dfca79fb42963b29cd3c2a8e5ae2acedebb3590. Latest stable0.13.90
подтверждён через GitHub. Новые shared-schema migration0.13.89 и owner compile
parameters0.13.90 уже архивированы; добавлены ссылки на их tasks/released evidence.
Все 21 прежних активных changes и счётчики сверены повторно. Runtime, schemas,
новые примеры и архивы main сохраняются без изменений. Roadmap product gates прежние.
Предыдущий SHAe0095f1 прошёл оба GitHub verify; это не результат нового SHA.
Локальный Go toolchain и OpenSpec CLI по-прежнему отсутствуют.

Полный helper ci-check-once повторно вызван с контекстом cloud-roadmap-docs-2026-11-10: BLOCKED до тестов, отсутствует .tools/go/bin/go (Error127). Python-сверка inventory/counts/ссылок и git diff --check PASS.

## Повторная сверка после 0.13.91

База main60931dc6f77770d748ff4ff0fc58b8c177b74dfa, release v0.13.91.
PR25 добавил сохранённый выбор миграции схем launch; change архивирован
2026-11-10. Roadmap отражает поставленную capability, но не объявляет
приёмку частной authority или P1/P2. Новые исходники, спецификации, схемы,
справочники и архив main сохранены без правок.
Все 21 исходных active changes и точные checklist counts перепроверены.
PASS: inventory, относительные ссылки roadmap и git diff --check.
Локальные full CI и strict OpenSpec остаются недоступны без Go toolchain
и OpenSpec CLI. Предыдущий SHA c9de00b прошёл GitHub verify; это не результат
нового SHA. Required CI проверяется отдельно после публикации ветки.
