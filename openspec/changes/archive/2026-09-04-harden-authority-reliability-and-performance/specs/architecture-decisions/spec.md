Authoritative source set: `openspec/specs/architecture-decisions/spec.md`
(перенесено). Compatibility path: опубликованные версии state/read/next/
preview contracts сохраняют идентификаторы; меняется только способ их
вычисления в коде.

## ADDED Requirements

### Requirement: Версии state contract образуют один упорядоченный ряд

Реализация MUST вычислять совместимость Run со state, read, next, preview и
step-read contracts из одного порядкового ряда версий и одной таблицы
соответствий. Проверка «версия не ниже» MUST быть единой функцией, а не
цепочкой сравнений строк; выбор версии при создании Run MUST быть максимумом
требуемых рангов. Один и тот же state MUST давать одинаковый read/next/preview
contract во всех точках чтения. Опубликованные идентификаторы версий не
меняются.

#### Scenario: Добавлена новая версия state
- **WHEN** разработчик добавляет следующую версию contract
- **THEN** изменяется одна запись таблицы, а все точки чтения сообщают новую
  версию согласованно

### Requirement: Persistence layer не протекает в runtime

Runtime MUST различать сбой персистентности через exported predicate слоя
хранения, а не через импорт конкретного database driver. Runtime и CLI MUST
собираться без cgo для проверки этой границы, даже если выбранный driver в
release требует cgo. Замена driver MUST быть отдельным ADR с измерениями.

#### Scenario: Сборка без cgo
- **WHEN** `CGO_ENABLED=0 go vet ./...` выполняется в CI
- **THEN** runtime и CLI проходят vet, а отказ возникает только в самом
  storage driver
