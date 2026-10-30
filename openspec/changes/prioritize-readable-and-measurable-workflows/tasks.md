## 1. Обновление roadmap

- [x] 1.1 Согласовать proposal/design/delta с подтверждённым расширением до 14 направлений и применить три ADDED Requirements к `openspec/specs/delivery-roadmap/spec.md`. Проверка: все прежние направления сохранены, новые имеют prerequisites и результат; roadmap действительно обновлён, возможности не объявлены реализованными.
- [x] 1.2 Проверить связи с `refresh-author-guides-and-examples`, активными launch/monitor changes и существующими model-profile/source-adapter/cost записями. Проверка: нет дублирующих задач, переноса backlog внешних packages и изменения статусов P1/P2; ссылки ведут к существующим sources.
- [x] 1.3 Сохранить согласованный HTML-разбор YAML и расширенный обзор в `openspec/specs/delivery-roadmap/reports/`; сослаться на оба из roadmap. Проверка: standalone HTML содержит дату и ненормативный статус, repository links относительны и разрешаются; нет machine-local paths, сетевых scripts и обязательных внешних ресурсов.
- [x] 1.4 Уточнить в `openspec/SOURCE-OF-TRUTH.md`, что HTML-отчёты не являются normative source или вторым backlog. Проверка: ownership-таблица прежняя, runtime/build pipeline и release-documentation-layout не меняются.

## 2. Приёмка документационного изменения

- [x] 2.1 Выполнить `openspec validate prioritize-readable-and-measurable-workflows --strict`, `openspec validate --all --strict --no-interactive` и `git diff --check`; проверить также новые Markdown/HTML на whitespace, ссылки, встроенный JavaScript и отсутствие зависимости от сети. Проверка: команды проходят, delta совпадает с добавленными требованиями roadmap; HTML читается без builder.
- [x] 2.2 Проверить `git diff --name-status` и список новых файлов. Проверка: diff ограничен этим change, delivery roadmap с отчётами и пояснением в SOURCE-OF-TRUTH; нет правок runtime, `.prifly/`, опубликованных `schemas/`, glossary bindings, `openspec/changes/archive/`, historical `release-*.md` и release manifests. Наличие плана не объявляет функции реализованными. Если затем создаётся commit, итоговый snapshot проходит `make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1` по `AGENTS.md`; документационные проверки этот gate не заменяют.

## Результат проверки 2026-10-30

- Все 14 направлений находятся в основном roadmap; три добавленных требования
  совпадают с delta и встречаются ровно один раз.
- OpenSpec: новый change valid; полный strict batch — 33 passed, 0 failed.
- `git diff --check` и отдельная проверка whitespace новых файлов прошли.
- Все локальные ссылки обоих standalone HTML разрешаются; machine-local paths,
  зависимости от active change paths и внешние runtime resources отсутствуют. Встроенные scripts
  прошли `node --check`; визуальная проверка браузером не выполнялась.
- Таблицы P1/P2 и ownership не изменились. Защищённые исходники, schemas,
  установленные workflows, архивы и release evidence не затронуты.
- Коммит, push, архивирование, новые runtime Runs и полные runtime gates
  не выполнялись: сохранены документы и план будущих возможностей.
