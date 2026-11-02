# Единая композиция и точки расширения

Пример требует capability `workflow_composition_with_points`. Он объединяет
parent `/4`, extension `/5` и public point export folder `/3`.

`setup.py` создаёт три исходника в явно выбранном проекте. Файлы имеют расширение
YAML и содержат совместимую JSON-запись. Справочник обычной YAML-записи находится
[здесь](../authoring/unified-composition-authoring-reference.yaml).

```text
classic /4: produce -> [before-commit] -> commit
                              |
                    quality: parallel all/wait
                         /             \
                   UI review          audit
                       |
              private UI body /3
                       |
             exported before-read point
                       |
             typed measurement Call /5
                       |
                 fresh carrier -> reader
```

1. Создайте отдельный проект и инициализируйте его через `prifly project init`.
2. Выполните `python3 examples/unified-composition/setup.py --repository /absolute/project` из репозитория Pri-Fly.
3. Выполните `prifly project compile --repository /absolute/project --package delivery --output /absolute/compiled --explain`.
4. Зафиксируйте проект в Git. Подготовьте launch `delivery` через `project questionnaire --prepare` с host `codex-app` и workspace `worktree`.
5. Проверьте summary. Запустите `project start` с полученным `--expected-launch-digest` и ведите Run через `run next`.

Установите runner для `codex-app`, если выбранный проект его ещё не содержит.
`project workflows` показывает launch `delivery`. Команда `setup.py` принимает
`--operator sequence|parallel|choice`; по умолчанию она выбирает `parallel`.

Classic владеет anchor, inputs, routes и пустым `consumers` точки before-commit.
UI library публикует только hook before-read своего private body. Delivery
выбирает measurement Call и явно связывает оба consumer ports с новым carrier.

Группа quality не передаёт outputs ветвей в commit. Commit получает исходный
carrier. Reader внутри UI получает новый carrier через объявленную точку.
Для общего schema contract все ports закрепляют внешний `core:schema/context-json@1.0.0`.
Реальный проект должен выбрать общую подходящую exact schema для своих данных.

Native минимум: Call v1, Parallel v2, typed projection v5, blocked v6,
resumable v8. Пример использует WorkflowRevision v8. Его авторские limits
учитывают generated wrappers; compiler их не увеличивает.

`project compile --explain` показывает owners, scopes, variants и exact refs.
Проверка `test/e2e/verify-unified-groups.py` исполняет пример через native CLI.
Она проверяет carrier с настоящим вложенным ArtifactRef, branch settlement,
отказы, technical failure, choice/default/unknown и отмену после результата audit.

Шаги возвращают synthetic replies. Имена UI, audit и commit не означают
настоящую проверку браузера, независимое одобрение или Git commit. Эти действия
требуют своих implementations, effects и отдельной приёмки потребителя.
