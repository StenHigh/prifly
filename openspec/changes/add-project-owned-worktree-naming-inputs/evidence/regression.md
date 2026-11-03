# Отказная regression

`go test ./cmd/prifly -run '^TestProjectOwnedNamingClosedWorkflowBeforeEffects$' -count=1`
До реализации: FAIL. Получен `project_profile_invalid` вместо `project_worktree_input_missing`.
Fixture сохраняет исходный workflow и закрытый JSON input. Prepare проверяет отсутствие claim.

Вторая regression: `TestProjectOwnedNamingRepeatIgnoresFlagOrder`.
До исправления: перестановка двух `--naming-input` дала `command_conflict`.
После исправления: повтор возвращает исходный результат. Naming namespace
входит в fingerprint как map; пустой namespace не меняет legacy fingerprint.
Legacy replay между baseline и candidate подтверждён отдельно в `legacy-replay-source.json`.
