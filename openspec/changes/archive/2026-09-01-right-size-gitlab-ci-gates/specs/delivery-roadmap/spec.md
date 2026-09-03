## MODIFIED Requirements

### Requirement: Текущая очередь отделена от future catalogue
Delivery plan SHALL хранить active priority, contributor-readiness work и
post-RC queue отдельно от catalogue возможных workflow и дальних идей.
Изменение future idea MUST NOT неявно менять committed release scope или
runtime contract.

Текущая engineering priority после qualified core-local RC выполняется строго
в таком порядке:

1. Reproducible native GitLab CI запускает automatic fast product gates
   `make ci-check` и `make e2e` только при изменении продукта, OpenSpec
   contract или CI; обычные documents runner не запускают. Full `make race`
   остаётся отдельным manual GitLab job и обязательным evidence RC/release
   batch; локальный `make check` сохраняет тот же полный смысл. Удалённые
   document checks не возвращаются.
2. Единственный YAML authoring route заменяет дорелизные альтернативы: profile
   v1, Python task recipes и плоский package source удаляются до первого
   public release. Source проекта остаётся в profile v2 и YAML-каталоге
   workflow.
3. Independent validator corpus проверяет этот единственный YAML route вне
   встроенных unit tests.
4. Local YAML editor contract описывает стабильный authoring surface только
   после того, как route и corpus приняты.

Эти работы защищают поставленную поверхность и не меняют порядок или formal
status P1/P2. Они не создают compatibility obligation для authoring sources,
которых не было в public release.

Future catalogue может предлагать packages и workflows, но их install/listing
внешнего происхождения требует future action authority. Empty install и
default execution не ходят в сеть; install не предлагается молча в init.
Будущими, а не promised current capabilities, остаются полное provider usage
and cost view, additional workflow operators, workspace-visible delivery
records, trusted reuse, full dry run, helper continue command и MCP surface.

#### Scenario: Команда выбирает contributor-ready работу
- **WHEN** team начинает следующий post-RC change
- **THEN** она берёт первую незавершённую работу из объявленной
  последовательности и не создаёт compatibility scope для unreleased source
  form

#### Scenario: В каталог добавлен новый workflow
- **WHEN** team добавляет возможный workflow или integration
- **THEN** он остаётся proposal с prerequisite и explicit authority boundary,
  а не появляется как supported scenario текущего release
