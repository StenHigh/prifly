# CSV report — no AI, Git or host required

An ordinary managed workflow: **parse CSV → validate rows → write report**.
The graph, typed ports and portable execution bindings live in YAML. The small
Node worker contains only the work performed by each step, not the workflow.
Pri-Fly itself is the installed binary; neither Go nor Python is needed.

This example currently requires the unreleased source build with Project
profile `/3`; it is not compatible with the earlier released binary.

## Try it

You need Pri-Fly and Node. Start in an ordinary empty directory. Replace the
two `/absolute/...` paths below with your local paths. The state directory must
be outside the project. No repository initialization or AI skills are needed.

```sh
prifly project init --repository "$PWD" --state-root /absolute/path/to/csv-state
cp -R /absolute/path/to/prifly/examples/workflows/csv-report .prifly/workflows/
```

Declare the copied folder in `.prifly/project.yaml`. For this empty project its
complete contents are:

```yaml
schema_version: prifly-project-profile/3
packages:
  csv-report: {source: .prifly/workflows/csv-report}
launches:
  csv-report:
    title: CSV report
    description: Parse, validate and summarize a CSV file.
    kind: workflow
    workflow: .prifly/workflows/csv-report/workflow.yaml
```

Approve your installed Node binary. This machine-specific path is stored in
the ignored `.prifly/local.yaml`, never in the shared workflow:

```sh
prifly project local set --allow-executable "node=$(node -p process.execPath)"
prifly project workflows
prifly project questionnaire --launch csv-report
prifly project start --launch csv-report \
  --input csv=.prifly/workflows/csv-report/sample.csv --allow-execution
```

`--allow-execution` approves this launch's declared programs, arguments and
supporting files. It is separate from allowing a machine's `node` binary.
`process.execPath` resolves the actual Node binary, including when your terminal
uses a version-manager shim that would not work in a worker's clean environment.
Pri-Fly executes programs with the owner's permissions; this is not a sandbox.
The questionnaire has no decisions here: there is no invented brief, host,
workspace choice or AI questionnaire.

To check an edited workflow without starting a Run:

```sh
prifly project compile --package csv-report --output /absolute/path/to/new-csv-package
```

The output directory must be new and outside the project and its state directory.
Compilation validates and seals files; it does not execute the worker or install
the package into the local authority.

## Read the result

The start response includes the Run ID and its state. If it is still running,
continue the same Run with its external state directory:

```sh
prifly --project /absolute/path/to/csv-state run drive RUN_ID
prifly --project /absolute/path/to/csv-state run status RUN_ID --json
```

Copy the `run.output_artifacts.report` object from the completed response into
`report-ref.json`, then export the immutable output to a new file:

```sh
prifly --project /absolute/path/to/csv-state artifact export \
  --ref report-ref.json --output report.txt
```

For `sample.csv` the actual output is:

```text
Pri-Fly CSV report
Rows: 3
Total: 24
```

The input is an intentionally small CSV dialect: exact `name,amount` header,
1–10000 rows, unquoted ASCII names beginning with a letter, and nonnegative
integer amounts. Names may additionally contain letters, digits, spaces, `_`
and `-`. Names must be unique and amounts and total must be JavaScript-safe
integers. CRLF and LF endings are accepted. This is not a general CSV parser.
Malformed input or duplicate names ends with `rejected`, without a final report.

## What to change

- `workflow.yaml`: graph, portable bindings and package metadata.
- `steps/*.yaml`: typed inputs, outputs and execution contracts.
- `schemas/rows.yaml`: intermediate data contract.
- `files/worker.mjs`: the three ordinary Node operations; no dependencies.

The worker reads the execution envelope from stdin and port paths from
`PRIFLY_CONTEXT_FILE`. It writes each output to its allocated slot and returns
one typed result on file descriptor 3. Pri-Fly verifies and seals those bytes.
The worker never edits the project, claims a Git worktree or invokes an AI model.

`TestCLIProjectCSVReportNoGitNoAI` exercises these shipped files through the
public CLI with an empty `PATH`, allowing Node only through its absolute path.
