# Evidence — add-input-only-workspace-tree-binding

Ворота на финальном дереве, 2026-09-17 (движок, Apple M1, go1.27.0):

- `make ci-check` exit 0: `cmd/prifly` 41.7 с, `internal/flow` 6.3 с,
  `internal/runtime` 240.8 с; fmt-check 290 файлов; refusal-check 159 файлов;
  staticcheck 9 пакетов, находок нет; vuln-check 9 пакетов, уязвимостей нет;
  schemas-check — все публичные bundle'ы совпали, в том числе новые
  `materialized-session` (233 424 байта, `0231a0b7…`) и `step-definition-v8`
  (9 911 байт, `c22c3f81…`); `effects-session` (/29) и `step-definition-v7`
  байт в байт прежние.
- `make e2e` exit 0: 6 наборов `passed`.
- `make race` exit 0: `cmd/prifly` 171.3 с, `internal/runtime` 697.0 с, гонок нет.
- `openspec validate --all --strict`: 22 passed, 0 failed.

Что измерили тесты, которых не было:
- `TestMaterializeOnlyTreeHandsAReadOnlyStepTheCapturedPlan` (fast/ultra ×
  «файл на месте / файла нет»): read-only шаг получает `repository_workspace`
  claim того же Run, binding без `output_port`, `materialized_entries` ровно
  то, что положил движок (для `exact_file` — всегда, потому что захват
  освобождает файл; для bundle — только если его нет), guide
  `workspace-tree-guide/2`, отчёт без outputs принят, после settle снято только
  положенное, пустой родитель — тоже.
- `TestMaterializeOnlyTreeRefusesAHostThatEditsIt`: правка materialized entry —
  `effect_not_permitted` с путём (отпечаток `git status` содержимого untracked
  не видит, байты сверяются отдельно); восстановление — отчёт принят.
- `TestMaterializeOnlyWorkspaceTreeAuthoringAndValidation` (flow): v7 отказывает
  binding'у без `output_port`, v8 принимает на `effects: none`, отказывает на
  `workspace_write`, отказывает смеси форм; lowering из `prifly-step/1` даёт v8.
- `TestGuideShowsAMaterializeOnlyPortWithoutAnOutput`: порт без `output_port`
  на проводе, note называет правило.

Не измерено здесь: живой заход на настоящем хосте (стенд пакетчика — задача 3.4).
