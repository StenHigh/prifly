## MODIFIED Requirements

### Requirement: Project launch является единственной исполнимой точкой входа
Public Project launch MUST принимать exact ID объявленного workflow launch и
typed значения его inputs, compile/seal/register exact package до Run и
закреплять выбранные source/context bytes и inputs. Launch MUST NOT выбирать
сценарий по тексту задачи, default launch или наличию файлов.
Для `/3` host, Git Workspace и RunBrief MUST требоваться только по объявленному
контракту выбранного сценария. Обычный command launch MUST обходиться без них.
Interactive host MUST получить explicit `worktree` или `checkout` до запуска
работы, требующей Git Workspace; отсутствие ответа означает ожидание.
Без такой работы вопрос и claim MUST отсутствовать. CLI `/3` MUST требовать
явный workspace mode для Git-записи; default `/2` сохраняется для совместимости.
Запуск MUST NOT неявно запускать model/provider или расширять права host.

Когда host предоставляет native question tool, конечный Workspace decision
MUST быть показан через него, а не текстом, имитирующим кнопки. Если
количество допустимых вариантов любого конечного developer decision превышает
лимит tool, host MUST показывать все варианты последовательными страницами и
сохранять возможность отказаться; он MUST NOT скрывать варианты или переходить
к default. RunBrief, file path и произвольное typed input value остаются
обычным вводом, так как не являются заранее известным конечным набором
вариантов.

#### Scenario: Объявленный launch запускается
- **WHEN** пользователь назвал launch, required inputs и нужные ему ресурсы
- **THEN** Run использует только sealed revision этого launch и сообщает
  выбранный Workspace, если он требуется

#### Scenario: Launch не объявлен
- **WHEN** пользователь называет отсутствующий или не-workflow launch
- **THEN** система отказывает до compilation, registration, claim или Run

#### Scenario: Host не получил выбор Workspace
- **WHEN** launch требует Git-запись, а worktree/checkout не выбран
- **THEN** host спрашивает через native tool, если он предоставлен, и не
  создаёт package, claim или Run до ответа

#### Scenario: Обычная папка содержит command workflow
- **WHEN** launch `/3` не требует Git или assisted execution
- **THEN** он исполняется без host, Git claim и фиктивного RunBrief

#### Scenario: Варианты не помещаются в один native question
- **WHEN** launch или другой известный host конечный выбор содержит больше
  вариантов, чем принимает native question tool host
- **THEN** host показывает последовательные страницы без скрытого default и
  ждёт explicit selection до mutation
