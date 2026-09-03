## 1. Host runner generation

- [x] 1.1 Replace the one generic runner renderer with one Codex and one Claude Code template while preserving the three fixed host IDs; verify a fresh `project init` writes the expected tool instruction, fallback and no-mutation boundary for every host.
- [x] 1.2 Add the one frozen predecessor runner form to exact-runner validation; verify `project init` accepts it only to create absent `.prifly/local.yaml`, leaves all tracked runner bytes unchanged and still rejects an unknown runner.

## 2. Developer decision UX

- [x] 2.1 Add exact generation tests for native launch and Workspace questions, recommended options, complete paging and ordinary free-form inputs; verify `go test ./cmd/prifly` passes.
- [x] 2.2 Update the Project profile documentation with the reviewed-runner migration rule and the fact that Codex native questions require an exposed `request_user_input`; verify the documented paths and command names against generated output.
- [ ] 2.3 Manually exercise one finite question in a Codex runtime that exposes `request_user_input` and in Claude Code; record only the observed host UI result in the change evidence, without claiming runtime or release qualification.

## 3. Validation

- [x] 3.1 Run `openspec validate add-native-host-question-ux --strict`, `go test ./cmd/prifly`, and `git diff --check`; verify all exit successfully.
- [x] 3.2 Run `git diff --name-only -- openspec/changes/archive` and verify it is empty, confirming protected historical planning evidence was not changed.
