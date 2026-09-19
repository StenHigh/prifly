## 1. Откат

- [x] 1.1 `internal/runtime/workspace_trees.go`: `prepareWorkspaceTrees`
  больше не закрывает root раньше отката. Возвращает `func(bool) []string` —
  «освободить, и откатить, если шаг не допущен»: root закрывается в обоих
  исходах, поэтому дескриптор не течёт на допущенном шаге. Внутренние ранние
  возвраты закрывают root сами через `prepared`-флаг.

- [x] 1.2 Откат перестал отбрасывать ошибки: `materializeWorkspaceTree`
  возвращает пути, которые не смог снять. `os.ErrNotExist` — не остаток
  (снимать нечего), `syscall.ENOTEMPTY` на каталоге — не остаток и не ошибка:
  каталог, в который владелец положил своё, не наш, чтобы его опустошать.

- [x] 1.3 `internal/runtime/driver.go`: `admit` получил именованный возврат,
  освобождение вызывается в обоих исходах, а непустой остаток присоединяется
  к отказу через `errors.Join`. Код отказа остаётся прежним — причиной, по
  которой шаг не допущен: `ProblemFor` читает первое совпадение в цепочке.
  Диагностика для этого не годится — она живёт в состоянии Run, которое
  отказанная команда не пишет.

## 2. Проверка

- [x] 2.1 `TestARefusedAdmissionTakesBackWhatPreparationPlaced`: read-only шаг
  с `materialize_only` деревом, подготовка прошла, допуск отказан активной
  остановкой проекта. Красный до правки: «a refused admission left
  .ai-factory/PLAN.md in the owner's working folder». Зелёный после.

- [x] 2.2 Тот же тест держит вторую половину требования: файл владельца
  `.ai-factory/NOTES.md`, положенный в тот же каталог до отказа, остаётся на
  месте. Без этого откат «чинился» бы удалением лишнего.

## 3. Документы и ворота

- [ ] 3.1 Delta-spec `runtime-resources` синхронизирован в основной spec при
  архивации.

- [x] 3.2 Ворота зелёные: `make ci-check` rc=0 — vet прочитан на linux и
  darwin, fmt-check 301 файл, refusal-check 162 файла, staticcheck 9 пакетов ×
  2 платформы, 57 бандлов схем сошлись; `make e2e` rc=0; `make race` rc=0 —
  `cmd/prifly` 179.9 с, `internal/runtime` 723.0 с, гонок нет.
  `openspec validate workspace-tree-preparation-rollback --strict` — valid.
