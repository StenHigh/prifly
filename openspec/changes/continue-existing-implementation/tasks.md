## 1. Continuation provenance и Project CLI

- [x] 1.1 Добавить continuation entry point с существующим `ForkProvenance` и runtime reader exact source task/handoff/plan/последней accepted Implementation; разрешить declared internal refs только этому entry point, сохранив raw fork только для root outputs. Проверить eligible/ineligible source и отсутствие обязательного output; source snapshot не менять.
- [x] 1.2 Реализовать `project continue --source-run` с `--prepare`/review digest: проверить Git HEAD, ancestry source base и exact `changed_files`, seal-ить актуальную Implementation как imported input; вручную проверить на SMSPlace successful start и stale digest до claim/Run creation.
- [x] 1.3 Расширить read-only Run/monitor projection fork provenance с continuation reason; проверить, что новый Run виден как continuation, а не обычный fork, и historical Run сохраняет прежний status/outcome/events.
- [x] 1.4 Добавить переносимые CLI integration tests на dirty checkout, stale digest и claim binding первого read-only gate. `TestCLIContinuationFromUnmergedImplementation` создаёт partial source, запускает continuation от незамерженного SHA и проверяет эти отказы, сохранение source и защиту от повторного старта.
- [x] 1.5 Проверить `--implementation-head` для незамерженной реализации: exact commit и ancestry, review digest, materialized worktree от выбранного SHA, отказы для неверного SHA и checkout mode; пройти pilot read-only prepare без изменения source Run. `TestContinuationSelectsUnmergedImplementationCommit` и read-only pilot на partial `run:5c6fd74e…`: HEAD `542493c8691bdb634082b5910032e3833bb7f95a`, 291 changed files, review digest `sha256:3401cccade58b05dd1b6f9b88aec9c542383f53a82e04a9ad8db5f8e3c838c8f`; сокращённый SHA и checkout mode отказали до claim.

## 2. AI Factory continuation package

- [x] 2.1 В `prifly-aif-workflows` добавить generated `aif-classic-continuation` с tail `verify → fix → review → commit`, exact typed inputs и обновлёнными versions; проверить compile для declared hosts и отсутствие warmup/plan/improve/implement в compiled graph.
- [x] 2.2 Добавить derived profiled continuation folder и sync/version gates; проверить generator `--check`, folder/version tests и то, что normal `aif-classic` сохраняет прежний launch/input contract.
- [x] 2.3 Опубликовать package и проверить `project workflows add/update` по Git origin без перезаписи team `extend.yaml` или project subtree; отдельная запись каталога не нужна для прямого declared origin.

## 3. Runner и проектная интеграция

- [x] 3.1 Добавить generated `prifly-run` инструкцию continuation: host вызывает только `project continue`, после accepted report вновь читает `run next` и не использует `run fork`/ручной artifact JSON; проверить pinned runner bytes и regression assertion.
- [x] 3.2 Обновить SMSPlace на опубликованный continuation package, добавить declared launch и создать новый Run от `run:176bdd49611198acd06a0a5a96e161f22a6d79eb50c4addfdf0e36aea76db1ac`; source остаётся partial, новый Run выдал verify, а не implement.
- [ ] 3.3 Провести новый Run через реальные verify/review/commit Attempts до terminal quality outcome.

## 4. Проверка и поставка

- [x] 4.1 Выполнить targeted Go/JS/package tests, `make test`, `make build`, vet, schema/refusal gates, `openspec validate continue-existing-implementation --strict` и `git diff --check`; exact commits назвать в handoff, локальный report предыдущего Run не добавлять.
- [x] 4.2 После удаления trial package проверить его повторное exact восстановление тем же Engine и отказ с исходным кодом, не stale `missing_ref`.
