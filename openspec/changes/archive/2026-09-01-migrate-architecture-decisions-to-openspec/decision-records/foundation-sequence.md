# Инженерная основа локального sequence profile

## Контекст

Первый профиль должен быть самостоятельным локальным executor без hidden
network, model или background service, но с durable authority, sealed artifacts
и честным состоянием внешнего процесса.

## Решение

Reference core использует один Go module, local CLI/library, SQLite authority
journal и content-addressed file store. Router остаётся deterministic; model
может быть только declared worker/check. Closed DTO и versioned schemas не
расширяются неизвестными полями. Bounded schema helper, YAML validation,
canonical JSON и exact dependency locks изолируют authoring input от authority.

## Последствия

Commit authority facts и state выполняются одной transaction; blob seal
предшествует journal reference. Dispatch перед OS spawn оставляет uncertainty,
а не permission for blind retry. Local process получает explicit workspace,
bounded envelope и scoped channel, но cooperative host profile не обещает
изоляцию от владельца ОС.

## Пересмотр

Изменение core runtime, storage, trust profile или process boundary требует
versioned contract и measured qualification; документальная или schema check
сама по себе этого не доказывает.

## Не входит

Нет automatic retry, inter-run scheduler, arbitrary external write, mandatory
sandbox или distributed recovery claim.
