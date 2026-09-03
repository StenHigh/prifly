# Manual host UI evidence

## Claude Code — 2026-09-02

Claude Code 2.1.246 was asked in an interactive terminal to call
`AskUserQuestion` for one `worktree`/`checkout` decision without executing a
command. It rendered a native chooser headed `Workspace`, showed both options
with their consequences, offered keyboard navigation and `Esc` cancellation.
The question was cancelled; no repository file or command was changed.

## Codex

Not exercised in this implementation session. The current Codex runtime does
not provide `request_user_input` (it is exposed only in eligible Plan-mode
contexts), so no UI behavior is inferred from the generated instruction.
