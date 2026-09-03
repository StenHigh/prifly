# Task intake остаётся provider-neutral source adapter boundary

## Контекст

Task may arrive as owner chat text or from many external systems, while no
provider protocol, external status or roadmap must leak into core workflow.

## Решение

A read-only source adapter produces one immutable task input with original text,
acceptance constraints, optional sealed sources and source provenance. Owner
selects repository and optional milestone relation; authority transforms the
input into pinned brief and source snapshot before normal Run start.

## Последствия

Chat, GitLab, GitHub, Jira and later sources share one contract. Provider status
does not change Run status; publishing a result back is a separate owner-chosen
effect with its own admission.

## Пересмотр

New source must implement the same typed input contract. Search, polling,
webhook, credential or write-back capability needs its own adapter decision.

## Не входит

Нет automatic host session, provider-specific core branch, two-way issue sync,
automatic close/merge request или roadmap mutation.
