# Совместимость расширенного core workflow

## Контекст

Нужно добавить typed graph, configuration и explicit error routes без смены
смысла historical Runs и без неявной DSL для будущих operators.

## Решение

Новый semantics profile и state/read views вводятся отдельными versioned
contracts. Он исполняет finite `step`/`finish` graph с typed bindings, declared
outcomes и explicit technical-error consumption. Project и run configuration
имеют declared input ports, deterministic precedence и pinned effective values.

## Последствия

Historical profiles, baseline DTO и stored runs остаются byte-stable. Missing
route, invalid projection и preparation failure дают durable diagnostic rather
than fabricated success or invisible retry. Preview resolves only declared
inputs and cannot become execution.

## Пересмотр

Новый operator, profile property, projection semantics или configuration source
нуждаются в новой versioned contract и compatibility decision.

## Не входит

Нет arbitrary expression language, template execution, implicit type coercion
или policy override через run input.
