[Индекс](index.md) · [Основы](basics.md)

# Полный указатель команд и flags

Этот справочник фиксирует публичную поверхность исходного снимка, включённого в поставку.
Сначала прочитайте процедуру нужной операции. Таблица параметров не заменяет admission или требуемые подтверждения.
Defaults ниже — значения CLI parser. Пустая строка часто означает «требуется» либо «взять declaration»; это объясняет форма команды.
Поле с default `-1` означает отсутствие CAS value, а не допустимую negative version.
Повторяемые значения передаются отдельными flags; их destination и допустимость задаёт конкретная команда.
Глобальные flags: `--project DIR`, `--json`, `--format text|json|csv`, `--help`/`-h`, `--version`.

## Формы публичных команд

```text
Pri-Fly — deterministic local workflow runner

Global: --project DIR  --json  --format text|json|csv

  update                           Install the newest signed stable release for this managed binary
  init [--profile PROFILE] [DIR]    Empty installation; default foundation-sequence/1
  project init [--repository DIR] [--state-root DIR] [--host codex-cli|codex-app|claude-code]
                                   Create a tracked .prifly project profile and separate local authority state
  project workflows [--repository DIR]
                                   List the project scenarios that the launcher may start and their declared inputs
  project workflows search [QUERY] [--category ID] [--catalog URL]
                                   Read a workflow catalog; discovery only, nothing is copied into the project
  project workflows add SOURCE [--ref REF] [--path DIR] [--name NAME] [--catalog URL] [--repository DIR]
                                   Copy one workflow folder from a Git repository or catalog entry into .prifly/workflows and declare it; nothing is sealed, trusted or executed
  project workflows update NAME [--ref REF] [--repository DIR]
                                   Refresh an installed folder to its tracked ref; local edits are refused, extend.yaml and the team's project/ subtree are kept
  project workflows remove NAME [--repository DIR]
                                   Delete the folder and its launches from the tracked profile; authority packages and Runs stay
  project questionnaire --repository DIR (--package NAME|--launch ID)
                [--package-profile NAME] [--decision-policy attended|autonomous] [--preflight-answer ID=JSON] [--runtime-answer ID=JSON]
                                   An unnamed profile, policy or answer is the project's standing one from extend.yaml (profile:, answers:), then the package's
                [--expected-decision-catalog-digest DIGEST]
                                   Read applicable/conditional decisions and optional typed preanswers without creating a Run
  project questionnaire --prepare --launch ID [the same arguments as project start]
                                   Profile /3: validate and show the exact launch summary read-only; pass its review_digest to start
  project runners update [--repository DIR]
                                   Replace only exact known generated host runners; customized files are refused,
                                   a declared host without a runner in this clone is listed as missing_hosts
  project runners add --host codex-cli|codex-app|claude-code [--repository DIR]
                                   Attach selected hosts explicitly; repeat --host to add more than one
  project local set [--executable PATH] [--allow-executable NAME=PATH] [--env NAME=VALUE] [--repository DIR]
                                   Set machine-only local.yaml paths and the environment its programs run with; the authority and shared workflow stay unchanged
  project extend --workflow FILE --workflow-id ID --extensions FILE --output FILE [--step-ref NAME=JSON] [--step-source NAME=FILE]
                                   Insert declared steps into one exact compiled workflow; nothing is sealed, imported or run
                                   --step-ref and --step-source are one pair per step: the ref is written into the workflow, the source is the file it is checked against
                                   NAME is the short step: name from extend.yaml, not the id: inside the file; the known-step list is built from --step-ref alone
                                   examples/troubleshooting.md of the Pri-Fly repository has the worked pair and the refusal each half produces alone
  project compile --repository DIR --package NAME [--host codex-cli|codex-app|claude-code] --output DIR [--value NAME=JSON]
                                   Seal one declared YAML package; import remains a separate owner decision
  project start --repository DIR --launch ID [--host codex-cli|codex-app|claude-code] [--brief FILE] [--input PORT=FILE] [--input-ref PORT=REF.json] [--workspace worktree|checkout]
                [--package-profile NAME] [--preflight-answer ID=JSON] [--decision-policy attended|autonomous] [--expected-decision-catalog-digest DIGEST]
                [--runtime-answer ID=JSON] [--allow-execution] [--expected-launch-digest DIGEST]
                                   A runtime answer is sealed before the Run starts: the step that raises that decision gets this value and does not wait
                                   Show a pre-dispatch summary on stderr, then seal and drive; stdout keeps one final result
                                   A refusal certain before the summary (such as dependency_limit) comes alone, with no summary; one met
                                   only after it (the machine's configuration changed meanwhile) is the last document of stderr, exit non-zero
                                   Profile /3 needs host/Git/brief only when declared; Git writes require a workspace: --workspace, else the launch's standing workspace: in project.yaml
                                   A launch's preflight: program in project.yaml runs in the repository before anything is taken; a non-zero exit refuses the launch with its output
                                   Answer the declared questions up front with repeated --preflight-answer; project questionnaire lists them and returns the digest
                                   Profile /3: get --expected-launch-digest from project questionnaire --prepare with the same start arguments
                                   A package profile is chosen once: with --package-profile, do not also answer the decision that selects it
  project continue --repository DIR --launch ID --source-run RUN_ID --prepare [--host HOST] [--workspace worktree|checkout] [--workspace-commit COMMIT] [--from-stage STAGE] [--allow-duplicate-continuation]
  project continue --repository DIR --launch ID --source-run RUN_ID --expected-launch-digest DIGEST [the same reviewed options]
                                   Start a Run of a workflow that declares a continuation from a finished Run: it takes the inputs the workflow names and the source Run's tree as left; COMMIT claims a new tree there instead
                                   With a launch of the source Run's own workflow, resume it where it stopped, if that workflow declares resumable: accepted stages before that point are carried, the rest runs again with the source's inputs in its tree; STAGE starts from an earlier accepted stage; with no answer, profile or policy named, the source Run's sealed answers are taken
                                   An active continuation or resume of the same source refuses another launch unless --allow-duplicate-continuation is reviewed on prepare and start
  project recover --repository DIR --launch ID --source-run RUN_ID --host HOST --prepare [--from-stage STAGE]
  project recover --repository DIR --launch ID --source-run RUN_ID --host HOST --expected-launch-digest DIGEST --allow-execution [--from-stage STAGE]
                                   Resume a technically failed Run at its failed stage, or STAGE before it, on a new package, in the source Run's own tree with its sealed answers unless others are named; prepare is read-only
  capabilities                     Implemented contracts/profiles, not permission grants
  exit-codes                       What every exit status of this tool means, for a driver that reads $?
   0 the command was carried out. Waiting is reported this way too: run drive returns 0 while an
     assisted attempt is outstanding, so read status/outcome/verdict for the workflow's own result
   2 refused with no narrower class: the form, the input, an object that does not exist, or a
     refusal this build does not sort further
   3 refused by the authority's state or object policy: the version, epoch, claim, slot, admission
     or access this command assumed is not the one held
   4 refused as forbidden: the authority is read-only, or this principal or file may not do it
   5 refused because it cannot be done now: unsupported by this build, an exhausted allowance, or
     a bound that was not met in time
   6 the authority needs attention before more work: recovery, integrity, storage version, persistence
   7 interrupted: the client was stopped; inspect the recorded run before any retry
   Every nonzero status is a refusal and ends stderr with one Problem envelope whose code names the
   exact reason; a command that already wrote a pre-dispatch document to stderr leaves it in front,
   so read the last document of stderr, not the whole stream. No nonzero status means a healthy
   wait; 5 in particular is a refusal, not a handoff
   That code is greppable: examples/troubleshooting.md of the Pri-Fly repository indexes the ones
     teams actually hit by symptom, and names the field or flag that answers each
  version | doctor [--claim ID | --storage] | inventory     Versions, integrity, claim ownership, definitions
  ref FILE --id ID --version X.Y.Z [--raw-text]
                                   Canonical JSON/YAML; --raw-text hashes exact UTF-8 resource bytes
  schema [NAME] [--def DEFINITION]
                                   One exact wire contract or authoring document, or the list of names when NAME is omitted
                                   --def returns one definition with the closure it references, instead of the whole bundle
                                   Authoring YAML has worked references in examples/authoring/ of the Pri-Fly repository;
                                   extension-authoring-reference.yaml shows a complete extend.yaml, extensions included
  validate --workflow FILE          Shape, refs, graph and profile validation; FILE is resolved from the authority root, not the working directory
  preview --workflow FILE [--brief FILE] [--input-ref PORT=REF.json]
  run start --workflow FILE --brief FILE [--input PORT=FILE] [--input-ref PORT=REF.json] [--drive] [--grant ID] [--schema-migration ID]

  run fork --file REQUEST.json      Create a linked Run from exact sealed refs; old Run is unchanged
  run list [--limit N]              This authority's Runs, newest first: id, workflow, status, outcome, created,
                                   and whether a task waits for its host; --json answers run-list/1. Reads only
  run status|next|explain|events|timing RUN_ID
  run output RUN_ID --attempt ID    The newest 64 KiB of stdout and stderr of a program whose executor declares
                                   live_output; offsets name what was skipped. Nothing for any other program.
  run progress RUN_ID               What each program step said about its own progress over fd 4: phase, count,
                                   when, and whether that is now (reported), old (stale), absent (not_reported)
                                   or unknowable here (unavailable). A diagnostic, never the step's result.
                                   events reads one bounded page: --limit N (1..1000) and --after SEQ continue it,
                                   and a partial answer names the next value in next_after, so a reader never
                                   has to open the authority's own storage to finish reading a Run's history.
                                   With --json the events are in .view.events, not .events: jq '.view.events[]'
                                   status names what each step declared it may change: its effects class,
                                   retry_class and, for external_write, the system, operations and target;
                                   question lines say which questions each assisted step answered, the answer and
                                   its basis, as the host reported them (.run.attempts[].question_report in --json)
                                   next answers the move this Run has now. awaiting_host: a task handed to a host waits
                                   for you, and next_command names the command that takes it; idle: nobody waits
                                   explain on a finished Run names where its graph stopped in finish: the invocation,
                                   the finish stage, the outcome, and the stage and verdict of the edge that reached it
                                   when the sealed plan names exactly one, so the reason for an outcome is read here
                                   rather than from activations and the workflow source
                                   timing node kinds: run, workflow_invocation, stage_activation, step_instance, attempt, check_execution
                                   A node's metrics carry only numbers; the boundaries they were measured between are the same-named entry of its intervals, as from_ref/to_ref
                                   kind inside from_ref/to_ref names what a boundary was read from and is a separate vocabulary from a node's kind: it also uses step, activation, invocation, stop and report
  run drive RUN_ID [--next] [--stop-before-program]
                                   Foreground owner; interrupt requests cancel. --next answers with the next action instead of the run view
                                   A program step runs to completion inside this call: a host with a client timeout drives such a Run in the background
                                   --stop-before-program drives through control stages and handoffs and hands control back before admitting one, so the host chooses how to run it
  run decisions RUN_ID              Read the sealed decision ledger and pending question
  run decision RUN_ID request --attempt ID --envelope-digest DIGEST --decision ID --expected-run-version N [--yield-execution]
                                   Compatible executor requests one declared runtime decision
  run decision RUN_ID answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON
                                   Answer exactly the current pending decision; --request-digest is pending_request_digest from run decisions, not the request_digest of a command receipt
  run pause|cancel|stop RUN_ID --reason TEXT [--command-id ID]
                                   stop reads the restriction from --kind pause|cancel; pause and cancel state it in the operation
  run release RUN_ID --expected-epoch N --stop ID:GENERATION --reason TEXT
  run resume RUN_ID --expected-version N --reason TEXT
  run reopen RUN_ID --expected-version N --reason TEXT
                                   Run again the stage a technically failed Run broke on; completed stages keep their sealed outputs and are not re-run. Refused for a Run that reached an outcome: an accepted fail is an answer, not a breakage
  run resolve RUN_ID (--attempt ID | --check ID) --outcome not_applied|applied --reason TEXT [--expected-version N]
                                   Close one obligation whose outcome the authority never observed, by owner attestation; it frees the slot and never re-runs anything. A program declaring external_write lands here when it ends without an accepted result unless its retry_class is idempotent (failure code external_write_unreconciled); after it, project continue runs the step again
  run waive RUN_ID --step STEP --check-id ID --check-version X.Y.Z --check-digest DIGEST --reason TEXT
  run waivers RUN_ID                A waiver is not a pass: the outcome stays completed_with_waivers
  control status                    Enrolled session principal, object access and control stops
  control stop --scope installation|project --reason TEXT [--command-id ID]
  control release --scope installation|project --expected-epoch N --stop ID:GENERATION --reason TEXT
                                   Authority-side stops; they forbid new admissions, not running work
  package import --dir DIR --reason TEXT [--command-id ID]
                                   Seal, verify and trust a local package; nothing in it is executed
  package schema-migration prepare --request FILE
                                   Read-only exact consumer/schema/supplier review; incomplete choices are diagnostic
  package schema-migration apply --plan FILE --expected-review-digest DIGEST --reason TEXT [--command-id ID]
                                   Owner-scoped exception for verified shared schemas; preserves terminal revoke and old Runs
  package schema-migration inspect --id ID
                                   Recorded decision and its current validity; later lifecycle decisions invalidate it
  package list                      Trusted packages, their origin and recorded trust decision
                                   author_package names the edition an author wrote; it is shown, never accepted by --package, since two builds of it are one version
  package inspect --ref REF.json | --component ID [--package ID@VERSION]
                                   Read one exact sealed package, or the bytes of one component it declares, by the ID the package gave it
                                   A JSON component comes back in component; a context is prose and comes back in component_text
                                   Several trusted packages can declare one component; --package names which edition to read
  package import --archive FILE.tar [--signature FILE.sig] --reason TEXT
  package trust-root --id ID --public-key HEX --reason TEXT | --id ID --remove --reason TEXT
                                   A key inside a package never appoints itself trusted
  package verify --id ID --version X.Y.Z
                                   Re-read every sealed byte; a mismatch is reported, never repaired
  package remove|quarantine|revoke|restore --id ID --version X.Y.Z --reason TEXT
                                   Remove refuses while a run holds it; revoke also blocks old pins
                                   Remove deletes nothing: it withdraws trust, frees the edition's registry entries,
                                   and restore with the same --id and --version undoes it
                                   Remove is the only status that releases this package's inventory pins, and revoke is terminal:
                                   a revoked revision is neither removed nor restored afterwards, so revoking forfeits that release
  claim create --repository DIR --owner ID [--base REF]
  claim create-set --repository DIR --repository DIR --owner ID
                                   One decision for the whole set: all resources or none
  claim list | claim heartbeat --id CLAIM --generation N | claim release --id CLAIM --generation N
                                   An expired lease blocks a conflicting claim; it never hands ownership over
  session task --run RUN_ID [--all] The sealed handoff an assisted host currently holds; --all lists every outstanding one
                                   Each handed-over task is also written to task.json in its attempt workspace, so the host needs only that directory
  session publish --file COMMAND.json
                                   Publish a sealed artifact before the assisted attempt settles
  action propose --file COMMAND.json
                                   Seal one exact ActionIntent proposal; this never executes it
  action admit --file COMMAND.json
                                   Consume exact approvals and retain ActionAdmission; this never delivers it
  session action --file COMMAND.json
                                   Backward-compatible alias for action propose
  session take --run RUN_ID --attempt ATTEMPT_ID
                                   Record that you took this task, once, when you start it (the task's take_command).
                                   Reading a task writes nothing, so without it the monitor cannot tell the time the
                                   task waited for you from your own working time. A second take changes nothing
  session submit --file SUBMISSION.json
                                   Report for exactly the attempt and envelope that were handed over
                                   When the task says question_report: required, the report carries answered_questions:
                                   every question the step met -- a skill's own chat questions included -- with the answer
                                   and its basis (decision + decision_id, input + port, instructions, person, judgement);
                                   [] when there was none. Without it the report is refused as answered_questions_missing
  session submit --template --run RUN_ID [--attempt ATTEMPT_ID]
                                   The skeleton of that report: identities and admitted output slots filled, verdict, summary, digests and answered_questions left to the host
  session disconnect --run RUN_ID --attempt ATTEMPT_ID
                                   Record an expired handoff as unknown; it never becomes success
  approval policy --operation OP --quorum N --independence RULE --reason TEXT
  approval request --operation OP --intent-digest DIGEST --reason TEXT
  approval decide --id APPROVAL --decision approve|reject --reason TEXT
  approval revoke --id APPROVAL --reason TEXT | approval list
                                   One owner cannot form an independent quorum; such a decision is refused
  grant issue --subject ID --capability OP [--resource RESOURCE.json] --max-operations N --lifetime-ms MS --reason TEXT
  grant revoke --id GRANT --reason TEXT | grant list
                                   Bounded delegation; issuing it is gated exactly like what it delegates
  monitor [--addr 127.0.0.1:7777] [--scan-root DIRS]   registered authorities; --scan-root also walks DIRS
    --ui light --full-monitor-url URL [--base-path /light/] [--light-projection FILE]  read-only task board
                                   Local user Runs, disk usage and confirmed cleanup; DIRS is an OS path-list. Auto-starts on Run creation, without opening a browser.
  storage budget show
  storage budget set --bytes N --reason TEXT --prepare
  storage budget set --bytes N --reason TEXT --expected-review-digest DIGEST --command ID
                                   Raise the finite logical-page ceiling; review first, drive the existing Run separately
  capacity show | capacity set --capacity N --reason TEXT
                                   How many attempts run at once here; a workflow declares its own and the smaller governs
  command receipt --id COMMAND_ID   Inspect a lost response without re-running
  artifact import --file FILE [--type json|blob] [--schema-ref REF.json]
  artifact inspect --ref REF.json
  artifact export --ref REF.json --output FILE
  source import --file FILE --type json|blob [--schema-ref REF.json] [--media-type MIME]
    [--external-identity TEXT] [--external-version TEXT] [--external-scope TEXT]
                                   Returns a SourceSnapshot artifact; external metadata is unverified
  task prepare --input TASK.json  Validates TaskInput/1, seals it as a SourceSnapshot and writes a RunBrief
  step publish --file COMMAND.json | step status   (inside an admitted step)
  telemetry catalog | telemetry query --file QUERY.json

Release and resume are separate; neither silently starts a background driver.
Use --command-id to retry the same command. Exit 0 confirms a command/read,
not a successful workflow outcome. Read the typed status/outcome/verdict.
Local executables are trusted and run with your OS rights; this is not a sandbox.
A program runs in its own process group. Stopping it (timeout, cancel, a lost
driver) sends SIGTERM to the whole group, then SIGKILL to the group after its
grace_ms (at most 5000); the attempt ends only when the group is empty. Cleanup
the program starts is killed with it, so a program that needs longer finishes
its cleanup on its next run rather than moving it out of the group.
A step's effects.class external_write and its external_write boundary are its
author's statement in the Run's record, not a limit on what a program does.
Its retry_class decides what an interrupted program does: idempotent (running it
again ensures the state by itself, which nothing checks) is run again by project
continue; deduplicated, reconcile_required and never leave the Run uncertain
until run resolve. pure is refused: it means a step without effects.
Opt in with init --profile core-workflow/1 for on_error, JSON projections,
declared input defaults, choices, calls and bounded repeat.
Full context and automatic checks require core-configuration/2 and
core:adapter/local-process@2.0.0; automatic check executors use operation check.
Existing Runs retain their pinned semantics.
Not yet supported: external effect delivery/execution (ActionDelivery),
unattended execution, automatic retry/reconcile/compensation, generic external
subscriptions, backpressure/retention/GC, provider usage, comparisons and profiling.
```

