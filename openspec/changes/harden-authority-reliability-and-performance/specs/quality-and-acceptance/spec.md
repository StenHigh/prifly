Authoritative source set: `openspec/specs/quality-and-acceptance/spec.md`
(перенесено). Compatibility path: существующие gates сохраняются; добавляется
benchmark gate горячих путей.

## MODIFIED Requirements

### Requirement: Performance report сохраняет гарантии

Benchmark MUST публиковать hardware/software profile, workload, distribution,
durability/authorization settings и failures рядом с latency. Он MUST отличать
control transaction latency от queue, execution и external-effect wait.
Repository MUST содержать воспроизводимые benchmarks горячих путей authority
(открытие, чтение Run, admission, publication, telemetry на объявленном
потолке) на детерминированно генерируемых fixture БД объявленного размера; их
результаты фиксируются как evidence change, а регресс относительно
записанного baseline MUST быть замечен до release.

#### Scenario: Быстрый benchmark отключает durability

- **WHEN** измерение исключает обязательную persistence или redaction
- **THEN** результат не может объявляться production profile

#### Scenario: Горячий путь стал квадратичным

- **WHEN** benchmark на fixture БД удвоенного размера показывает рост latency
  быстрее объявленной сложности
- **THEN** release gate не проходит, пока причина не записана и не исправлена
