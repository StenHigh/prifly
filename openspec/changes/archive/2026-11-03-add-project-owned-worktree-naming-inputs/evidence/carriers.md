# Inventory контрактов issue №19

Исходный commit: `3d8fd301bccddd18a5c2be752570918439eb1c5d`.
Контрольные суммы прежних 114 файлов: `contract-baseline.json`.

| Carrier | Прежний контракт | Новый контракт |
|---|---|---|
| Project profile | /2–/4 | /5 |
| Questionnaire | /4 | /5 |
| Launch summary | /3–/7 | /8 |
| Start result | /3–/7 | /8 |
| Persisted claims | authority-claims/1–/4 | authority-claims/5 |
| Read claims | foundation-claims/1–/3 | foundation-claims/4 |
| Full claim in summary/start | WorktreeClaim без snapshot | новый closed carrier в /8 |
| Continuation/recovery source | reduced ContinuationClaim | без расширения |
| Queue intent | opaque summary/result bytes | без расширения; вложенные /8 |
| Run/state/session | старые формы | без расширения |

Composition и queue сочетаются с /8. Старые файлы schemas остаются неизменными.
