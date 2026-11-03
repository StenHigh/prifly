# Приёмка issue №19

Source baseline: `3d8fd301bccddd18a5c2be752570918439eb1c5d`.
Snapshot contract: inline claim metadata, максимум 131072 bytes canonical UTF-8 JSON.
Source-built probe evidence: `native-probe-source.json`; released-Core gate пока открыт.

| Критерий issue | Проверка | Граница |
|---|---|---|
| Неизменённый workflow и закрытая task schema | `TestProjectOwnedNamingClosedWorkflowBeforeEffects`, generic probe и pinned official journey | Naming приходит только через отдельный flag; expected claim `demo-tracker-issue-19` |
| Missing/invalid/stale до effects | `TestProjectOwnedNamingRefusalsAndPathGuards`, `TestProjectOwnedNamingChangedAfterSummaryBeforePreflight`, `TestProjectOwnedNamingSnapshotOverflowBeforeStartEffects` | Queue reservation и executable preflight не происходят; prose ID не извлекается |
| Полный reviewed snapshot | `TestProjectOwnedNamingReviewFreshnessAndPersistedStart`, `TestProjectOwnedNamingNamespaceAndFlagRefusals` | Изменение unused field/schema/declaration даёт stale; whitespace canonical equivalent сохраняет digest; compiled refs не меняются |
| Update сохраняет consumer и upstream | Official journey, `aif-classic v1.57.0`, commit `ee76702dec0d8e0958ef068ef58074ecf2db37e9` | Explicit inert schema dependency; consumer profile/schema/extend/project files и upstream inventory сравниваются по bytes |
| Continue/resume/recover | `TestProjectOwnedNamingSourceSnapshotAfterPolicyRemoval`, `TestProjectOwnedNamingRecoveryFromTechnicalFailure` | Policy/schema/value удалены; handover сохраняет snapshot/path/branch; новый commit получает source base и suffix |
| Native CLI без AI host | `examples/workflows/project-owned-naming/probe.py EXACT_BINARY --official` | Synthetic reports только в generic fixture; official Run отменяется до выполнения host skill |
| Bounded persistence и idempotency | `TestNamingSnapshotAggregateByteBoundary`, `TestNamedSnapshotPersistenceAndSemanticIdempotency`, `TestProjectOwnedNamingRepeatIgnoresFlagOrder`, `legacy-replay-source.json`, `TestSnapshotClaimsUseBoundedStrictStateReader` | 131072 принимается, 131073 отказывается; duplicate не расходует имя; snapshot меняет command identity; новый reader >2 MiB остаётся strict |
| Queue + composition | `TestProjectOwnedNamingUnifiedCompositionAndQueue` | Полные 128 KiB проходят реальные summary/start `/8` и queue replay; новые published schemas принимают outputs |
| Checkout / без Workspace | `TestProjectOwnedNamingOnlyAppliesToNewWorktrees` | Values не требуются; явно переданные naming values получают отказ |
| Закрытые declarations и безопасные paths | `TestProjectOwnedNamingDeclarationClosure`, `TestProjectOwnedNamingReaderRefusesTraversalAndLinks` | До 16 inputs; mixed/unknown/unused declarations, traversal, symlink и remote refs отказываются |

## Совместимость

`contract-baseline.json` фиксирует 114 прежних tracked schema/index files.
112 файлов `*.schema.json` сохраняют точные bytes. Старые schema versions не расширены.
`schemas/authoring/manifest.json` — изменяемый индекс документов; он регистрирует новый `/5`.
Его embedded копия обновляется синхронно. Historical archive и release evidence не переписываются.
Новые публичные формы находятся в отдельных `project-owned-naming.schema.json` и `project-profile-v5.schema.json`.

## Released gate

Source-built binary не подтверждает совместимый опубликованный Core.
Пункт 5.4 остаётся открытым до выполнения probe и official journey на exact released executable.
Evidence должно назвать version, binary digest, release commit, commands, expected/actual claim и upstream integrity.
Issue №19 не закрывается этим source evidence.
