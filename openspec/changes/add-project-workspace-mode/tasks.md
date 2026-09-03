## 1. Workspace contract и совместимость

- [x] 1.1 Расширить lifecycle claim режимами `worktree` и `checkout`, сохранив legacy raw claim commands worktree-only; проверить `go test ./internal/runtime -run 'TestWorktree|TestWorkspace' -count=1`, включая clean-checkout, physical conflict и отсутствие Git mutation.
- [x] 1.2 Закрепить exact Workspace claim в новом Run/session state и разделить authority scratch Attempt.Workspace от repository workspace host; проверить runtime acceptance test, что direct checkout не получает context/output files и workspace-write handoff не может адресовать иной claim.
- [x] 1.3 Опубликовать compatible state/read and action-intent schema boundary для `write_inside_claimed_workspace`, сохранив historical worktree-only forms; сгенерировать artifacts через `make schemas` и проверить `make schemas-check`.
- [x] 1.4 Обновить словарь и binding map для Workspace mode/claim без изменения historical JSON names; проверить `go test ./internal/runtime -run '^TestGlossaryBindings$' -count=1`.

## 2. Запуск Project workflow

- [x] 2.1 Выделить read-only preflight declared Project launch: exact profile launch, host, RunBrief, typed inputs и YAML compilation; проверить CLI test, что invalid launch/host/input/workspace не создаёт package, claim или Run.
- [x] 2.2 Реализовать typed `prifly project start` с default `--workspace worktree`, explicit `checkout`, bounded rollback и honest incomplete diagnostic; проверить CLI integration test, что оба режима возвращают Run/Workspace identities, а checkout отказывает на dirty repository.
- [x] 2.3 Передать selected repository workspace только workspace-write assisted tasks и обновить три generated `prifly-run` host skills; проверить fixture, что host без выбора Workspace ждёт и не создаёт Run, а stated choice запускает handoff без automatic provider/model launch.
- [x] 2.4 Обновить CLI help, authoring references и project documentation; проверить `python3 -B test/e2e/verify-authoring.py --binary bin/prifly` после build.

## 3. Сквозная проверка и границы

- [x] 3.1 Добавить author-visible positive/negative Project fixture для worktree/checkout launch path; проверить `python3 -B test/e2e/verify-authoring.py --binary bin/prifly` и отсутствие network/AI Factory requirement.
- [x] 3.2 Проверить old worktree claim/session fixtures and archived OpenSpec evidence against new reader; выполнить targeted compatibility tests и `git diff --exit-code -- openspec/changes/archive`.
- [x] 3.3 Выполнить product gates `make ci-check`, `make e2e`, `openspec validate add-project-workspace-mode --strict` и `openspec validate --specs`; зафиксировать фактические результаты перед отдельным решением о release version.
