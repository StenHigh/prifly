# Локальная приёмка recovery

## Реализация

Implementation checkpoint: `17738b705dd30a81cee9dd50da6300bc9b906df7`.
Claim-ветка: `prifly/2eb9330e08f69edc74f830aff6423d71221313b908f8e20a2c2887df6290fb05`.
Baseline: `9df35bbe94d45a7f5140962df42ebca335f5f159`.

Recovery получает отдельный selector одной recorded failure chain; `brokenStage`
и reopen не меняются. Call/repeat возвращается к root stage, исключая весь
subtree. До admission нет source Attempt/candidate, действие — execute.
Natural eligibility применяется также перед earlier/inherited root selection
и при проверке ancestry. Контракты схем, capability IDs и historical evidence
не изменены.

## Проверки реализации

- Красная regression на baseline 9df35bb: root step, call, repeat, mixed chain,
  handled historical failure и repeat после принятой итерации — реальный
  `recover_frontier_unsupported`. Fixture создаёт настоящий recorded отказ.
- Матрица `Test.*(Recover|Recovery|Resume|Reopen)` PASS:
  runtime 14.839s; CLI 46.026s.
- Добавленные invalid/empty/late/root-choice и stale-source проверки PASS:
  `TestRecoverPreparationRootFrontier` 5.662s.
- `make refusal-check schemas-check` PASS, опубликованные schema bytes совпали.
- Первая полная test suite PASS (CLI 210.756s, runtime 176.717s); vet
  обнаружил self-assignment в fixture. Checkpoint 17738b7 устранил его;
  `go vet ./internal/runtime` и focused regression (6.241s) PASS. Итоговый
  required gate после этого исправления фиксируется runtime отчётом.
- `TestEveryDeclaredCapabilityIsInTheAuthorIndex` PASS.
- Disposable public CLI: prepare/default/earlier; takeover того же dirty claim;
  generation +1; old plan/untracked/HEAD/diff и source journal сохранены;
  default вновь даёт drift без Attempt. После inherited earlier stage новый
  output захвачен в законной новой location; старый dirty plan сохранён.
- Missing/cyclic/mismatched ancestry, independent causes в том же caller,
  inconsistent repeat decision, отсутствующие pre-admission facts и uncertain
  execution остаются отказами. Старые reopen tests проходят.

Исправленный полный `ci-check` PASS; OpenSpec **31/31**, `git diff --check` PASS.
Canonical verify/security/review установлены без блокеров в Run
`run:bdc18cb72dd0775f05d573de6f18edd776bae25ce7bc29d2b8b4f85740bc52ab`.
Advisory findings: milestone none; прежнее отсутствие .env в .gitignore;
generic security script сканирует cache и не включает Go — cache false
positives разобраны, Go diff проверен вручную. Standing gate_warnings=ignore.
Local commit и финальный снимок ledger фиксируются runtime отчётом этого Run;
результаты предыдущего claim сюда не переносились. Закреплённый helper SHA256:
`1855a4fffa7b8e9c6935908d42067565e5b5b1fd002c4942027ffc44acae500b`.

## Development CLI

Version: `0.13.76-dev.nested-recovery+17738b705dd30a81cee9dd50da6300bc9b906df7`; Go 1.27.2, darwin/arm64.
SHA256: `bb88ca56f5a82f35929a2d4b334d4545b95fadf20f260c55b4bf7ced99044bca`.
Проверенная сборка: `/private/tmp/prifly-nested-recovery/authority/.prifly/work/claims/2eb9330e08f69edc74f830aff6423d71221313b908f8e20a2c2887df6290fb05/bin/prifly-recovery-dev`.
`go version -m`: vcs.revision = implementation checkpoint; vcs.modified=false.
Итоговый documentation commit сохраняет эти code bytes.
Для исходного хоста после завершения Run сохраняется копия с тем же SHA256:
`/Users/sh/PhpstormProjects/Pri-Fly/github/bin/prifly-recovery-dev`.

Установленный `/Users/sh/.local/bin/prifly`: `0.13.76`,
SHA256 `5df5aceccde63c173d3f38f20426eb9dd131e6c3b0f6511a99002ad874a49f00`; он сохранён.
Release/tag/push и внешнее recovery START не выполнялись.

## Read-only SMSPlace prepare

Source: `run:697e667357c9b14b0e250b07d3423a49fbfe349751dce6c0de1532fb58e2a6d0`; Run version **34**, event sequence **76**.
Claim `claim:314dacaed9b5053a557d1393e4b8b25737dcf216245f4a554b1d92fee69b803e`
сохранён целиком, active generation **1**.
Текущий `.ai-factory/PLAN.md` SHA256: `835061e4b95486bce3c7ac0a86e55bae120162cddc006d8629af110cf8cd9c9f`.
До/после сравнивались весь source Run, version/event sequence и claims list;
source/task/answers/artifact refs, history и дерево не изменились.

| Prepare | Frontier | Action | Reused | Review digest |
|---|---|---|---|---|
| natural | improve | execute | 2 | `sha256:55238e4f3e84ff07ec8f1a4c2bd7b2eff4b2f73c0c573e92dd039656df883a0d` |
| from_plan | plan | execute | 1 | `sha256:b7ba351bb1585c00b58e9c42d58de29a46b81ff4b2f242367251d2829973e255` |

Оба результата не содержат source frontier Attempt или candidate.
Рекомендованная команда (сначала fresh prepare, затем собственный reviewed START
у исходного хоста):

```sh
/private/tmp/prifly-nested-recovery/authority/.prifly/work/claims/2eb9330e08f69edc74f830aff6423d71221313b908f8e20a2c2887df6290fb05/bin/prifly-recovery-dev --project /Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority project recover --repository /Users/sh/PhpstormProjects/SMSPlace/backend --launch aif-classic --source-run run:697e667357c9b14b0e250b07d3423a49fbfe349751dce6c0de1532fb58e2a6d0 --host codex-app --prepare --from-stage plan --json
```

Default START не исправит drift. При earlier plan точный output-only capture
старого workflow может дать `workspace_tree_output_exists` для сохранённого
unattributed `.ai-factory/PLAN.md`: этот отказ не ослаблялся. Внешний START
этим change не подтверждался и не выполнялся; сохранение/перемещение файла
и дальнейшую продуктовую работу решает исходный хост с владельцем.

Предыдущий Pri-Fly claim verification reuse с HEAD `b98064d` остаётся отдельно;
main остаётся на planning commit. Claude UI-приёмка по прежнему change отложена.
