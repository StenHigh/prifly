# Контекст, source snapshots и checks

## Контекст

External input, workspace content and result claims may drift after planning.
The core therefore needs explicit source provenance and a separate acceptance
boundary for content and semantic checks.

## Решение

Source descriptor captures declared origin; selected bytes are sealed as
immutable snapshot before use. Context manifests bind exact resources to
declared ports. Checks use typed request/result contracts, own admission and
sealed subjects; producer output becomes accepted only after required checks
settle.

## Последствия

Live external reads cannot silently replace pinned data. Pending acceptance,
late evidence, check failure and unknown outcome remain distinct from an
accepted worker result. Preview may describe checks but cannot dispatch them.

## Пересмотр

Additional context source, checker execution mode or evidence claim requires
declared contract, bounded resources and qualification of its boundary.

## Не входит

Нет hidden lookup, mutable workspace repair, checker-selected route или claim
that a shape-valid artifact proves its semantic correctness.
