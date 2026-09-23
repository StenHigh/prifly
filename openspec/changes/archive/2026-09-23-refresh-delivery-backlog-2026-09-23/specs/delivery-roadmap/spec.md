## MODIFIED Requirements

### Requirement: Текущая очередь отделена от future catalogue
Delivery plan SHALL хранить active priority, contributor-readiness work и post-RC queue отдельно от catalogue возможных workflow и дальних идей. Текущий backlog SHALL перечислять все неархивированные changes, показывать прогресс задач по их `tasks.md` и отличать полностью отмеченные changes, ожидающие архивации, от незавершённой реализации. Архивированные changes MUST NOT оставаться активными записями. Изменение future idea MUST NOT неявно менять committed release scope или runtime contract.

#### Scenario: Владелец сверяет очередь с changes
- **WHEN** reader открывает текущий backlog
- **THEN** он видит каждый неархивированный change с его фактическим статусом, а завершённые архивированные работы не принимаются за открытые задачи

#### Scenario: Команда выбирает contributor-ready работу
- **WHEN** team начинает следующий post-RC change
- **THEN** она берёт первую незавершённую работу из active backlog и не создаёт compatibility scope для unreleased source form

#### Scenario: В каталог добавлен новый workflow
- **WHEN** team добавляет возможный workflow или integration
- **THEN** он остаётся proposal с prerequisite и explicit authority boundary, а не появляется как supported scenario текущего release

### Requirement: Каталог решений Run остаётся в backlog до архивации
Единый current backlog MUST хранить `add-run-decision-catalog` до его
архивации. После отметки всех OpenSpec tasks запись MUST отличать завершение
задач от независимой квалификации release; следующий шаг — проверить evidence
и архивировать change. Она MUST отделять этот scope от
`assisted-model-profile-protocol`, upstream AI Factory compatibility и live
pilot qualification.

#### Scenario: Команда выбирает следующую работу
- **WHEN** владелец читает current delivery backlog
- **THEN** он видит завершённые задачи каталога решений и оставшуюся архивацию отдельно от upstream AI Factory work
