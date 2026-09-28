## ADDED Requirements

### Requirement: project continue возобновляет Run тем же launch
`project continue --source-run RUN` SHALL возобновлять исходный Run, если
выбранный launch запускает тот же workflow, что и исходный Run; с launch
другого workflow он MUST продолжать по объявлению `continuation`, как прежде.
`--from-stage STAGE` SHALL выбирать более раннюю завершённую стадию корневого
вызова как точку возобновления. `--prepare` MUST показывать точку
возобновления, перенесённые стадии и передаваемое дерево. `run next`
завершённого или отменённого Run MUST включать в `continuations` его
собственный workflow, если тот объявил `resumable` и допускает этот Run, и
тогда предлагать `project.continue`. Возобновление и `project recover` MUST
брать режим дерева из claim исходного Run, а не из `workspace:` launch или
`--workspace`. Если не передано ни одного ответа анкеты, профиля пакета или
политики, они MUST брать ответы, запечатанные в исходном Run; переданные
ответы MUST сверяться с исходными. Продолжению другим workflow ответы
исходного Run MUST NOT подставляться.

#### Scenario: Пилот продолжает свой Run тем же launch
- **WHEN** оператор вызывает `project continue --source-run RUN --launch L`,
  где L — launch, которым RUN был запущен, и его workflow объявил `resumable`
- **THEN** prepare показывает точку возобновления и перенос, а start создаёт
  возобновлённый Run в дереве исходного

#### Scenario: Точка возобновления указана явно
- **WHEN** оператор передаёт `--from-stage plan`
- **THEN** возобновлённый Run начинает с `plan`, и prepare это показывает

#### Scenario: Возобновление без workspace у launch и без ответов
- **WHEN** у launch нет `workspace:`, а оператор не передаёт ни `--workspace`,
  ни ответов анкеты
- **THEN** возобновление берёт режим дерева и ответы исходного Run и
  проходит; другой ответ даёт отказ `recover_context_changed`
