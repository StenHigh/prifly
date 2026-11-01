# Preserve program ownership before source flattening

A compiled composition contains components from several sources. A matching ID
in that combined inventory does not prove that a provider owns the component.
Retain each program declaration's permitted document aliases before flattening.
Source programs may target local and explicitly inherited step/check documents.
Layer programs retain only their resolved targets; their supporting-file directory
is not their ownership boundary.

Validate kind and author ID against that source inventory. Then select compiled
components through their Name and Names aliases. The latter preserves exact
shared dependency deduplication. Reject unknown owned targets with a typed fault.
Skip owned private documents outside the selected closure before parsing config
or reading supporting files. Pass only selected owned components to the existing
execution reader; retain the existing program conflict guard.

Cover both directions across consumer/provider boundaries, check and wrong-kind
targets, unselected private ID collisions at different bytes/versions, valid
provider and inherited programs, and shared exact refs with equal/conflicting
programs. Assert refusal cleanup and exact sealed configs/files, not only IDs.

An identical shared binding emits once, but every read supporting-file path
still belongs in source provenance. Record the second provider path before
skipping duplicate payload emission. Cover equal bytes in two source folders.
