## ADDED Requirements

### Requirement: Project continuation и recovery принимают любой объявивший launch
`project continue` SHALL принимать любой declared launch, чей скомпилированный
workflow объявил продолжение, а `project recover` — любой launch по общим
правилам восстановления. Launch без объявления продолжения MUST отказывать
`project_continue_undeclared` до claim и Run, называя launch и недостающее
объявление. `--prepare` MUST показывать для каждого перенесённого входа его
источник в исходном Run и точный ArtifactRef, а также claim, который будет
передан, или указание на новый claim. `--workspace-commit` с полным
идентификатором commit MUST создавать новый claim от этого commit вместо
передачи; `project recover` его не принимает. CLI MUST NOT вычислять входы
из Git и MUST NOT читать содержимое артефактов за workflow. `run status`
MUST показывать последний принятый checkpoint Run.

#### Scenario: Launch другого пакета объявил продолжение
- **WHEN** оператор вызывает `project continue --prepare` для launch
  пакета, не связанного с AI Factory, и исходного Run допустимого workflow
- **THEN** prepare возвращает источники и refs всех перенесённых входов,
  передаваемый claim и review digest, а start с этим digest создаёт Run
  продолжения

#### Scenario: Launch без объявления
- **WHEN** оператор вызывает `project continue` для launch, workflow которого
  не объявил продолжение
- **THEN** CLI отказывает `project_continue_undeclared` без claim и Run

#### Scenario: Работа сделана на другом commit
- **WHEN** оператор передаёт `--workspace-commit` с полным существующим
  commit в режиме `worktree`
- **THEN** создаётся новый claim от этого commit, claim исходного Run
  остаётся за ним, а проверку связи commit с checkpoint делает первый шаг
  workflow продолжения
