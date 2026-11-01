# Composition wrappers preserve technical failures and exact imports

Generated wrappers must distinguish business rejection from technical failure.
A wrapper leaves technical failure unhandled. Native Call propagates `child_failed`;
native Parallel propagates `branch_failed`. The outer inserted Call applies the
authored `arrange.on_error`. Provider-authored handlers stay unchanged. Do not add
an outcome to represent a technical fault or modify runtime for this lowering.

Before changing group lowering, exercise a distinct error route and rejection
route. Cover Choice predicate errors, sequence Calls, parallel failed branches,
step process failure and provider workflow failure. Require failed invocations to
have no outcome; require siblings to settle and publication to remain absent.
Unknown Choice predicates and ordinary rejected outcomes keep their own routes.

Import aliases merge by declared target within one source namespace. Repeating
one exact alias/target pair in a folder, layer or extend reuses that dependency.
The same alias with a different target retains its typed refusal. Test both forms
and check that the reachable provider closure contains no duplicate identities.
