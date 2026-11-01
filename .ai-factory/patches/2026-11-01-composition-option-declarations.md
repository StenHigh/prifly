# Merge option declarations by source ancestry

A parent's effective option map contains unchanged values from its ancestors.
Those values are not independent overrides. Retain immutable local declarations
before inheritance, then normalize each owner's root settings to the final root.
For each replaceable key, a descendant declaration shadows its ancestor. Compare
only independent maximal declarations. An explicit consumer value resolves their
conflict. An explicit reset to the base value remains a declaration.

Keep configuration objects, answers, resources and model profile records opaque.
Preserve absent versus empty exclusions and give package profile defaults one
owner. Keep all setting origins for explain. References retain their uniqueness
rule; executable programs retain separate ownership and source-folder checks.

Regression coverage needs direct ancestor/descendant parents, diamonds, both
parent orders, untouched branches, explicit resets and consumer resolution.
Verify emitted configuration defaults and sealed Run resource limits through the
public CLI, not only internal option maps.
