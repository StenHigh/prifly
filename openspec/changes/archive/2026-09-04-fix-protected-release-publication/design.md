## Context

См. [proposal.md](proposal.md). GitLab policy проекта разрешает создавать
protected tags `v*` только Maintainers. `release-publish` получает отдельный
project token с ролью Developer и поэтому GitLab API отвечает 403 уже после
подписанной сборки и загрузки assets.

## Goals / Non-Goals

**Goals:**

- Сделать controlled release publication совместимой с current protected-tag
  policy.
- Проверять права publisher credential до mutating GitLab Release request.
- Сохранить разделение signing и publishing credentials.

**Non-Goals:**

- Не ослаблять protected-tag policy до Developer.
- Не публиковать Release личным токеном в обход CI job.
- Не менять runtime, updater или уже опубликованный `v0.2.0`.

## Decisions

### Dedicated Maintainer publisher token

Создаётся отдельный project access token с `api` scope, ролью Maintainer и
именем, явно связывающим его только с release publication. Его exact secret
записывается в существующую protected masked variable
`PRIFLY_RELEASE_PUBLISH_TOKEN`; прежний publisher token отзывается только
после успешной замены variable.

Рассматривалось разрешить Developer создавать `v*` tags. Это отвергнуто:
оно позволило бы любому Developer создавать release-shaped protected tags и
расширило бы круг управления release boundary.

### Read-only permission preflight

`release-publish` после установки `GITLAB_TOKEN` запрашивает project metadata
и проверяет project access level `>= 40` до `glab release create`. При отказе
job выдаёт понятную причину, не создаёт или не меняет Release и не требует
повторной сборки assets.

Локальная статическая проверка CI contract закрепляет наличие этого preflight,
manual protected-tag route и отсутствие личного credential в repository.
Live dry-run GitLab Release не добавляется: у GitLab нет безвредного endpoint,
который доказывает право создать Release для protected tag, а реальная
публикация остаётся owner-controlled action.

## Risks / Trade-offs

- [Maintainer token имеет больше прав] → token остаётся project-scoped,
  protected, masked и доступен только manual `release-publish`; signing key
  остаётся отдельным.
- [Token истечёт или будет отозван] → preflight остановит job до publication
  с точной диагностикой; owner заменяет только protected CI variable.
- [Неудачная первая publication уже оставила assets] → после исправления
  credential повторяется только `release-publish` для того же tag; rebuild и
  duplicate package upload не требуются.

## Migration Plan

1. Добавить preflight, статический CI-contract check и исправить specification.
2. Создать новый dedicated Maintainer project token и атомарно обновить
   protected masked publisher variable.
3. Отозвать прежний publisher token после обновления variable.
4. Повторить только `release-publish` job для существующего `v0.3.0`; проверить
   GitLab Release и его четыре asset links.

Откат: удалить новый token только после возврата variable к заведомо рабочему
publisher credential. Protected-tag policy не ослабляется.
