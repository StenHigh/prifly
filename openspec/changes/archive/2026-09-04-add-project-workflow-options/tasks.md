## 1. Authoring contract

- [x] 1.1 Добавить в glossary канонические термины Project workflow setting и optional feature; расширить YAML authoring schema/reference для `settings`, `exclude` и declared feature, проверив static authoring-contract check.
- [x] 1.2 Расширить strict parser `extend.yaml`, сохранив существующий `extensions` form; покрыть accepted и unknown/duplicate/contradictory option cases в `go test ./cmd/prifly`.

## 2. Compile-time options

- [x] 2.1 Применить settings только к declared project-scoped configuration defaults с existing schema validation; проверить успешную настройку и отказы неверного workflow/input/value через `go test ./cmd/prifly`.
- [x] 2.2 Реализовать declared boolean features и `exclude` как compile-time false setting без graph rewrite; проверить feature lookup, конфликты, validation и неизменность исходного component bytes в `go test ./cmd/prifly`.

## 3. AIF-cycle и независимое подтверждение

- [x] 3.1 Объявить improve, verify и review как optional features в AIF-cycle YAML с explicit enabled/disabled graph routes; проверить `prifly project compile` fixture, что disabled variant проходит от plan к implement и commit.
- [x] 3.2 Добавить author-visible positive/negative YAML corpus для settings/exclude и проверить его public compiler route без сети, AI Factory или authority mutation.

## 4. Приёмка

- [x] 4.1 Выполнить targeted Go tests, `make check`, `openspec validate add-project-workflow-options --strict` и `git diff --check`; отдельно убедиться, что `openspec/changes/archive/` и опубликованная history не изменены.
