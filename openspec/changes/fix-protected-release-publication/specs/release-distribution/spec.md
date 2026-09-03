## MODIFIED Requirements

### Requirement: Credentials публикации разделены
Private signing key и credential для GitLab Release API MUST быть разными
protected masked CI variables. Publisher credential MUST быть отдельным
project-scoped access token с ролью Maintainer и GitLab `api` scope, потому
что release tag защищён как Maintainers-only. Он используется только manual
publication job, который до изменения GitLab Release MUST проверять, что
credential имеет project access не ниже Maintainer, и давать понятную
диагностику при отказе.

#### Scenario: Publisher credential отсутствует
- **WHEN** owner запускает publication job без publisher credential
- **THEN** job отказывает до создания или изменения GitLab Release

#### Scenario: Publisher credential не имеет прав protected tag
- **WHEN** owner запускает publication job с credential, у которого project
  access ниже Maintainer
- **THEN** job отказывает до создания или изменения GitLab Release и сообщает,
  что protected tag требует отдельный publisher token с ролью Maintainer

#### Scenario: Publisher credential совместим с protected tag
- **WHEN** owner запускает publication job с отдельным protected masked
  project token роли Maintainer и `api` scope
- **THEN** job создаёт или обновляет GitLab Release только для уже
  квалифицированного protected semantic-version tag
