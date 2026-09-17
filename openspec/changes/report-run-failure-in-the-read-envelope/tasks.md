## 1. Read contract

- [ ] 1.1 `internal/runtime/model.go`, `compatibility.go`: `core-read/30`, `RunView.Failure` (`code`, `diagnostic_id`, `attempt_id`, `step_instance_id`; omitempty) в допустимых read versions; `View` заполняет его из diagnostics только для `failed`/`cancelled`. Проверка: тест — Run, остановленный `effect_not_permitted` программы, читается с `failure.code`; `completed` Run — без поля; `go test ./internal/runtime -run 'View|Failure'`.
- [ ] 1.2 Public bundle нового read contract сгенерирован (`cmd/schema-gen`, `scripts/check-schema.py`); bundle /29 байт в байт прежний. Проверка: `make schemas && make schemas-check`; `git diff --stat schemas/core` называет только новый файл.

## 2. Runner text

- [ ] 2.1 `cmd/prifly/project.go`: текущий текст называет `run.attempts[].id` (SessionTask — `attempt_id`); прежний текст заморожен как `projectRunnerSkillTemplateBefore…`; `project runners update` распознаёт и заменяет его. Проверка: `go test ./cmd/prifly -run 'Runner'` — старый runner из 0.13.29 заменяется, кастомизированный отказывается.

## 3. Документы и ворота

- [ ] 3.1 `examples/troubleshooting.md`: «Run `failed`, `outcome` null» → `failure` с `core-read/30`, на старом бинаре — `diagnostics[]`. Проверка: `openspec validate --all --strict`.
- [ ] 3.2 Защищённая история не тронута: `git diff --stat openspec/changes/archive schemas/core/effects-session.schema.json` пуст; `make ci-check`, `make e2e`, `make race` в фоне зелёные; счётчики записаны в change.
