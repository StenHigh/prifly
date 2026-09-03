## Why

Pri-Fly — движок, но product workflows AI Factory лежат в его `examples/`, а
их Go/Python проверки и два нормативных требования привязывают репозиторий
движка к одному продукту. После `add-project-workflow-catalog` сценарий можно
получить из любого Git-репозитория через каталог, поэтому AI Factory packages
должны развиваться в собственном публичном репозитории, а Pri-Fly — хранить
только нейтральные authoring references и fixtures.

## What Changes

- Создать публичный workflow repository `StenHigh/prifly-aif-workflows`:
  папки `aif-classic/` и `aif-fanout/` в корне (имя папки остаётся именем
  установки, потому что `decision_catalog` ссылается на
  `.prifly/workflows/aif-classic/...`), README с установкой через каталог,
  требованиями к host skills и правилом версий, собственные проверки
  (статический контракт папок и black-box compile обоих packages через
  released Pri-Fly) и GitHub Actions; первый tag `v1.0.0`.
- Перевести записи каталога `StenHigh/prifly-workflows` на новый repository и
  tag с pinned commit.
- Удалить из Pri-Fly `examples/aif-classic`, `examples/aif-fanout`, Go-тест
  `TestCLIProjectCompileSealsAIFWorkflowFolders` и Python
  `AIFWorkflowFolderTest`; их проверки переезжают в новый repository.
  Engine-покрытие `project compile --package-profile` и отказ неизвестного
  profile остаётся нейтральным тестом Pri-Fly.
- **BREAKING** для спецификации: требования «AI Factory examples separate
  classic and fanout workflows» и «AI Factory classic preserves native Fast,
  Full and Ultra plans» удаляются из `workflow-and-context`; контракт этих
  packages принадлежит их repository. Добавляется требование, что Pri-Fly не
  поставляет product workflows.
- Roadmap: RC-описание ссылается на внешний repository; AIF-записи backlog
  (живой pilot, совместимость с released AI Factory package) переносятся в
  README нового repository; требование «Canonical AI Factory plan bridge
  остаётся active delivery work» удаляется, потому что change
  `add-workspace-artifact-bindings` завершён. Завершённые changes
  `align-aif-classic-and-fanout` и `add-workspace-artifact-bindings`
  архивируются до удаления требований, чтобы поздний sync не вернул их.
- Документация: `examples/README.md`, `test/README.md` и README указывают на
  каталог и внешний repository вместо локальных папок.

Runtime, binary, published contracts, YAML authoring contract и сохранённые
Runs не меняются: изменение затрагивает только содержимое репозитория,
тесты и спецификацию.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: удаляются два требования о AI Factory packages как
  части repository; добавляется требование, что product workflows живут во
  внешних Workflow repositories и проверяются там.
- `delivery-roadmap`: RC boundary и backlog ссылаются на внешний repository;
  требование про plan bridge как active work удаляется.

## Impact

Изменяются `examples/`, `cmd/prifly/main_test.go`, `test/e2e/test_examples.py`,
`examples/README.md`, `test/README.md`, `README.md`, OpenSpec specs и archive.
Внешние артефакты: новый репозиторий `StenHigh/prifly-aif-workflows` и
`catalog.yaml` в `StenHigh/prifly-workflows`. Go runtime, CLI, schemas и
published contracts не меняются; новых зависимостей нет.
