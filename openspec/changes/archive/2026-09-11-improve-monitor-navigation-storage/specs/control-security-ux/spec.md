## MODIFIED Requirements

### Requirement: Read-only monitor проверяет origin и не раскрывает внутренности

Loopback monitor MUST принимать запросы только с `Host`, равным его
собственному loopback адресу, MUST отвечать `X-Content-Type-Options: nosniff`,
MUST выдавать ошибки через тот же safe Problem contract, что и CLI, без
внутренних путей и сообщений, и MUST ограничивать отдаваемое содержимое
артефакта объявленным пределом без чтения всего blob в память. Операции чтения monitor не изменяют Run. Отдельный endpoint локального
обслуживания SHALL принимать только подтверждённый same-origin POST с
непредсказуемым token, ограниченным JSON body и повторной проверкой owner,
прав и exact source identity. Остальные методы/пути не получают право записи.

#### Scenario: Запрос с чужим Host
- **WHEN** страница с внешнего домена, разрешённого в loopback, обращается к
  `/api/*`
- **THEN** monitor отказывает и не отдаёт записанные данные
