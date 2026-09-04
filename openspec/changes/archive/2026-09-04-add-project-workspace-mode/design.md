## Context

Сейчас Project profile умеет перечислять и compile-ить YAML packages, а
`claim create` отдельно создаёт только Git worktree. `SessionTask.Workspace`
служит authority-local scratch directory: туда попадают sealed context и output
slots. Он не может быть заменён checkout repository, иначе запуск сам добавит
служебные файлы в проект. См. [proposal.md](proposal.md) и delta specs.

## Goals / Non-Goals

**Goals:**

- Один `project start` запускает только declared launch и даёт host точную
  рабочую область.
- Interactive host всегда получает выбор `worktree` или `checkout`; CLI default
  `worktree` остаётся только для автоматизации. `checkout` работает в текущей
  ветке без Git topology mutation.
- Оба режима сохраняют один exclusive physical-repository claim и передают
  его exact identity только workspace-write assisted attempts.
- New state/read contracts не меняют смысл сохранённых worktree-only Runs.

**Non-Goals:**

- Не запускать Codex, Claude, AI Factory, provider или background driver.
- Не принимать произвольный directory вместо Git repository и не добавлять
  tracked project setting для личного выбора режима.
- Не разрешать dirty checkout в первом варианте: пользователь commit/stash-ит
  изменения до explicit direct launch. Это исключает смешение старой ручной
  работы и нового Run; поддержка pinned dirty baseline потребует отдельного
  source-snapshot protocol.
- Не менять существующие raw `claim create` и `run start` команды.

## Decisions

### 1. Один public `project start`; хост всегда спрашивает Workspace

Команда получает `--repository`, `--launch`, `--host`, RunBrief и declared
inputs; `--workspace worktree|checkout` опционален для scripting. Before an
interactive host invokes it, host asks one mandatory Workspace question and
waits for an answer; a choice already stated in the user request satisfies the
question. Only a direct non-interactive call defaults to `worktree`. `checkout`
не живёт в tracked `project.yaml` и не угадывается из branch, host или наличия
worktree: это осознанный выбор запускающего разработчика.

Не используется отдельная настройка в `local.yaml`: один флаг покрывает
личное решение без добавления второго формата и precedence rules.

### 2. Project start является orchestration boundary, не model launcher

Команда сначала read-only проверяет profile, launch, host, brief, inputs и
compiler output в temporary directory вне repository. После этого она создаёт
Workspace claim, импортирует sealed package, создаёт Run с exact claim ref и
однократно drive-ит его до первого honest wait/handoff. Host продолжает работу
через existing `session task`, `session submit` и `run drive`; Pri-Fly не
создаёт agent process и не выбирает provider/model.

При ошибке после claim команда выполняет ограниченную компенсацию: удаляет
только созданный worktree или снимает checkout claim и удаляет только package,
который импортировала сама без Run holder. Если компенсация не доказана,
результат называет surviving identity и recovery action; это не маскируется
успехом. Ошибки preflight не создают ни package, ни claim, ни Run.

### 3. Workspace claim расширяет существующий claim record совместимо

Новый mode добавляется к существующей recorded claim форме; отсутствующий mode
старого record означает исторический `worktree`. Existing `claim create` и
`claim create-set` продолжают создавать только этот mode. Новый internal
project-start route создаёт `checkout` claim с canonical repository top-level,
clean HEAD и теми же identity, lease, owner и generation checks.

В `checkout` mode cleanup меняет только claim state: он никогда не вызывает
`git worktree remove`, `checkout`, `switch`, `reset`, `clean` или удаление
файлов. В `worktree` mode сохраняется прежняя device/inode-checked cleanup.
Direct checkout требуется чистым на admission; проверка не выполняет Git
mutation.

### 4. Scratch и Repository Workspace остаются разными путями

Attempt продолжает materialize pinned context, skills и output slots в
authority-local scratch `Attempt.Workspace`. Exact claimed repository path
передаётся отдельным SessionTask field and claim reference only for
`workspace_write`; host использует его как рабочую директорию. Так direct
checkout не получает `context/`, `outputs/` или authority state, а proposal
steps не получают write access к repository.

Новый assisted effect говорит «inside claimed workspace», а не «inside claimed
worktree». Он несёт claim ID/generation and mode; старые enum values и their
state/read versions остаются для прежних Runs. New state/read/schema generation
представляет оба значения только на соответствующей declared compatibility
boundary.

## Risks / Trade-offs

- [Direct checkout меняется пользователем во время Run] → explicit mode и
  clean admission дают честную initial boundary; exclusion of another Pri-Fly
  Run не выдаётся за блокировку пользователя или сторонних программ.
- [Claim/import/start не являются одним filesystem transaction] → all
  validation происходит заранее; subsequent failure returns durable identities
  and bounded cleanup/recovery instead of hiding partial state.
- [Новый state недоступен старому binary] → separate reader version and
  generated schema tests make old reader refuse before interpreting checkout as
  disposable worktree.
- [Host путает scratch с repository] → SessionTask names both paths and host
  skill is rewritten; acceptance test checks no context/output files enter the
  direct checkout.

## Migration Plan

1. Add compatible Workspace claim/session/state contract and generated public
   schemas; old claim records default to `worktree` only on their old reader
   path.
2. Implement `project start`, runner instructions and fixture project.
3. Run focused runtime/CLI/e2e and generated-schema checks, then full release
   gate appropriate for binary change.
4. Publish in the next minor release after explicit version approval. Rollback
   is binary rollback before any checkout-mode Run; after it, an old binary
   refuses new authority state and the new binary releases/recovers the claim.