## Queue: дополнительные формы

```text
project queue add --input TASK.json --command-id ID [--priority N]
project queue list
project queue show --task-id ID
project queue claim-next --owner SESSION --command-id ID
project queue update --input TASK.json QUEUE_SELECTION --command-id ID [--priority N]
project queue release QUEUE_SELECTION --reason TEXT --command-id ID
project queue complete QUEUE_SELECTION --reason TEXT --command-id ID
project queue reconcile QUEUE_SELECTION --intent-id ID --resolution not_started|effects_resolved --reason TEXT --command-id ID
```

Все queue forms принимают `--repository DIR`. QUEUE_SELECTION состоит из четырёх flags из раздела ниже.
Точная процедура, phases и условия resolution: [queue](operations.md#локальная-очередь-задач).

## Значения и ограничения

`ID`, `DIGEST`, `N`, `FILE` берутся из текущего task/view/review либо явно выбранного input.
Файлы request у authority-команд разрешаются по правилам выбранной authority; project-команды используют repository.
Путь `--workflow` низкоуровневых validate/preview относится к authority, не к shell cwd.
`--input PORT=FILE` передаёт bytes; `--input-ref PORT=FILE` — JSON ArtifactRef. Не передавайте один port дважды разными путями.
`--value NAME=JSON` — typed значение compile; `--value JSON` у decision answer — сам JSON answer.
Длительности в `*-ms` задаются целыми миллисекундами. Count/revision/generation — целые значения текущего contract.
Шаблоны имеют placeholders, которые нельзя копировать как реальные refs.

## Flags по семействам

Default в таблице — значение parser до применения shared settings, standing answers и context.
Пустой список повторяемого flag означает отсутствие явно переданных значений.
Обязательность задаёт точная форма подкоманды выше: элементы в `[...]` можно опустить.
Условные требования поясняют страницы процедур. Пустой default не отменяет обязательность flag.
Повторяющиеся имена с разным типом принадлежат разным формам подкоманды, а не передаются одновременно.
`--expected-*` берёт текущую версию/digest из review, task или status, не константу из этой таблицы.
<!-- cli-flags:action -->

### action

Условия применения: [процедура](control.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--file` | строка | `""` | Обязательный JSON-файл ActionProposal для propose/admit. Используйте схему action и refs выбранного Run. |

<!-- cli-flags:approval -->

### approval

Условия применения: [процедура](control.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |
| `--reason` | строка | `""` | Явная причина изменения состояния; нужна в owner mutation с этим flag. |
| `--id` | строка | `""` | Точный ID выбранной сущности из её view либо новый ID при объявлении. Смысл определяется подкомандой. |
| `--operation` | повторяемое значение | `[]` | control operation that requires approval |
| `--quorum` | целое | `1` | Число необходимых approve-решений policy; положительное целое, по умолчанию 1. |
| `--independence` | строка | `"different_from_proposer"` | Условие независимости policy; default different_from_proposer требует другого principal, а не другого процесса. |
| `--operation` | строка | `""` | Имя control operation. В policy повторяется; в approval request выбирает одну операцию из policy. |
| `--intent-digest` | строка | `""` | Digest конкретного намерения из подготовленного запроса. Approval не переносится на другие bytes намерения. |
| `--decision` | строка | `""` | approve or reject |

<!-- cli-flags:artifact -->

### artifact

Условия применения: [процедура](artifacts.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--file` | строка | `""` | Файл фактических bytes для import. Для JSON используйте --type json и подходящий --schema-ref. |
| `--ref` | строка | `""` | JSON-файл полного ArtifactRef для выбранного artifact; для inspect/export обязателен. |
| `--schema-ref` | строка | `""` | JSON-файл точного schema ref для typed JSON import. Не применяйте схему другой версии. |
| `--type` | строка | `"blob"` | Форма artifact: json либо blob. Default blob сохраняет сырые bytes; json использует canonical JSON. |
| `--media-type` | строка | `""` | Media type blob. Укажите тип фактических bytes; это не команда конвертации. |
| `--output` | строка | `""` | Путь создаваемого output. Для export обязателен; существующий путь обрабатывается по условиям подкоманды. |

<!-- cli-flags:capacity -->

### capacity

Условия применения: [процедура](operations.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--capacity` | целое int64 | `0` | attempts admitted at once |
| `--reason` | строка | `""` | Явная причина изменения состояния; нужна в owner mutation с этим flag. |
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |

<!-- cli-flags:claims -->

### claims

Условия применения: [процедура](workspace.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |
| `--repository` | повторяемое значение | `[]` | git repository to claim (repeat for an atomic set) |
| `--base` | строка | `"HEAD"` | Git commit/ref для нового claim. HEAD — default текущего repository, не путь другой рабочей копии. |
| `--owner` | строка | `""` | run or session identity that owns the claims |
| `--repository` | строка | `""` | git repository to claim a worktree from |
| `--owner` | строка | `""` | run or session identity that owns the claim |
| `--id` | строка | `""` | Точный ID выбранной сущности из её view либо новый ID при объявлении. Смысл определяется подкомандой. |
| `--generation` | целое int64 | `0` | Текущее поколение claim из claim view. Heartbeat/release требуют подходящее поколение; 0 не подменяет его. |

<!-- cli-flags:control -->

### control

Условия применения: [процедура](control.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--scope` | строка | `"project"` | installation or project |
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |
| `--reason` | строка | `""` | Явная причина изменения состояния; нужна в owner mutation с этим flag. |
| `--expected-epoch` | целое int64 | `-1` | Текущий control epoch из status. Значение -1 означает отсутствие переданного ожидания; не подставляйте его вместо известного epoch. |
| `--stop` | повторяемое значение | `[]` | Повторяемый ID:GENERATION активного stop из status. Release относится к указанным stops и текущему epoch. |
| `--approval` | повторяемое значение | `[]` | approval consumed by this release |
| `--grant` | строка | `""` | grant this release draws one operation from |

<!-- cli-flags:grant -->

### grant

Условия применения: [процедура](control.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |
| `--reason` | строка | `""` | Явная причина изменения состояния; нужна в owner mutation с этим flag. |
| `--id` | строка | `""` | Точный ID выбранной сущности из её view либо новый ID при объявлении. Смысл определяется подкомандой. |
| `--subject` | строка | `""` | ID principal, которому выдаётся Grant. Grant относится к указанной операции и условиям. |
| `--capability` | повторяемое значение | `[]` | control operation this grant delegates |
| `--resource` | повторяемое значение | `[]` | exact action resource scope JSON file (repeatable) |
| `--max-operations` | целое int64 | `0` | Положительный предел операций Grant. 0 означает отсутствие обязательного значения, а не безграничный Grant. |
| `--lifetime-ms` | целое int64 | `0` | Положительный срок Grant в миллисекундах. 0 не означает бессрочное разрешение. |
| `--approval` | повторяемое значение | `[]` | Повторяемый ID Approval из approval list; укажите все разрешения, необходимые выбранной policy. |

<!-- cli-flags:monitor -->

### monitor

Условия применения: [процедура](observability.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--scan-root` | строка | `""` | also walk these directories for authorities, separated by the OS path-list separator; by default only registered authorities are shown |
| `--addr` | строка | `"127.0.0.1:7777"` | loopback address to listen on |
| `--ui` | строка | `"full"` | full monitor or read-only Light task board |
| `--full-monitor-url` | строка | `"http://127.0.0.1:7777/"` | Light: absolute full-monitor URL |
| `--base-path` | строка | `"/"` | Light: URL path prefix |
| `--light-projection` | строка | `""` | Light: versioned local projection mapping JSON |

<!-- cli-flags:packages -->

### packages

Условия применения: [процедура](artifacts.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--ref` | строка | `""` | exact package ImmutableRef JSON file |
| `--component` | строка | `""` | declared ID of one component an installed package carries |
| `--package` | строка | `""` | ID@VERSION of the installed package to read the component from |
| `--id` | строка | `""` | Точный ID выбранной сущности из её view либо новый ID при объявлении. Смысл определяется подкомандой. |
| `--version` | строка | `""` | Точная версия package/ref из inventory. Это не диапазон версий и не текущая версия binary. |
| `--reason` | строка | `""` | Явная причина изменения состояния; нужна в owner mutation с этим flag. |
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |
| `--public-key` | строка | `""` | hex ed25519 public key |
| `--note` | строка | `""` | Примечание владельца к trust root; не меняет sealed bytes package. |
| `--remove` | boolean | `false` | Явно удалить указанный trust root при true; false оставляет операцию добавления/изменения. |
| `--dir` | строка | `""` | sealed package source directory |
| `--archive` | строка | `""` | sealed package archive |
| `--signature` | строка | `""` | detached signature over the manifest digest |

<!-- cli-flags:projectCompile -->

### project compile

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--package` | строка | `""` | named package from project.yaml |
| `--output` | строка | `""` | new directory outside the repository and local authority |
| `--host` | строка | `""` | host entry point that selects project skills |
| `--package-profile` | строка | `""` | per-compilation package profile |
| `--explain` | boolean | `false` | show source composition and effective settings; output directory is optional |
| `--value` | повторяемое значение | `[]` | explicit YAML value NAME=JSON |

<!-- cli-flags:projectExtend -->

### project extend

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--workflow` | строка | `""` | compiled YAML workflow source |
| `--workflow-id` | строка | `""` | exact workflow ID named by extensions |
| `--extensions` | строка | `""` | YAML extension list |
| `--output` | строка | `""` | compiled workflow output |
| `--step-ref` | повторяемое значение | `[]` | exact step reference NAME=JSON |
| `--step-source` | повторяемое значение | `[]` | compiled step YAML NAME=FILE, read for the ports the insertion binds |

<!-- cli-flags:projectInit -->

### project init

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--state-root` | строка | `""` | machine-local authority root outside the repository |
| `--host` | повторяемое значение | `[]` | supported host to attach to a new profile (repeatable) |

<!-- cli-flags:projectLocal -->

### project local set

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--executable` | строка | `""` | absolute path to the prifly binary this machine runs |
| `--allow-executable` | повторяемое значение | `[]` | allow a local executable NAME=/absolute/path (repeatable) |
| `--env` | повторяемое значение | `[]` | environment for this machine's programs NAME=VALUE (repeatable); PATH, APP_ENV and the like live here, never in the shared package |
| `--model-profile` | повторяемое значение | `[]` | what a declared profile name means on this machine: "HOST NAME key=value" (repeatable); the value is handed to the host, never read here |
| `--env-from` | повторяемое значение | `[]` | environment read at run time NAME=env:VAR, NAME=file:/path or NAME=dotenv:/path:KEY (repeatable); the value is never written here and never printed |

<!-- cli-flags:projectPrepareAndStart -->

### project start / continue / recover / questionnaire --prepare

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--schema-migration` | строка | `""` | Явный ID записанного решения schema migration; пустое значение сохраняет прежний admission. |
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--launch` | строка | `""` | declared launch ID from project.yaml |
| `--host` | строка | `""` | host entry point that selects project skills |
| `--brief` | строка | `""` | confirmed RunBrief JSON file |
| `--workspace` | строка | `""` | explicit worktree or checkout for assisted repository writes |
| `--allow-execution` | boolean | `false` | approve the selected workflow programs, arguments and supporting files |
| `--package-profile` | строка | `""` | per-Run package profile |
| `--decision-policy` | строка | `""` | attended or autonomous declared-decision policy; unnamed, the project's answers.decision_policy in extend.yaml, then attended |
| `--expected-decision-catalog-digest` | строка | `""` | catalog digest returned by project questionnaire |
| `--expected-launch-digest` | строка | `""` | review digest returned by project questionnaire --prepare |
| `--source-run` | строка | `""` | completed partial or rejected Run to continue |
| `--workspace-commit` | строка | `""` | full commit ID to claim a new tree at instead of taking over the source Run's tree (project continue) |
| `--allow-duplicate-continuation` | boolean | `false` | explicitly start another continuation while one from the same source Run is active |
| `--from-stage` | строка | `""` | accepted root stage of the source Run to start again from, before the one it stopped at (resuming project continue, project recover) |
| `--command-id` | строка | `""` | stable command identity for an explicit retry |
| `--input` | повторяемое значение | `[]` | declared input PORT=FILE |
| `--input-ref` | повторяемое значение | `[]` | declared input PORT=ARTIFACT_REF.json |
| `--naming-input` | повторяемое значение | `[]` | project-owned naming NAME=FILE; profile /5, только новый Git worktree; общий snapshot до 128 KiB |
| `--preflight-answer` | повторяемое значение | `[]` | declared preflight decision ID=JSON |
| `--runtime-answer` | повторяемое значение | `[]` | declared runtime decision ID=JSON, sealed before the Run starts |

<!-- cli-flags:projectQuestionnaire -->

### project questionnaire

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--package` | строка | `""` | named package from project.yaml |
| `--launch` | строка | `""` | declared launch ID from project.yaml |
| `--package-profile` | строка | `""` | per-Run package profile |
| `--decision-policy` | строка | `""` | attended or autonomous declared-decision policy; unnamed, the project's answers.decision_policy in extend.yaml, then attended |
| `--expected-decision-catalog-digest` | строка | `""` | catalog from the preceding questionnaire |
| `--host` | строка | `""` | accepted for symmetry with --prepare; the read-only questionnaire selects no host |
| `--preflight-answer` | повторяемое значение | `[]` | declared preflight decision ID=JSON |
| `--runtime-answer` | повторяемое значение | `[]` | optional runtime preanswer ID=JSON |

<!-- cli-flags:projectQueue -->

### project queue add/list/show/update/claim-next/release/complete/reconcile

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--command-id` | строка | `""` | stable queue command identity |
| `--owner` | строка | `""` | owner session identity for claim-next |
| `--input` | строка | `""` | exact TaskInput/1 JSON file |
| `--priority` | целое int64 | `0` | priority; larger values are claimed first |
| `--reason` | строка | `""` | owner reason for release, completion or reconcile |
| `--intent-id` | строка | `""` | launch intent to inspect or reconcile |
| `--resolution` | строка | `""` | not_started for reserved, effects_resolved for dispatching |

<!-- cli-flags:projectQueueFlags -->

### queue selection в questionnaire/prepare/start и project queue

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--task-id` | строка | `""` | claimed queue Task ID |
| `--task-owner` | строка | `""` | recorded queue owner session identity |
| `--task-revision` | целое int64 | `0` | exact task content revision |
| `--task-claim-generation` | целое int64 | `0` | exact queue claim generation |

<!-- cli-flags:projectRunners -->

### project runners add/update

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |
| `--host` | повторяемое значение | `[]` | supported host to attach (repeatable, add only) |
| `--check` | boolean | `false` | with update: answer what would be replaced and write nothing |

<!-- cli-flags:projectWorkflows -->

### project workflows

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | directory that owns the shared Pri-Fly profile |

<!-- cli-flags:projectWorkflowsAdd -->

### project workflows add

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | Git repository that owns the shared Pri-Fly profile |
| `--ref` | строка | `""` | tag, branch or commit; the remote default branch when omitted |
| `--path` | строка | `""` | workflow folder inside the source repository |
| `--name` | строка | `""` | folder name under .prifly/workflows; the source folder name when omitted |
| `--catalog` | строка | `projectDefaultWorkflowCatalog` | catalog repository that resolves a bare entry name |

<!-- cli-flags:projectWorkflowsRemove -->

### project workflows remove

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | Git repository that owns the shared Pri-Fly profile |

<!-- cli-flags:projectWorkflowsSearch -->

### project workflows search

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--category` | строка | `""` | only entries of this catalog category |
| `--catalog` | строка | `projectDefaultWorkflowCatalog` | catalog repository |

<!-- cli-flags:projectWorkflowsUpdate -->

### project workflows update

Условия применения: [процедура](launch.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--repository` | строка | `"."` | Git repository that owns the shared Pri-Fly profile |
| `--ref` | строка | `""` | switch the tracked ref; the recorded ref when omitted |

<!-- cli-flags:ref -->

### ref

Условия применения: [процедура](artifacts.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--id` | строка | `""` | Обязательный immutable ID определения. Вместе с --version и canonical bytes задаёт полный ref. |
| `--version` | строка | `""` | Точная версия package/ref из inventory. Это не диапазон версий и не текущая версия binary. |
| `--raw-text` | boolean | `false` | hash exact UTF-8 resource bytes |

<!-- cli-flags:run -->

### init / validate / preview / command receipt

Условия применения: [процедура](hosting.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--profile` | строка | `foundation-sequence/1` | explicit execution semantics profile |
| `--workflow` | строка | `""` | explicit workflow file |
| `--brief` | строка | `""` | optional explicit brief to preview |
| `--input-ref` | повторяемое значение | `[]` | explicit immutable input port=REF.json |
| `--id` | строка | `""` | Точный ID выбранной сущности из её view либо новый ID при объявлении. Смысл определяется подкомандой. |

<!-- cli-flags:runCommand -->

### run start/fork/status/next/explain/events/timing/output/progress/drive/decision/pause/cancel/stop/release/resume/reopen/resolve/waive

Условия применения: [процедура](hosting.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--schema-migration` | строка | `""` | Явный ID записанного решения schema migration; пустое значение сохраняет прежний admission. |
| `--workflow` | строка | `""` | Явно выбранный workflow source/ref по форме команды. Низкоуровневый путь разрешается относительно authority. |
| `--brief` | строка | `""` | Файл brief legacy workflow. Project /3 использует declared inputs вместо универсального brief. |
| `--command-id` | строка | `""` | Уникальный ID намерения владельца. При повторе потерянного запроса используйте тот же ID и те же bytes. |
| `--drive` | boolean | `false` | После успешного создания или возобновления Run явно выполнить driver; false оставляет выполнение отдельному run drive. |
| `--grant` | строка | `""` | control grant that authorises a brief recording confirmation as standing_grant; starting spends one of its operations |
| `--input` | повторяемое значение | `[]` | Повторяемый PORT=FILE для declared input. Передаёт bytes файла; один port нельзя также передать через --input-ref. |
| `--input-ref` | повторяемое значение | `[]` | Повторяемый PORT=FILE.json с ArtifactRef. Передаёт существующий artifact вместо нового input файла. |
| `--file` | строка | `""` | JSON-файл RunForkRequest либо DecisionRequest по подкоманде. Envelope/version берите из текущего task. |
| `--reason` | строка | `""` | Явная причина изменения состояния; нужна в owner mutation с этим flag. |
| `--attempt` | строка | `""` | current session attempt ID |
| `--envelope-digest` | строка | `""` | current session envelope digest |
| `--decision` | строка | `""` | declared runtime decision ID |
| `--expected-run-version` | целое int64 | `-1` | current Run version from session task |
| `--yield-execution` | boolean | `false` | for assisted-session/6: transfer control and stop using this delivery after an accepted request |
| `--decision` | строка | `""` | pending decision ID |
| `--request-digest` | строка | `""` | pending_request_digest from run decisions |
| `--expected-run-version` | целое int64 | `-1` | current Run version from run decisions |
| `--value` | строка | `""` | typed JSON answer for the pending decision |
| `--next` | boolean | `false` | with drive: answer with the next action instead of the run view |
| `--stop-before-program` | boolean | `false` | with drive: drive through control stages and handoffs, hand control back before a program step is admitted |
| `--attempt` | строка | `""` | the program attempt whose output to read |
| `--after` | целое int64 | `0` | read events after this seq; the answer names the next value when it is partial |
| `--limit` | целое | `100` | how many events to read at most, 1..1000 |
| `--kind` | строка | `"pause"` | Тип stop: pause либо cancel. Выбор cancel не доказывает отсутствие частичных эффектов writer. |
| `--invocation` | строка | `""` | restrict this invocation and its descendants within the selected Run |
| `--expected-epoch` | целое int64 | `-1` | Текущий control epoch из status. Значение -1 означает отсутствие переданного ожидания; не подставляйте его вместо известного epoch. |
| `--stop` | повторяемое значение | `[]` | Повторяемый ID:GENERATION активного stop из status. Release относится к указанным stops и текущему epoch. |
| `--step` | строка | `""` | Точный step_instance_id из Run. Waiver относится к конкретному шагу, а не имени стадии похожего Run. |
| `--check-id` | строка | `""` | ID check из результата проверки; вместе с check-version/check-digest задаёт точный check для waiver. |
| `--check-version` | строка | `""` | Точная версия check для waiver, из check ref принятого результата. |
| `--check-digest` | строка | `""` | Digest check из полного ref. Не используйте digest текста отчёта вместо check definition. |
| `--attempt` | строка | `""` | uncertain attempt to resolve |
| `--check` | строка | `""` | uncertain check to resolve |
| `--outcome` | строка | `""` | not_applied or applied |
| `--expected-version` | целое int64 | `-1` | Текущий Run version из status/task. CAS обязателен для показанных reopen/resume/resolve форм; -1 не является версией Run. |

<!-- cli-flags:runList -->

### run list

Условия применения: [процедура](hosting.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--limit` | целое | `defaultRunListLimit` | how many Runs to show, newest first |

<!-- cli-flags:session -->

### session

Условия применения: [процедура](hosting.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--run` | строка | `""` | Точный run_id из start/list/next. Обязателен для session task/template/take. |
| `--attempt` | строка | `""` | the outstanding attempt this host came for |
| `--all` | boolean | `false` | list every outstanding handoff |
| `--file` | строка | `""` | JSON-файл SessionSubmission, action или publication по подкоманде. Submit получает форму через --template. |
| `--template` | boolean | `false` | print the submission skeleton for one outstanding handoff instead of reporting |
| `--attempt` | строка | `""` | the outstanding attempt the skeleton is for |
| `--attempt` | строка | `""` | Точный attempt_id из task; обязателен для take и template конкретного Attempt. |

<!-- cli-flags:source -->

### source

Условия применения: [процедура](artifacts.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--file` | строка | `""` | explicit source file |
| `--type` | строка | `""` | json or blob |
| `--schema-ref` | строка | `""` | exact JSON schema reference file |
| `--media-type` | строка | `""` | content media type |
| `--external-identity` | строка | `""` | declared external identity |
| `--external-version` | строка | `""` | declared external version |
| `--external-scope` | строка | `""` | declared external scope, not acquisition permission |

<!-- cli-flags:step -->

### step

Условия применения: [процедура](hosting.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--file` | строка | `""` | Обязательный JSON-файл StepPublicationRequest. Публикация declared hook не завершает Attempt. |

<!-- cli-flags:storage -->

### storage

Условия применения: [процедура](operations.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--bytes` | строка | `""` | finite allocated logical-page ceiling in exact integer bytes |
| `--reason` | строка | `""` | owner explanation |
| `--prepare` | boolean | `false` | review without changing storage |
| `--expected-review-digest` | строка | `""` | digest returned by --prepare |
| `--command` | строка | `""` | stable explicit command identity |

<!-- cli-flags:task -->

### task

Условия применения: [процедура](artifacts.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--input` | строка | `""` | TaskInput/1 JSON file or - for stdin |

<!-- cli-flags:telemetry -->

### telemetry

Условия применения: [процедура](observability.md). Все перечисленные flags поддерживает parser этого семейства; подкоманда проверяет применимость.

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--file` | строка | `""` | Обязательный JSON-файл TelemetryQuery для query. Форму берите из schema и catalog. |


## Поддержанные dispatch operations

Полный список учитывает операции, которых краткий help может не перечислять.
Формы и применимые flags находятся выше; операция без отдельного flag использует typed request из файла.

<!-- cli-dispatch:approval -->

| Команда |
|---|
| `approval decide` |
| `approval list` |
| `approval policy` |
| `approval request` |
| `approval revoke` |

<!-- cli-dispatch:artifact -->

| Команда |
|---|
| `artifact export` |
| `artifact import` |
| `artifact inspect` |

<!-- cli-dispatch:capacity -->

| Команда |
|---|
| `capacity set` |
| `capacity show` |

<!-- cli-dispatch:claims -->

| Команда |
|---|
| `claim create` |
| `claim create-set` |
| `claim heartbeat` |
| `claim list` |
| `claim release` |

<!-- cli-dispatch:control -->

| Команда |
|---|
| `control release` |
| `control status` |
| `control stop` |

<!-- cli-dispatch:grant -->

| Команда |
|---|
| `grant issue` |
| `grant list` |
| `grant revoke` |

<!-- cli-dispatch:packages -->

| Команда |
|---|
| `package import` |
| `package inspect` |
| `package list` |
| `package quarantine` |
| `package remove` |
| `package restore` |
| `package schema-migration` |
| `package revoke` |
| `package trust-root` |
| `package verify` |

<!-- cli-dispatch:projectCommand -->

| Команда |
|---|
| `project compile` |
| `project continue` |
| `project extend` |
| `project init` |
| `project local` |
| `project questionnaire` |
| `project queue` |
| `project recover` |
| `project runners` |
| `project start` |
| `project workflows` |

<!-- cli-dispatch:projectQueue -->

| Команда |
|---|
| `project queue add` |
| `project queue claim-next` |
| `project queue complete` |
| `project queue list` |
| `project queue reconcile` |
| `project queue release` |
| `project queue show` |
| `project queue update` |

<!-- cli-dispatch:projectRunners -->

| Команда |
|---|
| `project runners add` |
| `project runners update` |

<!-- cli-dispatch:projectWorkflowsCommand -->

| Команда |
|---|
| `project workflows add` |
| `project workflows remove` |
| `project workflows search` |
| `project workflows update` |

<!-- cli-dispatch:run -->

| Команда |
|---|
| `action` |
| `approval` |
| `artifact` |
| `capabilities` |
| `capacity` |
| `claim` |
| `command` |
| `control` |
| `doctor` |
| `exit-codes` |
| `grant` |
| `help` |
| `init` |
| `inventory` |
| `monitor` |
| `package` |
| `preview` |
| `project` |
| `ref` |
| `run` |
| `schema` |
| `session` |
| `source` |
| `step` |
| `storage` |
| `task` |
| `telemetry` |
| `update` |
| `validate` |
| `version` |

<!-- cli-dispatch:runCommand -->

| Команда |
|---|
| `run cancel` |
| `run decision` |
| `run decisions` |
| `run drive` |
| `run events` |
| `run explain` |
| `run fork` |
| `run list` |
| `run next` |
| `run output` |
| `run pause` |
| `run progress` |
| `run release` |
| `run reopen` |
| `run resolve` |
| `run resume` |
| `run start` |
| `run status` |
| `run stop` |
| `run timing` |
| `run waive` |
| `run waivers` |

<!-- cli-dispatch:session -->

| Команда |
|---|
| `session action` |
| `session disconnect` |
| `session publish` |
| `session submit` |
| `session take` |
| `session task` |

<!-- cli-dispatch:source -->

| Команда |
|---|
| `source import` |

<!-- cli-dispatch:step -->

| Команда |
|---|
| `step publish` |
| `step status` |

<!-- cli-dispatch:task -->

| Команда |
|---|
| `task prepare` |

<!-- cli-dispatch:telemetry -->

| Команда |
|---|
| `telemetry catalog` |
| `telemetry query` |

Строка help об automatic retry относится к общей автоматической оркестрации. Declared technical_retries — отдельная поддержанная возможность с явным bound.

### Project naming inputs, profile /5

`project questionnaire --prepare` и `project start` принимают повторяемый `--naming-input NAME=FILE`.
NAME объявлен в `worktree_naming.inputs`; FILE содержит JSON value для project-owned schema.
Этот flag не заменяет `--input` и не имеет варианта `--naming-input-ref` или default.
Общий snapshot ограничен 131072 bytes canonical UTF-8 JSON.
Questionnaire `/5` показывает условную потребность; summary/start `/8` показывает полный snapshot.
Continue/resume/recover отказывают override и используют source claim.
См. [поля и команды](launch.md#project-owned-naming-inputs).

<!-- cli-flags:schemaMigration -->

### package schema-migration

Процедура: [миграция схем](schema-migration.md).

| Flag | Тип | Default | Значение/источник |
|---|---|---|---|
| `--request` | строка | `""` | Файл schema-migration-request/1 для prepare. |
| `--plan` | строка | `""` | Проверенный schema-migration-plan/1 для apply. |
| `--expected-review-digest` | строка | `""` | Точный review_digest plan. |
| `--reason` | строка | `""` | Явная причина owner apply. |
| `--command-id` | строка | `""` | Стабильная identity retry; без значения CLI создаёт новую. |
| `--id` | строка | `""` | Записанный decision ID для inspect. |
