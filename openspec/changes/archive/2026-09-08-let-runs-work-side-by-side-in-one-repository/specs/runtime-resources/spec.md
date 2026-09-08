Authoritative source set: `openspec/specs/runtime-resources/spec.md`.

## MODIFIED Requirements

### Requirement: Workspace claim сохраняет exclusivity в обоих Git режимах

Authority MUST distinguish a disposable `worktree` Workspace from a direct
`checkout` Workspace while using the same canonical physical repository
identity, owner and generation rules. A `worktree` claim MAY create and later
clean up only its own confined directory. A `checkout` claim MUST refer to the
current canonical Git checkout and MUST NOT create, delete, switch branch,
reset or clean that checkout.

Конфликт claim MUST определяться **рабочим деревом**, которое claim занимает, а
не репозиторием, которому дерево принадлежит. Два `worktree` claim одного
репозитория по разным путям MUST допускаться одновременно: они делят объекты и
ссылки, которые git сериализует сам, и не делят дерево.

`checkout` claim MUST исключать и MUST быть исключён любым другим активным
claim того же репозитория: он работает в собственном дереве репозитория, где
движок не ограничивает радиус последствий.

Отказ по конфликту MUST называть отношение, а не только препятствие: читатель,
стоящий в собственном worktree, видит другой путь и другую ветку и без этого
заключает, что отказ адресован не ему.

Идентичность занятого каталога MUST проверяться величиной, переживающей
перезапуск машины. Номер тома MUST NOT решать: он перенумеровывается загрузкой,
и сравнение по нему запирало репозиторий навсегда — документированный выход
проверялся тем же сравнением и отвергался им.

#### Scenario: Checkout mode leaves Git topology unchanged
- **WHEN** authority admits a direct checkout Workspace
- **THEN** no Git worktree is added and no branch, HEAD or tracked file is
  changed by admission itself

#### Scenario: Две задачи одной ветки идут рядом
- **WHEN** два Run берут по своему `worktree` одного репозитория от одной базы
- **THEN** оба claim выдаются, каждый со своим путём и своей веткой, и первое
  дерево переживает выдачу второго

#### Scenario: Existing Workspace is held
- **WHEN** another Run requests a Workspace that would occupy a tree an active
  claim already holds — a second `checkout` of that repository, or a `checkout`
  while any claim of it is active
- **THEN** authority rejects the conflicting admission without creating a
  directory or changing the checkout

Прежняя формулировка этого сценария отвергала **любой** второй claim того же
физического репозитория. Она заменена намеренно: два worktree одного
репозитория занимают разные деревья, и отказ им стоил продукту параллельной
работы.

#### Scenario: Claim переживает перезагрузку машины
- **WHEN** claim, созданный до перезапуска, проверяется после него
- **THEN** каталог опознаётся, работа допускается, и освобождение claim не
  отвергается собственной проверкой идентичности
