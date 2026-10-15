## MODIFIED Requirements

### Requirement: Project результат различает авторскую версию и сборку
При compilation profile `/3` CLI MUST выдавать `project-compile/2`, а при
start без queue — `project-start/3`. Queue-bound start MUST использовать
`project-start/4` с exact queue binding и `project-launch-summary/4`;
prepare без queue сохраняет `project-launch-summary/3`. Оба результата MUST содержать `author_package`
с авторскими `id`/`version`, `build_key` и exact compiled ref в `package`.
Результат start MUST сохранять Run, Workspace и применимые Decision Sheet и
autonomy summary; compile и start одного входа MUST согласовывать root и
сборку. Legacy `/2` MUST сохранять существующие response versions и поля.

#### Scenario: Команда запускает другой вариант
- **WHEN** один author package компилируется и запускается с profile `/3`
- **THEN** оба ответа показывают понятную авторскую версию и одну exact сборку,
  а потребитель не принимает author version за alias последнего варианта

## ADDED Requirements

### Requirement: Queue CLI отделяет захват от разрешения исполнения

CLI SHALL предоставлять provider-neutral `project queue add/list/show/update/claim-next/release/complete/reconcile`, typed versioned JSON outputs и стабильные refusal codes. Queue mutation и queue-bound project start/continue/recover SHALL требовать явный caller `--command-id`, а owner-sensitive операции — task revision и claim generation. Read команды и prepare MUST NOT создавать queue либо Run. Task selection SHALL передаваться project questionnaire/prepare/start явными queue identity/owner/revision/generation, а source-bound продолжение MUST сохранять recorded Task identity.

Generated runner SHALL уметь принять claimed QueueTask, сохранить original request и показать доступные project launches. Он MUST получить фактический выбор launch и необходимые typed inputs, провести existing questionnaire/prepare/review и ждать требуемого ответа до start. Claim-next MUST NOT выбирать launch, разрешать executable effects, спавнить host или продолжать draining. Вторая host-сессия и повтор команды SHALL сначала читать durable intent/association; неизвестный dispatch SHALL переводить UX к reconcile, не к ещё одному start.

#### Scenario: Host взял next и ждёт выбора
- **WHEN** generated runner получил claimed task, но пользователь ещё не выбрал launch или не дал обязательный ответ
- **THEN** request и claim сохранены, а programs, workspace claim и Run ещё не созданы

#### Scenario: Ответ start потерян
- **WHEN** runner повторно получает ту же задачу после admission и потери ответа
- **THEN** он показывает записанный Run/association либо reconcile состояние, а не запускает задачу повторно
