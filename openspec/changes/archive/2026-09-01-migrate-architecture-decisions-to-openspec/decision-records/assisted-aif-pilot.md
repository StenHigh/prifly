# Assisted AI Factory pilot остаётся optional integration

## Контекст

Нужен реальный воспроизводимый assisted workflow для проверки Pri-Fly, но
внешние skills и agent host не могут стать обязательной зависимостью core или
источником authority.

## Решение

Pilot uses pinned skill bytes, declared context, an exclusive claimed worktree
and authority-issued invocation. Plan generation, review and implementation are
separate admitted steps; the host reports typed progress/result and cannot
choose routes, mutate authority state or widen workspace scope.

## Последствия

Only claimed-worktree changes and a checked local commit are in the bounded
pilot. Host loss becomes an honest waiting or unknown state. The pilot proves a
small disposable repository path, not sandbox isolation, remote execution,
provider usage, background continuation or a mandatory project lifecycle.

## Пересмотр

Remote/managed execution, parallel workflow operation or a different host must
add its own identity, pinning, effect and recovery qualification.

## Не входит

Нет automatic model launch, network publish, credentials, arbitrary shell,
package install during Run или automatic retries.
