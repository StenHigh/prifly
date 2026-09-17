## MODIFIED Requirements

### Requirement: Data policy контролирует каждый output channel

One data policy MUST apply to context, logs, human notes, exports, previews and
external publication. Предстартовый итог запуска MUST NOT печатать значения
environment объявленных программ: имена переменных достаточно, чтобы владелец
проверил состав, а значение — то самое место, куда проект кладёт секрет.
Документация и вывод MUST говорить об этом одно и то же. Writer validates actual bytes, viewer safely handles
untrusted markup and remote content, and retention/erasure follow project
policy without recreating deleted bytes from hidden cache.

#### Scenario: Error содержит secret

- **WHEN** writer detects sensitive value in output
- **THEN** it redacts value and does not echo it in the diagnostic reason

#### Scenario: Итог запуска показывает окружение программы
- **WHEN** `project questionnaire --prepare` описывает объявленную программу,
  чьё окружение задано в machine-local конфигурации
- **THEN** итог называет переменные по именам и не печатает их значения, а
  документация не обещает большего, чем вывод делает
