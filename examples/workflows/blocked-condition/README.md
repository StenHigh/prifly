# Blocked on a condition — retry bounded, stop, continue later

Work that needs one condition before it can be done: a program checks it,
returns **`blocked`** with the reason when it does not hold, a remedy step
records the obstacle, a bounded `repeat` tries again, and after three
attempts the Run stops `partial` with the obstacle as its output. A second
workflow declares that it **continues** such a Run and goes on once the
condition holds. No AI, Git or host is needed; the steps are small Node
programs.

The condition here is a file whose path the machine sets; in a real workflow
it is whatever your work needs — a service that answers, a document that
arrived, an approval that was given. Pri-Fly knows none of that: it routes
the verdict, keeps the obstacle, bounds the retries and hands the Run over.
What `blocked` means and how to handle it in your own workflow:
[`authoring/blocked-guide.md`](../../authoring/blocked-guide.md).

Needs Pri-Fly with `workflow_continuation` and `next_handoff` in
`prifly capabilities`.

## What is where

| File | What it shows |
|---|---|
| `source/steps/check.yaml` | a program step promising its `obstacle` output on `blocked` (step contract 12) |
| `source/steps/remedy.yaml` | the step `blocked` routes to: the obstacle is its input, it may change nothing |
| `source/workflows/attempt.yaml` | one attempt: `on: {pass, blocked, fail}`, `blocked` → remedy → finish `partial` |
| `source/workflow.yaml` | a `repeat` around the attempt: `continue_on: [partial]`, `max_iterations: 3`, `on_limit` → finish `partial` with the obstacle |
| `continue/workflow.yaml` | `continuation`: continues `example:workflow/blocked-condition` ended `partial`, takes its request and last obstacle, checks again |
| `source/files/worker.mjs` | the programs; the obstacle schema is in `source/schemas/obstacle.yaml` |

## Try it

You need Pri-Fly and Node. Start in an empty directory; replace the
`/absolute/...` paths.

```sh
prifly project init --repository "$PWD" --state-root /absolute/path/to/state
cp -R /absolute/path/to/prifly/examples/workflows/blocked-condition/source .prifly/workflows/blocked-condition
cp -R /absolute/path/to/prifly/examples/workflows/blocked-condition/continue .prifly/workflows/blocked-condition-continue
```

`.prifly/project.yaml`:

```yaml
schema_version: prifly-project-profile/3
packages:
  blocked-condition: {source: .prifly/workflows/blocked-condition}
  blocked-condition-continue: {source: .prifly/workflows/blocked-condition-continue}
launches:
  work:
    title: Work on a condition
    description: Retries at most three times while the condition is absent.
    kind: workflow
    workflow: .prifly/workflows/blocked-condition/workflow.yaml
  continue:
    title: Continue work on a condition
    description: Continues a Run that stopped partial.
    kind: workflow
    workflow: .prifly/workflows/blocked-condition-continue/workflow.yaml
```

Allow Node and tell the programs where the condition lives — a file that does
not exist yet:

```sh
prifly project local set --allow-executable "node=$(node -p process.execPath)" \
  --env CONDITION_FILE=/absolute/path/to/condition
echo '{"subject":"the report"}' > request.json
```

**1. The condition is absent.** Prepare, review, start:

```sh
prifly project questionnaire --prepare --launch work --input request=request.json --allow-execution
prifly project start --launch work --input request=request.json --allow-execution \
  --expected-launch-digest DIGEST
```

The Run ends `partial`. Three attempts each returned `blocked`, each remedy
recorded the obstacle, and the repeat stopped on its declared limit — not on
the Run's global budget.

**2. Read it as someone who was not there.**

```sh
prifly --project /absolute/path/to/state run next RUN_ID
```

`arrived_from` names stage `attempt`, route `on_limit`, and the `obstacle`
the last attempt handed over; `run status RUN_ID` shows the same obstacle as
the Run's output. Nobody's memory of the session is needed.

**3. Make the condition hold and continue.**

```sh
touch /absolute/path/to/condition
prifly project continue --prepare --launch continue --source-run RUN_ID --allow-execution
prifly project continue --launch continue --source-run RUN_ID --allow-execution \
  --expected-launch-digest DIGEST
```

The prepare output shows where each carried input comes from:
`request` from the source Run's input, `previous_obstacle` from the last body
of stage `attempt`. The continuation checks the condition again as its first
stage — it does not trust the old obstacle — passes, and ends `succeeded`.
The source Run stays `partial`; nothing it accepted runs again. From now on
`run next` on the source lists this workflow in `continuations`.

**4. `fail` is not `blocked`.** Unset the condition path
(`project local set --env CONDITION_FILE=`) and start again: the check cannot
work at all, returns `fail`, and the Run ends `rejected` — a judgement, routed
elsewhere, never retried as if it were an absent condition.

The same four runs are the test `TestCLIBlockedExampleAcceptance`; it copies
these folders into a fresh project and drives them through the public CLI.
