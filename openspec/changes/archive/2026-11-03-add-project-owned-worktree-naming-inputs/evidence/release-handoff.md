# Release handoff

Source implementation проходит локальные ворота в claimed worktree.
Exact candidate SHA фиксируется в host artifact `implementation.head_commit` и локальном commit, созданном каноническим workflow.
Сам этот файл не содержит собственного будущего commit SHA.

1. Владелец выбирает exact candidate SHA и выполняет обычный review/интеграцию.
2. Владелец получает зелёные GitHub `verify` и `qualify` на одном release SHA.
3. Владелец публикует совместимый Core по штатной процедуре проекта.
4. На exact released executable выполняется команда ниже.

```
local candidate SHA -> verify + qualify -> owner release
                                            |
                                 exact released executable
                                            |
                                  native issue19 probe
```

```sh
python3 scripts/ensure-release-checks.py --sha RELEASE_SHA --ref main --wait
python3 scripts/tag-release.py vX.Y.Z
python3 examples/workflows/project-owned-naming/probe.py /absolute/path/to/released/prifly \
  --official --evidence /tmp/issue19-released.json
```

Push, merge, tag и release требуют действий владельца. Локальные проверки не разрешают эти действия.
Сохраните release SHA, version и digest executable вместе с JSON evidence.
Только после этого отметьте task 5.4 и повторно оцените закрытие issue №19.
