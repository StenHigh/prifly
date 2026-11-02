[Индекс](index.md) · [Восстановление](recovery.md) · [Авторинг](authoring.md)

# Рабочие деревья, claims и effects

## Режимы

`worktree` создаёт отдельное Git tree и ветку Run. Несколько worktree Runs могут идти параллельно в одном repository.
`checkout` получает существующее дерево; другой claim этого дерева конфликтует.
Program-only workflow без Git work не требует claim или выбора workspace.
Передавайте режим из явного выбора или standing launch; не подменяйте его ради обхода отказа.

```text
repository -> claim -> worktree/checkout -> Attempt effects -> measured tree
```

Actual `workspace_path` из start и `repository_workspace` из task являются источником путей.
Project `/4` может выбирать читаемое имя по declared JSON/constant bindings.
Candidate из prepare не зарезервирован: actual suffix определяется first-free policy на старте.
Не переименовывайте directory/branch и не вычисляйте путь по хешу claim.
После Run ветку вливают обычным Git process; сам успех Run не означает merge или deploy.

## Permitted effects

| Effect | Что делать хосту |
|---|---|
| Только declared output slot | Читайте repository, пишите scratch и named outputs |
| `write_inside_claimed_workspace` | Меняйте только указанное claimed tree в рамках задания |
| `local_git_commit_on_claimed_workspace` | Коммит допустим в claim branch, если его требует задача |
| External-write declaration | Соблюдайте named system/operations/target и retry policy |

`repository_workspace: read_only` даёт материализованное дерево для чтения, но не workspace-write effect.
`effects.class:none` не разрешает тестам менять tracked files или HEAD.
Byproducts направляйте в scratch либо в уже допустимые ignored paths. Не расширяйте effects gate ради мусора сборки.
Движок сравнивает tree/HEAD и может отказать `effect_not_permitted` с именами путей.
Программы работают с OS правами пользователя; declared boundary не является OS sandbox.

## Workspace-tree artifact

Step может объявить `workspace_trees` с input/output ports и capture policy.
`exact_file` захватывает фиксированный файл. `direct_child_file` выбирает один файл внутри parent.
`direct_child_tree` выбирает один shallow bundle с declared entrypoint.
Input manifest materializes exact files до handoff. Output capture создаёт manifest при submission.
Materialize-only binding имеет input_port без output_port и требует StepDefinition v8.
Снятые bytes не заменяются live файлами другого worktree.

Report location находится top-level в submission, а ArtifactRef создаёт engine.
Форма показана в [hosting](hosting.md#workspace-tree-outputs).
Occupied producer capture path и drift materialized inputs сохраняют отдельные diagnostics.

## Ручные claim-команды

Обычный project start управляет claim сам. Ручные операции используйте только по допустимому recovery/owner действию.

```sh
"$PRIFLY_BIN" --project "$authority_root" claim list --json
"$PRIFLY_BIN" --project "$authority_root" doctor --claim "$CLAIM" --json
"$PRIFLY_BIN" --project "$authority_root" claim create --repository "$repository" --owner "$OWNER" --base "$REF" --json
"$PRIFLY_BIN" --project "$authority_root" claim create-set --repository DIR1 --repository DIR2 --owner "$OWNER" --json
"$PRIFLY_BIN" --project "$authority_root" claim heartbeat --id "$CLAIM" --generation "$GENERATION" --json
"$PRIFLY_BIN" --project "$authority_root" claim release --id "$CLAIM" --generation "$GENERATION" --json
```

Create-set допускает все ресурсы одной транзакцией либо ни одного.
Generation принадлежит текущему claim; не заменяйте её guessed числом.
Expired lease не передаёт владение автоматически и блокирует конфликтующее получение.
Heartbeat/release не используются как обход чужого claim.
Driver lifetime и ручной lease recovery описываются текущей диагностикой authority.

## Dirty tree и зависимости

Не выполняйте reset/clean для подготовки Run без явного решения сохранить или удалить bytes.
Worktree содержит tracked snapshot; gitignored toolchain/dependencies из другого checkout в нём могут отсутствовать.
Используйте абсолютный путь к проверенному toolchain или установите зависимости по процедуре проекта.
Symlink на чужую mutable dependency tree создаёт смесь состояний и не доказывает проверку нужного source snapshot.
Во время read-only stage установка зависимостей не должна менять claimed tree.

См. также [продолжение](recovery.md), [обслуживание](operations.md), [step reference](../authoring/step-authoring-reference.yaml).
