## Why

`aif-classic` сейчас передаёт между `aif-plan`, `aif-improve` и
`aif-implement` собственный JSON-пересказ плана. Это расходится с реальной
практикой AI Factory: skills должны последовательно читать и изменять один
нативный план. В Ultra это не один файл, а атомарный bundle: `index.md` и
связанные phase-файлы. У Pri-Fly уже есть immutable ArtifactRevision, но нет
контракта, который безопасно переносит такой набор артефактов в declared часть
claimed Workspace и принимает изменённый набор как следующую revision.

Это нужно исправить до живого `aif-classic` pilot, иначе пример создаёт
видимость канонического процесса, не исполняя его.

## What Changes

- Добавить generic декларацию Workspace artifact tree binding: запечатанный
  манифест с exact ArtifactRevision каждого файла materialize-ится в
  объявленную часть claimed Workspace, а captured tree после host handoff
  seal-ится как новый манифест и новые ArtifactRevision изменённых файлов.
- Сохранить strict provenance и integrity: путь не является артефактом,
  traversal/symlink escape и несовпадение bytes отказываются; найденный перед
  handoff drift не перезаписывается молча.
- Версионировать assisted-host task contract для объявленных tree bindings и
  добавить совместимое чтение прежних Runs.
- Перевести `aif-classic` с JSON `summary/tasks` на native AI Factory plan
  artifact set. Fast, Full и Ultra сохраняют исходные файлы и структуру;
  улучшенный set становится входом следующего круга и `aif-implement`.
- Для динамически названных Full/Ultra plan bundles использовать typed,
  confined capture location под declared parent; Core не читает и не
  интерпретирует конфигурацию AI Factory.
- Добавить запись этой high-priority работы в единый delivery backlog.

## Capabilities

### New Capabilities

_Нет._ Поведение расширяет существующие контракты исполнения, ресурсов,
assisted-host protocol и YAML authoring; отдельная продуктовая область не
создаётся.

### Modified Capabilities

- `domain-execution`: sealed WorkspaceTreeManifest получает declared роль
  immutable input/output набора файлов assisted workspace step.
- `runtime-resources`: runtime materialize-ит и захватывает declared tree
  внутри claimed Workspace с confinement, sealing и честным drift refusal.
- `workflow-and-context`: YAML authoring выражает generic tree bindings, а
  optional `aif-classic` использует нативные планы AI Factory всех трёх
  поддерживаемых форм.
- `cli-protocol`: versioned `SessionTask`/`SessionSubmission` contract
  сообщает host exact declared file bindings без прямой записи authority.
- `specification-governance`: словарь вводит канонические понятия
  WorkspaceTreeManifest и declared Workspace tree binding с их будущими
  Go/JSON соответствиями.
- `delivery-roadmap`: единый current backlog отражает эту активную
  high-priority работу и её границу.

## Impact

Меняются Go runtime, versioned session DTO и generated schemas, YAML compiler
и fixtures, optional `examples/aif-classic`, документация и targeted tests.
Новых внешних зависимостей, AI Factory runtime dependency в Core или изменений
прошлых sealed Runs не появляется. Манифест хранится как внутренний sealed JSON
ArtifactRevision в authority storage; AI Factory видит только восстановленные
обычные файлы.
