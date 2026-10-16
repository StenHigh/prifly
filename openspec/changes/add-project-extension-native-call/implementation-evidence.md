# Native Call extensions: инженерная приёмка

## Scope и baseline

Change: `add-project-extension-native-call`, issue https://github.com/StenHigh/prifly/issues/9.
Baseline: `00cbccac08329520271565d0d104fed672c43dcd` — #7/#8 уже в main, их changes архивированы; E5 не подменяет их приёмку.
Claim branch: `prifly/ba5628982f0031d716a09b1dd7d75d5108e53b2d2a8246b8f14af5173b86e9c2`.
Run: `run:f046c7b5f01582c1bb6a52fc21c33bcfbc893be762b428499ea3161a3bb1348a`; launch `aif-classic` 1.40.0, full / attended / codex-app, worktree.
Все fixtures — disposable; исходные authority/Run из issues не открывались и не менялись.

Текущий scope — инженерная реализация 1.1–6.3. Пункт 6.2 подтверждается финальным
staged batch и native verify; результат и exact head сохраняются в Run artifacts.
7.1–7.3 остаются открытыми до fixed-SHA release qualification, released CLI и
live host evidence. Design Migration Plan §4 прямо допускает implementation commit
до release. Ни закрытие issue, ни archive E5, ни P1/P2 acceptance здесь не объявлены.

## Реализация и совместимость

- `prifly-extension/2` требует Project `/3`, ровно одну step/call ветвь. Legacy
  unmarked step ID/cardinality/verdicts и b1 encoding сохранены.
- Source inventory разрешает один local callee до compilation. Extension даёт
  root→callee dependency edge; cycles (включая call/repeat) отказываются. Registry
  latest/installed-but-undeclared lookup отсутствует.
- Detached root splice создаёт обычный native Call с extension ID, typed bindings,
  всеми declared outcomes и optional on_error. Native port/config/default/schema
  и guaranteed-output проверки общие с authored Call.
- Новая ветвь получает native typed codes/paths; legacy compile errors сохраняют
  прежний wire code. Program bindings ограничены reachable owned steps closure,
  обычными supporting files/allowlist/consent; upstream binding override запрещён.
- Declared resources новой call ветви входят в b1 input; effective local resource
  limits включены в opaque review digest и перечитываются перед dispatch.
  Supporting program files присутствуют в compile `sources`.
- Runtime operator, storage migration и closed Run/Session/Problem DTO не добавлены.
  Дочерний WorkflowInvocation использует тот же Run/claim/candidate, native counters,
  controls, resources, policy и replay.
- Новые selecting `extension` и explicit `extension-v2` schemas опубликованы рядом
  с неизменной v1; automatic pattern только у selector, project-owned paths покрыты.
- `project_call_extensions`, author index/reference, continuation/troubleshooting,
  glossary 1.36 и generated runner guidance обновлены вместе.

## Checks

| Проверка | Результат / evidence |
|---|---|
| Existing baseline Call/Extension/Build | PASS, `/tmp/prifly-issue9-baseline-tests.log` |
| New marker/target regressions до кода | RED: baseline отвергает authoring/call, `/tmp/prifly-issue9-red.log` |
| Compiler/native negatives, exact schema/export guarantees, program scope | PASS, `/tmp/prifly-issue9-storage-compiler-tests.log`, `/tmp/prifly-issue9-compiler-tests.log` |
| Installed-but-undeclared + mixed cycle | PASS, `/tmp/prifly-issue9-source-inventory.log` |
| Call/recovery/resume/reopen + glossary/index | PASS, `/tmp/prifly-issue9-runtime-matrix.log` |
| Legacy refusal regression | Initial check-fast обнаружил изменение code; исправлен boundary; `/tmp/prifly-issue9-legacy-green.log` PASS |
| Working check-fast | PASS, `/tmp/prifly-issue9-final-check-fast.log` |
| vet (linux/darwin CGO=0), schemas, refusal guard | PASS, `/tmp/prifly-issue9-boundary-gates.log` |
| Legacy public authoring corpus + workspace modes + catalog | PASS, `/tmp/prifly-issue9-authoring-verifier.log` |
| Source-built E5 public harness | **51/51 PASS**, `/tmp/prifly-issue9-final-harness.log` |
| Full ci-check-once, actual context `codex-app-escalated-bwrap-unavailable` | PASS (exit 0), `/tmp/prifly-issue9-code-gate.log`; final staged batch `/tmp/prifly-issue9-final-staged-gate.log`, accepted result in native Run |
| Strict OpenSpec all | **34/34 PASS**, `/tmp/prifly-issue9-openspec.log` |
| Protected historical/schema/workflow bytes | 1482 unchanged; only two current authoring manifests intentionally changed |
| Legacy source fixtures / fixed b1 vectors | 40 unchanged, `/tmp/prifly-issue9-legacy-hashes.json`; vector 858eca4b122e4bd822811bbfa78d0d5ffc1fad6ae3d8af8f5267df8d10108779 preserved |

