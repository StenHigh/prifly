## MODIFIED Requirements

### Requirement: Project launch является единственной исполнимой точкой входа

Public Project launch MUST принимать exact ID объявленного `workflow` launch,
explicit host и typed значения только его объявленных input ports. Он MUST
compile, seal и зарегистрировать exact package before creating Run; source YAML,
host skill bytes и effective inputs MUST become pinned Run inputs. Launch MUST
not выбирать сценарий по тексту задачи, default launch или наличию файлов.
Interactive project host MUST require an explicit `worktree` or `checkout`
selection before it invokes the launch; absence of that answer is a wait, not
a fallback to a different Workspace. Когда host предоставляет native question
tool, этот конечный Workspace decision MUST быть показан через него, а не
текстом, имитирующим кнопки. Если количество допустимых вариантов любого
конечного developer decision превышает лимит tool, host MUST показывать все
варианты последовательными страницами и сохранять возможность отказаться;
он MUST NOT скрывать варианты или переходить к default. RunBrief, file path и
произвольное typed input value остаются обычным вводом, так как не являются
заранее известным конечным набором вариантов. A non-interactive CLI invocation
MAY use the declared command default. Запуск Project workflow не запускает
model/provider и не даёт host новых полномочий: assisted handoff остаётся
отдельным existing contract.

#### Scenario: Объявленный launch запускается
- **WHEN** пользователь называет существующий launch, declared host и все
  required inputs
- **THEN** система создаёт Run только из sealed revision этого launch и
  возвращает его identity вместе с выбранным workspace

#### Scenario: Launch не объявлен
- **WHEN** пользователь называет отсутствующий или не-workflow launch
- **THEN** система отказывает до compilation, package registration, claim или
  Run creation

#### Scenario: Host не получил выбор Workspace
- **WHEN** пользователь выбрал launch в диалоге, но не назвал worktree или
  checkout
- **THEN** host задаёт этот единственный вопрос через native tool, если он
  предоставлен, и не создаёт package, claim или Run до ответа

#### Scenario: Варианты не помещаются в один native question
- **WHEN** launch или другой известный host конечный выбор содержит больше
  вариантов, чем принимает native question tool host
- **THEN** host показывает последовательные страницы без скрытого default и
  ждёт explicit selection до mutation
