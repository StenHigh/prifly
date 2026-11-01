# Cached composition nodes preserve bounds and declared names

Memoizing a source node must not remove its descendants from the depth budget.
Store the longest dependency path after all parents and imports are loaded.
A leaf has height zero. On cache reuse, check current depth plus cached height.
Compute a maximum across branches, not a sum. Keep source/effective node heights
separate and check active source cycles before reading a cached node.

Test depth eight and nine with short-first and long-first traversals. Exercise
both parents and imports; swap sorted import alias targets to change read order.
Shared exact dependencies still count once in the source inventory.

A cached node's first display name is not the package name an author selected.
Store declared import targets and parent names separately from shared node
pointers. Compare duplicate import aliases against their declared target. Report
the declared names in the composition overview. Cover two profile entries for
one source folder, valid repeats, conflicting targets and parent overview names.
