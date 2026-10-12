## MODIFIED Requirements

### Requirement: Release tag использует уже qualified source tree

Перед созданием protected release tag тот же source tree MUST пройти required
product gates: полный CI `verify` и `qualify` с race и полным e2e на точном
commit SHA этого tag. Локальное evidence, результат другого SHA, synthetic PR
merge либо название workflow без проверки его identity MUST NOT заменять эти
gates. Tag workflow MUST выполнять только owner-controlled сборку и
публикацию signed assets и MUST NOT повторять `verify` или `race`. Нативные
build/smoke проверки release assets, signing и environment approval сохраняются.

#### Scenario: Owner публикует проверенный tag

- **WHEN** owner создаёт protected semantic-version tag для source tree с успешными required gates
- **THEN** GitHub Actions запускает только release workflow без второго product test batch

#### Scenario: Новый commit содержит только документацию

- **WHEN** candidate SHA отличается от уже проверенного commit
- **THEN** требуются собственные успешные `verify` и `qualify`; совпадение Go исходников не переносит qualification между SHA

#### Scenario: Локальная проверка прошла

- **WHEN** локальный `ci-check` имеет пригодное evidence, но на candidate SHA отсутствует успешный CI gate
- **THEN** tag остаётся запрещён, пока соответствующий CI gate не выполнен

## ADDED Requirements

### Requirement: Повторная подготовка релиза использует существующие CI результаты

Подготовка релиза SHALL сначала проверять текущие `verify` и `qualify` для
точного candidate SHA в том же repository и доверенном workflow. При пригодном
успехе она MUST ссылаться на выполненный batch вместо нового dispatch; при
queued/in-progress запуске MUST наблюдать его без второго batch. Отсутствие
успеха MUST NOT становиться pass. Неуспешный исход MUST быть назван и
предложен для явного retry, без бесконечного автоматического цикла.
Одновременные запросы той же проверки MUST предотвращать параллельный
дорогой batch и повторно оценивать результат перед новым исполнением.
Принудительный повтор MUST оставаться явным и наблюдаемым.

#### Scenario: Повторная команда после успешного qualify

- **WHEN** на том же candidate SHA уже завершены полный `verify` и полный `qualify`
- **THEN** подготовка показывает их run URLs и не запускает их заново

#### Scenario: Qualify ещё исполняется

- **WHEN** есть подходящий активный `qualify` на точном SHA
- **THEN** подготовка наблюдает его; timeout не становится успехом и не порождает второй dispatch

#### Scenario: Два запроса пришли одновременно

- **WHEN** один и тот же обязательный batch запрошен дважды на точном SHA
- **THEN** дорогая проверка выполняется последовательно; второй запрос использует подтверждённый успех первого либо честно сохраняет незавершённый или неуспешный исход

#### Scenario: Workflow зелёный, но обязательные проверки пропущены

- **WHEN** GitHub показывает успешную оболочку, но ни она, ни указанный исходный запуск не подтверждает весь обязательный batch на точном SHA
- **THEN** этот результат не допускает tag; skipped/cancelled/unknown не превращаются в qualification

#### Scenario: Candidate изменился перед dispatch

- **WHEN** ref уже указывает на другой SHA, чем подготовленный candidate
- **THEN** подготовка не принимает результат другого commit и не создаёт tag по прежнему предположению

Source set остаётся `openspec/specs/release-distribution/spec.md`; delta
уточняет процесс до sync. Версии release manifests, signatures, public
контрактов и прежнее release evidence сохраняются.
