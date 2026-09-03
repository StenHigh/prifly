# Versioned project profile отделён от local authority

## Контекст

Team members need shared workflows, project steps and skills through version
control, while Run history, credentials, artifacts, claims and worktrees cannot
live in a repository writable by a claimed step.

## Решение

Versioned project profile lives in repository, with ignored local override for
machine paths. Reviewed skills remain in the host skill location. Authority
state, artifacts, locks, sessions and claimed workspaces live outside the
repository in user-local installation data.

## Последствия

Team process is reviewable and reproducible without committing credentials or
runtime state. Launcher lists declared launches and requires exact selection;
raw authority CLI semantics remain compatible while profile path is explicit.

## Пересмотр

Automatic migration, multiple-task repository isolation, credential storage or
package installation require separate policy, authority and compatibility work.

## Не входит

Нет hidden default launch, automatic model invocation, local-state sync,
provider adapter or removal of exclusive repository claim.
