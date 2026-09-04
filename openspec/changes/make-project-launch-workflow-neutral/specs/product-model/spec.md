## MODIFIED Requirements

### Requirement: Предмет и границы закреплены до выполнения
До значимой работы Run MUST закреплять предмет, ожидаемый результат, scope,
criteria, разрешённые ресурсы и основание старта. Для task-driven запуска
RunBrief SHALL сохранять эти сведения; `TaskInput/1` MUST сохранять исходный
текст и provenance, а preparation выводить RunBrief и SourceSnapshot без
изменения scope. Существенная неоднозначность MUST требовать уточнения.
Confirmed project/resource identity имеет приоритет над соседним разговором.

Новый versioned запуск без task intake SHALL закреплять основание через
admitted Start владельца с exact WorkflowRevision, declared inputs,
configuration и policy/resources. Workflow определяет expected outputs и
criteria; отсутствие RunBrief MUST быть явно допустимым состоянием этого
контракта, не фиктивным документом. Если workflow требует RunBrief как input,
его отсутствие MUST отвергаться. Старые RunStart/state versions MUST сохранять
обязательный brief и свой смысл. Прямой выбор exact workflow/task MAY быть
основанием в существующих полномочиях; уже разрешённые безопасные действия
MUST не требовать повторного подтверждения без новой причины.

#### Scenario: Предложенный brief неверно понял предмет
- **WHEN** owner отклоняет preview RunBrief
- **THEN** работа не начинается до исправления и подтверждения scope

#### Scenario: Владелец запускает обработку файла
- **WHEN** он явно выбирает workflow и inputs без предметной задачи
- **THEN** новый Run закрепляет их и declared boundaries, не сочиняя RunBrief
