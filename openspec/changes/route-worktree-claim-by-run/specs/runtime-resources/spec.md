## ADDED Requirements

### Requirement: Неявный допуск claim выбирает записанного владельца Run
Project Start с выбранным Workspace claim MUST атомарно привязывать его к создаваемому Run. Когда `workspace_write` Attempt не называет claim явно, authority MUST сначала выбирать единственный активный claim, уже привязанный к этому Run. Активные claims других Run MUST NOT делать этот выбор неоднозначным. Сохранённый Project Run, начатый до этой привязки, MAY выбрать непривязанный claim только при совпадении производных Run/claim identities того же Project Start. Несколько активных claims этого Run или несколько активных непривязанных claims без такого доказательства MUST давать явный отказ до выдачи Attempt. Выбор и продление lease MUST сохранять прежнюю атомарную проверку поколения и владельца.

#### Scenario: Два Run одного репозитория продолжаются рядом
- **WHEN** два Run держат разные worktree claims одного репозитория и оба допускают следующий `workspace_write` шаг
- **THEN** каждый получает свой записанный claim независимо от активности другого Run

#### Scenario: Непривязанный Run видит несколько claims без доказанной связи
- **WHEN** Run не держит claim, а в authority есть несколько активных claims без точного совпадения Project Start identities
- **THEN** authority отказывает в неявном выборе и не выдаёт чужую рабочую копию

#### Scenario: Старый Project Run продолжает свой worktree
- **WHEN** сохранённый Project Run ещё не привязал claim, но его command identity точно выводит один активный Project claim
- **THEN** следующий admission закрепляет именно этот claim, не затрагивая worktree другого Run
