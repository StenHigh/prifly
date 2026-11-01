# Distinguish layer inheritance from repeated declarations

A repeated source traversal is not a new explicit layer declaration. The source
node cache applies a common ancestor once. If readOptions reaches a normalized
layer path already in the global effective-layer inventory, a new declaration
has repeated that layer and must refuse before merging settings or contributions.
A local seen map catches only repeated entries within one document.

Keep library and effective modes distinct. Library imports ignore standalone
layers. Test both cache orders, sibling declarations in both parent orders,
ancestor/descendant repetition, normalized path aliases, settings-only and
structural layers, and a valid common-ancestor diamond. Require the duplicate
layer code and absent partial output rather than an unrelated stage conflict.
