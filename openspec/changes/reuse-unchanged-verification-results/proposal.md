## Why

При подготовке 0.13.76 локальный `ci-check` повторялся на неизменённых входах между implement, verify и commit. Дополнительно standing `gate_checks` запускает полный `e2e` внутри обычного Run, хотя AGENTS.md относит race и полный e2e к релизу; отсутствие общей процедуры поиска текущих CI results также провоцирует лишний dispatch.

## What Changes

- Ввести проверяемое локальное повторное использование успешной проверки для того же полного снимка входов и контекста исполнения. Повторное использование не заменяет обязательный состав `ci-check`; отсутствие подходящего evidence запускает проверку заново.
- Согласовать обычные стадии разработки: точечные проверки во время правок, один полный `ci-check` для итогового снимка перед commit, без полного race/e2e в стандартном Run. Специальный сценарий или явный запрос владельца может требовать дополнительных проверок.
- Перед релизным dispatch находить успешные либо ещё работающие `verify` / `qualify` для точного SHA. Подходящий успешный результат использовать, активный запуск наблюдать, недостающий запускать; повторные обращения не создают второй дорогой batch без причины.
- Предусмотреть явный принудительный повтор и отчёт о том, что выполнено заново, а что использовано из прежнего запуска, с identity и причиной решения.
- Сохранить полный состав CI `verify`, race + полный e2e в `qualify`, проверку перед tag и owner-controlled release. Новый commit, включая documentation-only, требует собственных CI gates: перенос между SHA исключён решением владельца от 2026-10-12.

## Capabilities

### New Capabilities

Нет: это изменение процесса разработки и поставки репозитория.

### Modified Capabilities

- `quality-and-acceptance`: условия локального повторного использования проверок и разделение обычного Run и release qualification.
- `release-distribution`: дедупликация проверок на точном SHA без ослабления обязательных релизных ворот.

## Impact

Меняется только процесс этого репозитория: `Makefile`, новые небольшие helpers и их tests в `scripts/`, `.github/workflows/{verify,qualify}.yml`, `scripts/tag-release.py`, release CI guard, `AGENTS.md`, README и project standing answers/runner overlays. Точные имена новых helpers и интерфейсы определены в design.

Runtime, public CLI Pri-Fly, YAML authoring schemas, saved Runs, общий generated `prifly-run` и внешний canonical AI Factory package не меняются. Правила проверок не навязываются чужим workflows. Существующий `gate_checks` остаётся проектным объявлением состава; новые поля схемы не нужны.

Нормативная ownership не меняется: source sets — `openspec/specs/quality-and-acceptance/` и `openspec/specs/release-distribution/spec.md` по SOURCE-OF-TRUTH. Delta действует до последующего sync/archive. Historical evidence, archived changes и released schemas не переписываются. Отложенная Claude UI-приёмка остаётся отдельным `verify-claude-launch-choice-ui`, без изменения её статуса.