Рабочий `test-changed` сравнивает с origin/main и поэтому затрагивает также
интегрированные #7/#8: фактически выполнены все Go packages и schemas. Полный
race/e2e не запускался: это release-only gate; явно предписанные public harness
и targeted scenarios выполнены отдельно.

## E5 traceability

| Приёмка | Наблюдаемое доказательство |
|---|---|
| A1 | marker/discriminator/profile/editor matrix, legacy vectors unchanged |
| A2 | Review wrapper с внутренним call: одна root invocation и native children в одном Run, exact caller/parent связи |
| A3 | Request→report/gate и configuration/ui/ux→gate/ui_report/ux_report/environment/release, второй inserted consumer читает каждый export |
| A4 | 14 public negative cases + unit inventory/profile/standalone tests; named Problems, ни Run dispatch, ни valid partial package |
| A5 | У обоих consumers все declared succeeded/partial/rejected проверены; partial/rejected не дают implicit success или exports |
| A6 | 7 public control cases: competing resource, scoped cancel, unknown effect, handled/unhandled error и timeout; settlement наблюдаем |
| A7 | 3 shared graph limits, 2 policy/effect refusals, explicit program allowlist/consent; synthetic 2 MiB storage ceiling не допускает child Attempt, сохраняя accepted producer и claim |
| A8 | 13 независимых freshness mutations (включая nested descendant, каждый из трёх inputs, declared/local resource и file/context); inherited prefix через два resumes, changed child refusal и legal whole-child replay |
| A9 | Root/sibling source hashes одинаковы; file rename не меняет compiled identities; legacy fixtures не переписаны |
| A10 | New authoring schemas, existing closed DTOs, runner text/update checks; synthetic task/take/submit в codex-app/codex-cli/claude-code |

Artifacts проверены public `artifact inspect`: exact schema, digest, producer
Run/Invocation/Attempt, accepted candidate. Ordinary step artifact не получает
выдуманную input lineage: `provenance` остаётся штатным empty/null, а actual input
refs закреплены в Attempt envelope и проходят downstream без пересказа.

Native cancel read-only leaf оставляет waiting/blocked_child, затем whole-Run
cancel даёт settled cancelled. Cancel taken workspace-write leaf честно даёт
recovery_required и recover_source_unsettled; controlled fixture host отдельно
подтверждает not_applied, после чего fixture settles. Это не доказательство
остановки внешнего browser actor. Source Run bytes и dirty candidate file сохранены
через resume/replay; существующая nested dirty tracked/untracked matrix также PASS.

## Exact source-built candidate

Binary: `/tmp/prifly-issue9-candidate`; SHA256 `sha256:4087c3e966837c508abb10a262a28344dbc057d5c54b6d693a4d12f34151082a`.
Linux/amd64, Go 1.27.2; development version `0.2.0-dev.5`.
Собран из изменённого дерева на baseline `00cbccac08329520271565d0d104fed672c43dcd` до implementation commit.
Это source-built synthetic evidence, не released CLI и не live host qualification.
Итоговый clean-commit binary и полный Git head будут переданы отдельно в host handoff;
проверенный candidate по этому пути не перезаписывается.

## Следующая приёмка

- 7.1: owner-approved release preparation на fixed main SHA, verify/qualify,
  stable publication и запуск того же harness exact released binary.
- 7.2: live host handoff/control evidence по каждому host; synthetic native CLI
  результаты выше не закрывают этот пункт и не исключают hosts из release scope.
- 7.3: свести released/live blockers и передать реальную released Core version W6;
  независимые consumer tests и canonical AI Factory packages в этом срезе не менялись.

Push/tag/release и operational recovery исходных пользовательских Runs не выполнялись.
