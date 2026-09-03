## ADDED Requirements

### Requirement: Установка сценария из репозитория не исполняет контент и не расширяет trust
`project workflows add`, `update` и `search` MUST вызывать `git` типизированным
argv без shell, с ограниченным окружением, отключённым terminal prompt и
разрешёнными только протоколами `https`, `ssh` и `file`; они MUST NOT
инициализировать submodules, исполнять hooks, filters или любой файл из
полученного repository. В `.prifly/` копируются только regular files по
`lstat`. URL с userinfo MUST быть отказом до сети, а diagnostics MUST NOT
содержать credentials: аутентификация приходит только из credential helper
или SSH пользователя. Полученные bytes остаются данными: команды MUST NOT
seal-ить, импортировать, доверять или компилировать package против host;
единственное trust decision по-прежнему принимается при `project start`.
Каталог не является trust root, а его необязательный `commit` только
проверяет identity. Сетевые операции MUST иметь ограниченный timeout и
выполняться только внутри этих явных команд.

#### Scenario: URL содержит token
- **WHEN** пользователь передаёт repository вида `https://user:token@host/…`
- **THEN** команда отказывает до сети и не записывает credentials в
  `project.yaml` или diagnostic

#### Scenario: Repository содержит hooks и submodules
- **WHEN** установленный repository объявляет Git hooks, filters или
  submodules
- **THEN** ничего из них не исполняется и не инициализируется, а копируются
  только regular files выбранной папки

#### Scenario: Запрещённый протокол
- **WHEN** SOURCE использует `ext::` или иной неразрешённый Git transport
- **THEN** команда отказывает без выполнения внешней команды
