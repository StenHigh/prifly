## Why

Текущий пример AI Factory смешивает каноничный последовательный путь автора AI
Factory с веером двух рецензентов и называет это одним «core cycle». Из-за этого
разработчик не может выбрать понятный классический сценарий, а Claude Code
искусственно исключён привязкой к одному каталогу skills.

## What Changes

- Добавить `aif-classic`: последовательный optional workflow по документированному
  пути AI Factory — plan, ограниченное улучшение плана, implement, verify,
  security, review и commit. Блокер quality gate предлагает `/aif-fix`, но
  сценарий не исправляет его автоматически.
- Сохранить bounded repeat для улучшения плана как явно названную policy
  Pri-Fly: следующая итерация получает исправленный plan; параллельных improve
  или review в `aif-classic` нет.
- Добавить отдельный `aif-fanout` package: он явно запускает независимые
  профили параллельно и сводит их результаты. Сейчас это компилируемый
  YAML-полигон будущего host-контракта; он не будет ложно заявлять, что уже
  переключает provider, model или reasoning effort.
- Вместо одного conflict-prone skills root объявить три host entry points:
  Codex CLI, Codex app и Claude Code. Каждый автоматически передаёт свой host
  compiler-у, который берёт skills только из соответствующего directory;
  workflow graph не дублируется.
- Обновить примеры, authoring corpus, schema и документацию так, чтобы
  классический и веерный сценарии были отдельными, а AIF оставался optional
  integration.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: project contexts получают host-selected skills root
  и два разделённых optional AI Factory workflow packages.
- `cli-protocol`: `project init` создаёт host entry points, а compilation
  принимает host identity, выбранную entry point.
- `specification-governance`: словарь описывает host-specific project skills
  roots, а не один обязательный Codex directory.
- `delivery-roadmap`: статус AIF pilot отражает каноничный classic workflow и
  отдельный fanout package, а отдельная high-priority задача описывает
  настоящее управление model profile на границе assisted host.

## Impact

Это изменение затрагивает Project compiler и его CLI, локальные authoring
schemas, YAML examples, их contract tests и OpenSpec. Core runtime, saved Run
contracts, модель provider-а и обязательность AI Factory не меняются.
