# Native call extension fixtures

JSON-compatible readable YAML: each file is one authoring document. `repository/`
contains an unchanged upstream root and sibling plus explicit project-owned children,
steps and schema dependencies. `extensions/review.yaml` exercises request → report/gate
through an inner native call. `extensions/product.yaml` exercises configuration/ui/ux
→ gate/ui_report/ux_report/environment/release. A second extension consumes every export.
No package-name policy, AI, browser, nested project start or hidden graph patch is used.
The public harness selects one extend file in a disposable copy and records source hashes.
These fixtures prove synthetic engine transport, not released/live-host qualification.
