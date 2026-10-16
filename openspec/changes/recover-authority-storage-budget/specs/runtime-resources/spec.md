## ADDED Requirements

### Requirement: Authority хранит конечный storage budget как owner control
Authority SHALL иметь effective конечный soft limit allocated logical SQLite
pages, сохраняемый durable вместе с provenance/generation owner changes.
Default для обычной прежней authority SHALL оставаться 512 МиБ до явного
решения владельца; открытие новым binary MUST NOT автоматически повышать его.
Этот limit MUST NOT считаться Run policy budget, scan allowance, размером WAL
или гарантией свободного физического диска.

В первом срезе owner command SHALL только повышать effective limit в
поддержанном конечном range 64 КиБ…64 ГиБ. Unlimited, zero, overflow, снижение,
неизменное значение как новая mutation, target вне range или недостаточный
для текущего committed allocation MUST отказываться. Exact retry ранее
принятой команды SHALL возвращать recorded result идемпотентно.

Изменение SHALL проверять authority identity/epoch, reviewed payload и current
configuration generation атомарно, сохранять actor/reason/command evidence и
не создавать фиктивный Run. Малое control изменение SHALL оставаться
допустимым выше прежнего soft limit, если реальный storage способен его
зафиксировать. При ошибке записи MUST NOT объявляться успешно изменённый limit.

#### Scenario: Владелец повышает исчерпанный allowance
- **WHEN** владелец явно применяет reviewed повышение выше current allocation
- **THEN** finite новое значение и provenance сохраняются атомарно и переживают restart; existing Run/claim/evidence не переписываются

#### Scenario: Два изменения конкурируют
- **WHEN** два requests используют одну configuration generation и разные targets
- **THEN** один commit обновляет control, второй получает named stale conflict; он не заменяет новое значение по устаревшему review

### Requirement: Каждый admission проверяет актуальный persisted storage ceiling
Все existing storage-budget enforcement points SHALL использовать committed
effective limit из authoritative storage в своей write transaction. Уже
открытый store/driver MUST NOT продолжать применять stale process-local
значение после owner update. Legacy storage без нового setting SHALL
сохранять прежний effective default по declared compatibility path.

Повышение MUST NOT отключать ordinary admission checks или расширять
effects/trust/depth/step/capacity/claim permissions. Control/settlement reserve
SHALL сохранять прежний смысл и не финансировать новую работу обходным путём.
Новая работа, не помещающаяся в обновлённый budget, SHALL по-прежнему давать
`storage_budget_exhausted`. Disk-full, corruption, scan и per-snapshot failure
MUST оставаться самостоятельными причинами.

#### Scenario: Driver был открыт до изменения
- **WHEN** owner update закоммичен, а следующий admission исполняет ранее открытый driver
- **THEN** admission видит новое committed ceiling атомарно с проверкой allocation, без reopening для принятия setting

#### Scenario: Новый allowance снова исчерпан
- **WHEN** новая ordinary mutation превысит сохранённый ceiling
- **THEN** mutation отказывается штатно, а control/settlement остаются допустимыми в своей existing boundary

### Requirement: Budget recovery продолжает существующий Run без повтора accepted work
После successful budget update оператор SHALL иметь возможность повторить
ordinary `run drive` того же Run с его текущей lawful frontier. Setting
command MUST NOT самостоятельно dispatch-ить работу, менять Run outcome,
принимать unchecked outputs, сбрасывать pending acceptance или создавать
linked/fork Run. Existing unknown effects и Stops MUST продолжать блокировать
исполнение независимо от свободного budget.

Accepted artifacts, их provenance, claim generation, dirty/untracked Workspace
и source history SHALL сохраняться. Если нужного finite budget недостаточно
либо причина другая, recovery SHALL честно отказать, не удаляя evidence и не
предлагая manual authority substitution как эквивалент.

#### Scenario: Control frontier ждала storage
- **WHEN** Run с accepted fix и control frontier остановился по budget, затем owner явно повысил ceiling
- **THEN** ordinary drive продвигает тот же Run с этой границы, не переисполняя fix и не теряя его дерево/evidence

#### Scenario: Pending checks ещё не прошли
- **WHEN** quota оставила producer output pending acceptance, после чего ceiling повышен
- **THEN** drive выполняет ещё необходимые проверки, сохраняя единственное исполнение producer; setting не превращает pending result в accepted
