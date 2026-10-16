# Handoff W6: released Core 0.13.78 для native Call extensions

Подготовлен 2026-10-17. Core #9 завершён в согласованном release scope; независимая
приёмка W6 в репозитории `prifly-aif-workflows` ещё не выполнена этим change.
Этот artifact готов для владельца/следующего исполнителя W6; внешние сообщения
не отправлялись, consumer repository не изменялся.

## Закреплённая поставка

- Stable [v0.13.78](https://github.com/StenHigh/prifly/releases/tag/v0.13.78), source SHA `201d4428b60e6afc9aef84fc5197a3185cf2f20b`.
- Exact linux/amd64 binary: `sha256:60a6dd1ca0524b4927418a64311207b4d6739080a8f518374d1889726d68ab5f`, Go 1.27.2.
- Signed manifest: `sha256:eef734f13f5e9820a38826828c16f0b9fdee7576415874a716aba7fb1c2998d0`; archive hashes, key/signature и installer probe — [release evidence](release-0.13.78.md).
- Required exact-SHA [verify](https://github.com/StenHigh/prifly/actions/runs/38970868425), [qualify](https://github.com/StenHigh/prifly/actions/runs/38970886907) и [publication](https://github.com/StenHigh/prifly/actions/runs/38972184928): PASS.
- Тот же released binary: public E5 **51/51 PASS**, legacy authoring corpus PASS. [Инженерная traceability](implementation-evidence.md) описывает исходные регрессии; [live host evidence](live-host-evidence.md) дополняет прежние pending notes, не переписывая их.

Core 0.13.77 — только неподдерживающий baseline для новой call syntax.
Поддерживаемый минимум **0.13.78+** указан в author index/reference.

## Контракт для автора consumer

Использовать Project `/3` и `authoring: prifly-extension/2` в `extend.yaml`.
Запись содержит ровно один `step` или `call`. Call `id` — ID вставленной root
stage; `call` — short name единственного объявленного local WorkflowRevision
в той же folder (`workflows/` / `project/workflows/`). Вендорить child bundle
со всеми нужными definitions/files явно: установка package не создаёт
automatic cross-package import. Source root/siblings и canonical workflows
не редактируются ради extension; вставка заменяет один root route.

```yaml
authoring: prifly-extension/2
extensions:
  - id: product-quality
    workflow: my-root
    between: {from: prepare-product, to: commit}
    call: project-product-quality
    input_bindings:
      configuration: $stages.prepare-product.configuration
      ui: $stages.prepare-product.ui
      ux: $stages.prepare-product.ux
    on: {succeeded: commit, partial: stopped, rejected: stopped}
```

Это фрагмент: все названные root stages, callee ports/schema refs и три outcomes
должны быть объявлены. Для другого callee перечислить **все его** allowed outcomes;
optional `on_error` ведёт в существующую root stage. Native минимум: WorkflowRevision
1, core state/read 2; дополнительные fields сохраняют свои contract minima.
Step legacy сохраняет прежний ID/binding limit/verdicts; `/2` profile и unmarked
step не мигрируют неявно. Standalone `project extend` остаётся step utility;
call требует folder inventory `project compile`.

Входы типизированы exact schema refs. Downstream читает только гарантированные
exports через `$stages.product-quality.PORT`, не приватные child stages.
`configuration` не получает project scope по имени. Partial/rejected/error/cancel
не создают success artifacts. Program execution bindings допускаются только для
owned reachable leaf steps call closure, с обычными allowlist/consent.

Read-only prepare фиксирует closure/bindings/routes/resources/supporting files;
start проверяет expected launch digest заново. Resume не переносит stale accepted
outputs; допустимый root replay повторяет child целиком. Один Run/claim/candidate,
native ownership, depth/step/transition/storage/resource/effect/trust ceilings
остаются общими. Native cancel сам по себе не завершает внешний browser actor.
Полный справочник: `examples/authoring/extension-authoring-reference.yaml`,
continuation/troubleshooting и `test/fixtures/project-call-extensions/` в Core.

## E5-A1…A10: что передаёт Core

| E5 | Завершённое доказательство | Граница для W6 |
|---|---|---|
| A1 | Marker/one-of/profile/schema/legacy vectors; released compile | W6 фиксирует supporting release и свои package versions |
| A2 | Root native insertion и nested calls в одном Run, caller ancestry | Нужны реальные review/UIUX recipes minimal + classic project extension |
| A3 | Один/три inputs, exact schemas, все exports прочитаны downstream | W6 формирует настоящие request/configuration/ui/ux, без prompt bridge |
| A4 | 14 public negative cases + unit inventory/profile/program refusals, без partial execution | Сохранить named Problems для собственных негативных recipes |
| A5 | Declared succeeded/partial/rejected, output guarantees, technical errors | Product gate, warning/defect/unavailable/cleanup policy задаёт W6 |
| A6 | Cancel/timeout/error, settlement/resources/replay; live control двух hosts | Browser ownership/cleanup/reconciliation требует actual consumer observations |
| A7 | Shared limits/policy/resources и finite storage refusal | Повторить ceilings на своей реальной closure, без silent override |
| A8 | 13 freshness mutations, inherited prefix, changed-child refusal/whole-child replay | Проверить изменённые consumer reports/config и intervening candidate edit |
| A9 | Root/sibling hashes, unchanged legacy corpus/b1 | Canonical classic/profiled и прежние W1/W2/parallel recipes проверяет W6 |
| A10 | Schemas/docs/typed diagnostics; synthetic runners 3 hosts; live app/CLI PASS | Claude live исключён владельцем для 0.13.78; browser/model/consumer matrix независимы |

Live app execution runtime и Codex CLI — `0.162.1`; успех/control по обоим,
полные native identities и artifact digests — [JSON](live-host-results.json).
Claude Code `2.1.287` не авторизован, подписка недоступна. Владелец явно исключил
live Claude из приёмки 0.13.78; это **не PASS**. Отдельный
[follow-up](../../../../.ai-factory/plans/qualify-claude-native-call-host.md)
сохраняет эту проверку pending. W6 не наследует исключение автоматически.

## Следующая работа W6

Read-only snapshot consumer repository: `a4d900bb19f2f75eb59422a46de64be45ee26435`,
`docs/changes/add-review-uiux-call-extensions/{proposal,tasks,acceptance}.md` и
`docs/changes/engine/E5-nested-workflow-extensions.md`. Там supporting Core ещё
отмечен как pending/0.13.77 baseline — следующий исполнитель должен обновить
свой plan по этому handoff, затем реализовать и квалифицировать consumer.

Из published 1.52.0 известны `aif-evidence:workflow/quality@1.0.0`
(`request → report/gate`) и `aif-product:workflow/quality@1.0.0`
(`configuration/ui/ux → gate/ui_report/ux_report/environment/release`).
Наличие более новых package releases не доказывает их квалификацию по W6.

Проверить W6-A1…A10 по его собственному plan: три recipes (review-only,
UIUX-only, review→UIUX), minimal и неизменный upstream classic; реальные
inputs/exports/dirty candidate provenance; truthful gate routes; actual Chromium,
project-owned environment/session и cleanup; cancelled/timeout/error recovery,
fresh round и stale-prefix refusals; budgets/permissions; legacy/W1/W2 и
canonical hashes; docs/package bumps/поддержанные hosts. Required unavailable,
missing/stale report и cleanup failure не должны доходить до commit.

Для повторения Core corpus:

```sh
python3 -B test/e2e/verify-call-extensions.py --binary /absolute/path/to/released/prifly
python3 -B test/e2e/verify-authoring.py --binary /absolute/path/to/released/prifly
```

Live protocol воспроизводится отдельно по `live-host-evidence.md`. Сохранить
свои release/binary/package hashes, commands/results, original artifact refs,
source hashes и final CI URL. Core changes #7/#8 уже в main/archives; recovery
исходных пользовательских Runs не выполнялся. Ни W6, ни P2/product milestone
этим handoff не закрываются. Пункт Core 7.3 означает подготовленный handoff,
а не будущий PASS consumer qualification.
