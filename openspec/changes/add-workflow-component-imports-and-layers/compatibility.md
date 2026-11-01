# Матрица контрактов

| Вход | Compile | Prepare | Provenance | Start |
|---|---|---|---|---|
| Legacy folder `/1`, unmarked extend, Project `/2` | `/1` | прежний legacy contract | отсутствует | прежний legacy contract |
| Folder `/1`, extension `/2`, Project `/3` или `/4` | `/2` | `/3`, `/4` или `/5` | `/1`, algorithm b1 | `/3`, `/4` или `/5` |
| Folder `/2` либо extension `/3`, Project `/3` или `/4` | `/3` | `/6` | `/2`, algorithm b1 с composition input | `/6` |

Имена views имеют прежние prefixes: `project-compile`, `project-launch-summary`,
`prifly-build-provenance`, `project-start`. Новый marker не расширяет старую schema.
Новый bundle `workflow-composition.schema.json` публикует отдельные closed contracts.
Старые b1 inputs не получают новых полей при нулевой composition.

`TestProjectBuildDeterminismAndDataBoundaries` сохраняет полный legacy b1 vector:
`sha256:858eca4b122e4bd822811bbfa78d0d5ffc1fad6ae3d8af8f5267df8d10108779`.
`protected-files.json` содержит исходные hashes файлов history и schemas.
Frozen gates сравнивают archived history и прежние `*.schema.json`;
текущие README и additive manifest metadata могут получать новые entries.

Негативные schema cases проверяют unknown keys и отдельный выбор нового marker.
Legacy `/2` продолжает отвергать root-input/qualified Call syntax.
Выпущенный `/4` сохраняет point contract v0.13.83. Неизвестный marker для negative cases — `/999`.
Composition явно отказывает на point sidecar и effective `/4`; library import без sidecar игнорирует standalone `/4`.

Release replay выполняется отдельно на exact опубликованном binary. Локальная
компиляция и synthetic report не закрывают release, browser или smsplace qualification.
