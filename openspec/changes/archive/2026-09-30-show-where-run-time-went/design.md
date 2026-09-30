## Context

Калькулятор времени (`internal/runtime/timing.go`) чист и строит дерево по
Run. Для assisted-Attempt `Started` не наблюдается, поэтому `executor_time` и
rollup `executor_sum` у них unavailable. В Run уже записаны
`attempt.admitted`, `session.handed`, `session.reported`, `candidate_at`,
`settled`; у записей журнала решений — только `observed` (момент ответа).
Требование «Измеренная величина называет, что она измеряет» уже запрещает
потребителю вычитать наблюдения сам — поэтому интервалы считает движок, а
монитор только рисует.

## Goals / Non-Goals

**Goals:** работа агента, ожидания и простой видны числом с качеством; свод по
категориям и по стадиям.

**Non-Goals:** стоимость в деньгах/токенах (отдельные reported costs);
изменение прежних метрик; профилирование внутри сессии агента.

## Decisions

1. **Новые метрики, а не переопределение `executor_time`.** Опубликованные
   числа не меняются; `executor_time` у assisted остаётся unavailable с
   прежней причиной, а рядом появляются `host_pickup`, `host_work`,
   `decision_wait`. Ревизия калькулятора `core-timing/4` для Run состояния
   context и новее.
2. **Границы:** `session.handed` ставится в момент выдачи конверта, а не когда
   host его взял, и повторная выдача его сдвигает; чтение задачи ничего не
   пишет. Поэтому момент взятия записывает отдельная команда `session take`
   (первое взятие, `session.taken`). `host_pickup` = `admitted` → `taken`;
   `host_session` = `admitted` → `reported`; `host_work` = (`taken` или, без
   него, `admitted`) → `reported` минус `decision_wait`; `decision_wait` = Σ
   (`requested` → `observed`) записей журнала с `attempt_id`.
3. **`requested` на DecisionRecord** пишется в Run state 41 (та же версия, что
   вводит `record-questions-a-step-answered`; изменения выходят вместе).
   Раньше — unavailable с причиной `decision_request_time_not_recorded`.
4. **Свод — метрики, не новое поле.** `TimingNode` в опубликованных bundles
   закрыт (`additionalProperties: false`), а `metrics` — открытая карта: суммы
   `host_pickup_sum`, `host_session_sum`, `host_work_sum`, `decision_wait_sum`
   на узлах выше попытки и `idle` на корне. `idle` — длина пробелов между
   объединёнными интервалами попыток и проверок по UTC authority, всегда
   `estimated`. Телеметрия берёт только имена из закрытого каталога, поэтому
   новые метрики в неё не протекают.
5. **Монитор** рисует `breakdown` и таблицу стадий из дерева времени
   (rollup по `stage_id` c числом activations); фазы Attempt — из метрик его
   узла. Никакой арифметики над сырыми наблюдениями в JS.

## Risks / Trade-offs

- [Часы разных CLI-вызовов несравнимы] → интервалы — «оценка по часам
  authority», как уже делает ревизия 3; качество показано рядом.
- [Параллельные ветви] → сумма категорий больше elapsed; это явно сказано.
- [Общая state 41 с другим change] → оба change применяются и выпускаются
  вместе; тест достижимости версии покрывает обе причины.
