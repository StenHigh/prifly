# Архивная crosswalk: наблюдаемость, публикации и реакции

Это историческая карта переноса `docs/roadmap/state-and-telemetry.md`. Она
сохраняет прежние ключи, этапы, статусы и traceability; постоянная capability
их не использует как authoring interface. Наличие `passed` в legacy inventory
не является новой квалификацией, а `specified_not_executed` не означает
отмену нормы.

## Правила

| Historical rule | Постоянное требование |
|---|---|
| OBS-001 | Измерения принадлежат экземплярам исполнения |
| OBS-002 | Время сохраняет источник, единицу и качество |
| OBS-003 | Интервалы имеют разный смысл и не складываются молча |
| OBS-004 | Пробелы времени и поздние факты остаются видимыми |
| OBS-005 | Отчёт объясняет расход времени и исходы |
| OBS-006 | Экспорт не заменяет durable историю |
| OBS-007 | Read view имеет точный subject и согласованный cut |
| OBS-008 | Чтение не является управлением |
| OBS-009 | State, входы и progress остаются разными данными |
| OBS-010 | Новые read views не изменяют закрытые DTO |
| OBS-011 | Сигналы имеют provenance и completeness |
| OBS-012 | Outcome, error и warning независимы |
| OBS-013 | Каталог метрик задаёт арифметику |
| OBS-014 | Надёжность показывает полную population |
| OBS-015 | Ресурсы измеряются в квалифицированном scope |
| OBS-016 | Ядро измеряет собственную нагрузку |
| OBS-017 | Качество, work units и reuse имеют provenance |
| OBS-018 | Usage и charge не являются одним числом |
| OBS-019 | Шаг публикует метрику только объявленным mapping |
| OBS-020 | Historical query фиксирует cohort и метод |
| OBS-021 | Telemetry query остаётся закрытой read-only операцией |
| OBS-022 | Сбор ограничен, приватен и воспроизводим |
| OBS-023 | Анализ не меняет workflow сам по себе |
| OBS-024 | Телеметрия поставляется по profile phases |
| PUB-001 | Шаг объявляет immutable hooks contract |
| PUB-002 | State and event publication is fenced and idempotent |
| PUB-003 | Early artifact publication seals bytes before visibility |
| PUB-004 | Subscriptions bind declared producer to declared consumer |
| PUB-005 | Publication lifecycle preserves failure and backpressure truth |
| PUB-006 | Публичный protocol расширяется по фазам |
| REA-001 | Реакции исполняет один durable planner |
| REA-002 | Binding источника точен и fresh |
| REA-003 | Watch начинается без окна потери |
| REA-004 | Guard registration живёт дольше клиента |
| REA-005 | Guard predicate не исполняет произвольный код |
| REA-006 | Start guard gates existing graph work only |
| REA-007 | Stop guard creates durable restrictive control |
| REA-008 | Guard races preserve committed causality |
| REA-009 | Level, event wait and Trigger stay distinct |
| REA-010 | Waiting is finite and stable intervals are explicit |
| REA-011 | Worker observation does not create an orchestrator |
| REA-012 | Reactive load, privacy and lifetime are bounded |
| REA-013 | Decisions имеют historical explain и safe preview |
| REA-014 | Реакции включаются только по квалифицированным phase |

## Этапы правил

| Historical rule | Foundation stage | Completion stage |
|---|---|---|
| OBS-001 | P1-03 | P2-12 |
| OBS-002 | P1-03 | P1-07 |
| OBS-003 | P1-06 | P2-13 |
| OBS-004 | P1-07 | P2-13 |
| OBS-005 | P1-08 | P2-15 |
| OBS-006 | P1-08 | P2-16 |
| OBS-007 | P1-08 | P2-12 |
| OBS-008 | P1-08 | P2-12 |
| OBS-009 | P1-08 | P2-12 |
| OBS-010 | P1-03 | P2-12 |
| OBS-011 | P1-03 | P2-16 |
| OBS-012 | P1-06 | P2-14 |
| OBS-013 | P1-03 | P2-16 |
| OBS-014 | P1-06 | P2-14 |
| OBS-015 | P1-05 | P2-16 |
| OBS-016 | P1-03 | P2-16 |
| OBS-017 | P1-06 | P2-15 |
| OBS-018 | — | P2-16 |
| OBS-019 | P1-04 | P2-16 |
| OBS-020 | P1-08 | P2-15 |
| OBS-021 | P1-08 | P2-16 |
| OBS-022 | P1-03 | P2-16 |
| OBS-023 | P1-08 | P2-15 |
| OBS-024 | P1-09 | P2-18 |
| PUB-001 | P1-04 | P2-12 |
| PUB-002 | P1-06 | P2-13 |
| PUB-003 | — | P2-12 |
| PUB-004 | — | P2-12 |
| PUB-005 | — | P2-16 |
| PUB-006 | P1-08 | P2-16 |
| REA-001 | P1-03 | P2-12 |
| REA-002 | — | P2-12 |
| REA-003 | — | P2-12 |
| REA-004 | — | P2-16 |
| REA-005 | — | P2-12 |
| REA-006 | — | P2-12 |
| REA-007 | — | P2-14 |
| REA-008 | — | P2-12 |
| REA-009 | — | P2-12 |
| REA-010 | — | P2-12 |
| REA-011 | P1-08 | P2-12 |
| REA-012 | P1-08 | P2-16 |
| REA-013 | P1-08 | P2-15 |
| REA-014 | P1-04 | P2-12 |

## Приёмка

Точная инвентаризация 88 cases находится в
[`archive-acceptance-map.csv`](archive-acceptance-map.csv). В ней сохранены
original title, source line, owner stage, phase, gate, legacy requirement
links, verification kind, current execution status, Given/When/Then и evidence
references. Таблица правил выше однозначно связывает каждый legacy requirement
link этого inventory с постоянным subject heading.

| Family | Cases | Постоянная область |
|---|---:|---|
| OBS | 45 | Наблюдаемость и исторический анализ |
| PUB | 14 | Hooks и publication lifecycle |
| REA | 29 | Durable reactions и guards |
