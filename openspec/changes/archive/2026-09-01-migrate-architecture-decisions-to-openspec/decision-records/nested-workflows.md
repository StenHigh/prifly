# Вложенные сценарии внутри одного Run

## Контекст

Reusable workflow composition должна сохранять одну authority history и не
создавать hidden child project или independent execution semantics.

## Решение

Nested workflow is resolved to an exact pinned definition before start and runs
as an invocation inside its parent Run. Parent and child use typed inputs,
outputs, context and limits with explicit invocation provenance.

## Последствия

Current controls, claims, budgets, retry boundaries and evidence stay visible
in one lifecycle. A child cannot substitute a newer alias, create a second
authority or invent unbounded recursion.

## Пересмотр

Cross-authority composition, remote invocation or new call semantics require
separate identity, fencing, recovery and qualification decisions.

## Не входит

Нет dynamic network resolution, implicit detached Run или arbitrary recursive
workflow graph.
