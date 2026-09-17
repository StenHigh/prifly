# aif-fix: project context

- A fix that changes spec text, the glossary, a published contract or a
  frozen schema needs an OpenSpec change first (an external defect is its own
  change). A fix inside the current contract goes straight to code and tests.
- Refusals carry their code in `runtime.Fault`, never in error text
  (`make refusal-check`); a fix adds the failing case before the green one.
