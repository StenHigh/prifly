## Context

См. `proposal.md`. Уже существует typed project configuration input:
`configuration: {scope: project, default: ...}`. AIF-cycle использует его для
четырёх независимых ограничителей. Напротив, `extend.yaml` сейчас принимает
только `extensions` и может вставить только no-input step на одном exact route.

## Goals / Non-Goals

**Goals:**

- Дать команде tracked и читаемый выбор project values и необязательных частей
  reusable workflow.
- Не позволить shorthand незаметно поменять stage, binding, route или уже
  начатый Run.
- Сделать AIF-cycle первым реальным потребителем без связи Pri-Fly с AI Factory.

**Non-Goals:**

- Не вводить JSON Patch, произвольный `override`, удаление stage, новый runtime
  operator, CLI command или отдельный configuration store.
- Не угадывать, как обходить часть workflow: полный graph остаётся YAML автора.

## Decisions

### `settings` повторно использует существующие project configuration inputs

`extend.yaml` получит читаемое group-by-workflow представление:

```yaml
settings:
  improve-batch:
    improve_round_limit: 2
```

Имя workflow — exact package-local name, уже используемый simple extension.
Compiler разрешает его только в workflow component этого package, находит input
с `configuration.scope: project`, проверяет value его existing JSON Schema и
подменяет только `configuration.default` перед обычной compile/validation.
Следовательно, не нужен второй конфигурационный язык или runtime channel.

### `exclude` — короткая запись для author-declared boolean feature

Workflow author явно объявляет feature рядом с graph:

```yaml
features:
  improve:
    input: improve_enabled
```

`input` обязан быть project-scoped configuration input, который принимает
boolean. Сам graph содержит choice и оба route: enabled выполняет часть,
disabled продолжает работу безопасным обходом. `extend.yaml` может написать
`exclude: [improve]`; compiler превращает это только в default `false` для
указанного input. Metadata `features` не попадает в sealed WorkflowRevision.

Так `exclude` остаётся удобным словом для команды, но не становится опасной
операцией удаления. Feature IDs уникальны в package; один ID указывает на один
declared input одного workflow component. Для более сложного набора автор
объявляет несколько features и перечисляет их в `exclude`.

Альтернатива — дать compiler `from/to` для вырезания stage — отклонена: для
call, repeat и bindings это скрытый graph-rewrite и невозможно надёжно вывести
нужный output. Альтернатива `override: /definition/...` отклонена по той же
причине.

### Один compile-time путь и строгие конфликты

Compiler сначала собирает declared feature metadata из всех workflow
components, затем накладывает `settings` и `exclude`, удаляет metadata и
выполняет прежние lowering, exact-ref, graph и package checks. Неизвестные
names, non-project input, non-boolean feature, invalid setting value,
duplicate feature и явный `settings`-конфликт с `exclude` — errors. Existing
`extensions` применяются как сейчас после options к уже определённому graph.

Schema, published reference и independent YAML corpus получают те же examples
и negative cases. Новый stable terminology добавляется в OpenSpec glossary.

## Risks / Trade-offs

- [Feature описан, но автор не сделал meaningful bypass] → compiler проверяет
  graph; AIF fixture дополнительно проходит route-level acceptance example.
- [Обновление package удалило used feature/input] → следующий compile откажет,
  а не проигнорирует team configuration; старый sealed package остаётся в
  history.
- [Нужна сложная перенастройка graph] → команда правит полный YAML graph, как
  и сейчас; shortcut намеренно не растёт в patch language.

## Migration Plan

1. Расширить authoring parser/schema и compiler, сохранив `extensions` form.
2. Объявить optional parts и configuration flags в AIF-cycle; вне AIF никаких
   dependency или special case не добавлять.
3. Добавить positive/negative corpus и targeted Go checks, затем relevant local
   gates. Rollback — убрать новый `extend.yaml` content: прежний workflow
   source и ранее sealed Runs остаются совместимыми.
