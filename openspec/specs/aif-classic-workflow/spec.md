## Purpose

Canonical AI Factory package направляет исправимые finding gate через
объявленный repair round, поэтому слабый host не может завершить Run одним
неудачно выбранным verdict.

## Requirements

### Requirement: Gate finding маршрутизируется по artifact, а не по неточному verdict host

В `aif-classic` verify и review MUST требовать пригодный `gate` artifact для
`pass` и `needs_revision`. Когда такой artifact доступен, оба verdict MUST
достигать одной declared decision: `blocking_owner_only=true` завершает
текущий bounded round без repair, `blocking=true` выдаёт `$aif-fix`, а иначе
implementation проходит дальше. Host MUST NOT сам запускать `$aif-fix`, gate,
commit или следующий stage.

#### Scenario: Verify называет finding needs_revision

- **WHEN** verify возвращает `needs_revision` вместе с gate, где
  `blocking=true` и `blocking_owner_only=false`
- **THEN** Run выдаёт declared `$aif-fix` Attempt и после его `pass` повторяет
  verify в пределах configured round limit, а не завершает root Run как
  `partial`

#### Scenario: Finding принадлежит owner

- **WHEN** verify или review возвращает gate с
  `blocking_owner_only=true`
- **THEN** Run не выдаёт `$aif-fix`, сохраняет gate как terminal `partial`
  evidence и не запускает последующие stages

### Requirement: Канонический пакет объясняет единственный control loop

Pinned bridge context и README `aif-classic` MUST называть `gate.blocking` и
`gate.blocking_owner_only` единственными данными, выбирающими repair route.
Они MUST объяснять, что `needs_revision` у gate с пригодным artifact
эквивалентен `pass` для маршрутизации, а `$aif-fix` всегда выдаётся Pri-Fly
отдельной Attempt и не запускается вручную из verify/review.

#### Scenario: Новый host читает pinned контекст verify

- **WHEN** host получает Attempt verify с finding, который исправим в claimed
  workspace
- **THEN** контекст предписывает вернуть gate с `blocking=true` и завершить
  только текущий Attempt, чтобы engine выдал `$aif-fix` следующим узлом

### Requirement: Host продвигает Run по observed action

Инструкция `prifly-run` MUST предписывать host после каждого accepted session
report снова читать `run next`. Для action `control` и `program` host MUST
вызывать `run drive`; для `assisted_session` он MUST исполнять только выданную
Attempt и submit-ить её result; для waiting или terminal action он MUST NOT
придумывать работу. Separate session MUST использоваться только когда host
действительно умеет её создать; иначе host выполняет Attempt сам и сообщает
фактическую unavailable provenance.

#### Scenario: Codex завершил verify Attempt

- **WHEN** Codex submit-ит результат verify и следующий observed action ведёт
  через control nodes к repairable gate finding
- **THEN** он drive-ит тот же Run, читает новый action и исполняет выданный
  `$aif-fix` Attempt вместо остановки или ручного запуска skill
