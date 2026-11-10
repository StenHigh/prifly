## Context

См. proposal.md. Scalar substitution уже работает до schema compilation. Compiled variants b1/b2 уже обновляют owned refs. Schema diagnostic уже считает Unicode code points и показывает pointer, limit и actual length.

## Goals / Non-Goals

**Goals:** единый narrow путь от package declaration и owner value до immutable compiled schema и reviewed launch.

**Non-Goals:** произвольные schema patches, новые runtime operators, миграция старых Runs, выпуск downstream classic и закрытие workflows issue #3.

## Decisions

1. Используем самостоятельные sidecars `/1`. Это сохраняет текущие markers folder/extension и старые public schema bytes.
2. `compile-parameters.yaml` содержит `parameters.NAME: {type: integer, minimum: 1, maximum: 1000000, default: 12000, bindings: [{schema: schema_task, pointer: /properties/description/maxLength}]}`. Binding допускает только maxLength объекта schema с type string. Поле содержит `{{NAME}}`; compiler проверяет exact target и не разрешает менять другие keywords.
3. Owner file `project/compile-values.yaml` содержит marker и `values: {task_description_limit: 32000}`. Существующая update boundary сохраняет каталог project/. Никакая команда не переписывает upstream-owned файл.
4. Resolver читает declaration и owner values отдельно для каждого source owner до flattening composition. Он использует существующий whole-scalar substitutor для разрешённых полей. Неизвестные placeholders и bindings получают отказ. Namespace параметра ограничен его source; одинаковые имена разных packages не смешиваются.
5. Declarations и owner bytes входят в source inputs composition. Effective rendered schemas входят в b1/b2 identity. Prepare/start компилируют тот же source и используют existing stale-review check. Для profile /2 capability явно запрещена.
6. Compile --value сохраняет собственный legacy смысл. Он не заменяет owner sidecar и не может молча переопределять уже разрешённый declared parameter.
7. В summary effective contract уже представлен exact package/build refs. Новых полей в старые closed DTO не добавляем.

```text
package sidecar + project sidecar
  → validate integer/range/binding → scalar substitution
  → compile whole closure → reviewed summary → immutable Run
```

## Risks / Trade-offs

- Импорт и наследование могут потерять owner scope → применяем bindings до alias rewrite и проверяем два пакета с одинаковыми именами параметров.
- Source bytes могут измениться после чтения → используем прочитанные document values и existing final launch freshness check.
- Update может сузить диапазон → сохраняем owner bytes и отказываем следующей compilation с причиной.
- Новые файлы могут быть symlinks → читаем через confined source reader и ограничиваем размер.

## Migration Plan

Пакет opt-in добавляет sidecar и placeholder в существующий schema keyword. Проект создаёт owner sidecar при необходимости. Без sidecars bytes и поведение compilation сохраняются. Rollback удаляет opt-in sidecars и возвращает исходный literal schema limit; прежние Runs остаются закреплёнными.
