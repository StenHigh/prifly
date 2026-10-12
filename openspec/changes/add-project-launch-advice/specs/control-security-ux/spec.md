## Source of truth

Текущий source set — `openspec/specs/control-security-ux/spec.md`; его ownership не меняется. Delta уточняет пользовательскую границу до запуска и синхронизируется в тот же source set при archive. Authority, public contracts и история Runs сохраняются; рекомендация не расширяет effects, claims или outputs.

## MODIFIED Requirements

### Requirement: Project profile separates versioned packages from local authority

Project profile MUST хранить версионируемые workflow files под repository
`.prifly`, а runtime authority — вне repository. `local.yaml` SHALL называть
exact binary/state paths без угадывания global PATH. Launcher MUST показывать
объявленные launch и inputs до mutation и MUST NOT автоматически выбирать
исполняемый workflow по task URL или chat text.

Текущая host-сессия SHALL поддерживать условный совет по исходной задаче,
`title` / `description` объявленных launch и применимым критериям `PROJECT.md`
по контракту общего runner. Совет MUST оставаться необязательной помощью при
выборе; его пропуск или проектное отключение MUST NOT мешать ручному выбору.
Явно названный точный ID SHALL считаться уже сделанным выбором. До выбора
host MUST NOT создавать package, claim или Run; совет, предвыбранный вариант
в интерфейсе, молчание и истечение времени MUST NOT считаться ответом.

Конечный выбор SHALL использовать фактически предоставленный и допустимый
для этого вопроса native question tool host, независимо от того, был ли совет,
сохраняя все реальные варианты и страницы при лимите tool. Host MUST учитывать
ограничения режима и при asynchronous tool ждать отдельный фактический ответ,
а не receipt показа. При недоступном или неприменимом tool host MUST ждать
явного текстового ответа без переключения режима ради обхода ограничения.
Подтверждение подготовленного запуска SHALL оставаться отдельным действием.

#### Scenario: Launch не выбран

- **WHEN** пользователь передал task URL без explicit launch ID
- **THEN** launcher показывает selectable declared launches, даёт совет только при применимости и наличии оснований и ждёт явный выбор без запуска Run

#### Scenario: Предвыбранный совет не является ответом

- **WHEN** UI выделил рекомендованный launch, но пользователь не отправил выбор
- **THEN** host остаётся в ожидании без mutation независимо от молчания или прошедшего времени

#### Scenario: Совет пропущен

- **WHEN** пользователь либо `PROJECT.md` отключил совет
- **THEN** все объявленные launch остаются доступными для явного выбора с обычной последующей процедурой запуска

#### Scenario: Асинхронный вопрос ещё не получил ответ

- **WHEN** host показал native выбор, но получил только receipt показа
- **THEN** пользователь видит доступные варианты, а runner не переходит к зависимым действиям до отправленного выбора
