## 1. Контракт чтения

- [x] 1.1 `internal/runtime/model.go`, `versions.go`, `compatibility.go`: семейство версий 31 (`core-state/31`, `core-read/31`, `core-next/31`, `core-preview/31`, `core-step-read/31`) в таблице контрактов и в манифесте профиля. Проверка: `go test ./internal/runtime -run 'Versions|Compatib'`.
- [x] 1.2 `internal/runtime/engine.go`: `NextView.StageWork` (`assisted_session`|`program`|`control`, omitempty), заполняется только при `action: stage` из закреплённого шага стадии; недоступный план — поле опущено, чтение не отказывает. Проверка: новый тест — ассистируемая стадия даёт `assisted_session`, программная `program`, `finish`/`choice` `control`, и `run next` ничего не исполняет; `go test ./internal/runtime -run 'Next'`.
- [x] 1.3 Генератор и bundle: новый public bundle семейства 31, поле скрыто из /30 и старше; `scripts/check-schema.py` знает новую строку. Проверка: `make schemas && make schemas-check` — прежние bundle'ы байт в байт, новый совпал.

## 2. Документы и ворота

- [x] 2.1 `examples/troubleshooting.md`: в записи про `idle` назвать род работы и правило «программу гонит фоновый драйвер». Проверка: `python3 -B test/e2e/test_examples.py`, `openspec validate --all --strict`.
- [x] 2.2 Защищённая история не тронута: `git diff --stat openspec/changes/archive schemas/core/materialized-session.schema.json` пуст; `make ci-check`, `make e2e`, `make race` зелёные, счётчики записаны в change.
