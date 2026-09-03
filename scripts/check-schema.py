#!/usr/bin/env python3
"""Check/generate embedded and distributed public DTO bundles; preserve F1/v1."""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
IMMUTABLE = {
    "foundation": "4fda82e5908602a8df274a23981884682c38fb08bc465d8cc9a1dd27af3d9c42",
    "core": "573e440951b857afc6b22e4b77a4c0db08a1a252bd9e8007b5bde300cff06441",
    "choice": "7fcacd3aa4719606b3f7ec0d1395b20feabdd393f22f013e48178a13f38f9cd8",
    "invocations": "ff73ea6801148b60e077b20093b904b465ca298c3b233c106375ae2194654864",
    "repeats": "50102a1139609120e763cb614ee86ea67843c0d5d1550f7193b3bae4055f0c06",
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default=str(ROOT / ".tools/go/bin/go"))
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    for profile, options, paths in (
        ("foundation", [], ("internal/runtime/public.schema.json", "schemas/foundation/public.schema.json")),
        ("core", ["--core"], ("internal/runtime/core-public.schema.json", "schemas/core/public.schema.json")),
        ("choice", ["--choice"], ("internal/runtime/choice-decision.schema.json", "schemas/core/choice-decision.schema.json")),
        ("invocations", ["--invocations"], ("internal/runtime/invocations.schema.json", "schemas/core/invocations.schema.json")),
        ("repeats", ["--repeats"], ("internal/runtime/repeats.schema.json", "schemas/core/repeats.schema.json")),
        ("contexts", ["--contexts"], ("internal/runtime/contexts.schema.json", "schemas/core/contexts.schema.json")),
        ("sessions", ["--sessions"], ("internal/runtime/sessions.schema.json", "schemas/core/sessions.schema.json")),
        ("waivers", ["--waivers"], ("internal/runtime/waivers.schema.json", "schemas/core/waivers.schema.json")),
        ("parallel", ["--parallel"], ("internal/runtime/parallel.schema.json", "schemas/core/parallel.schema.json")),
        ("map", ["--map"], ("internal/runtime/map.schema.json", "schemas/core/map.schema.json")),
        ("wait", ["--wait"], ("internal/runtime/wait.schema.json", "schemas/core/wait.schema.json")),
        ("guard", ["--guard"], ("internal/runtime/guards.schema.json", "schemas/core/guards.schema.json")),
        ("reported-cost", ["--reported-cost"], ("internal/runtime/reported-cost.schema.json", "schemas/core/reported-cost.schema.json")),
        ("artifact-publication", ["--artifact-publication"], ("internal/runtime/artifact-publication.schema.json", "schemas/core/artifact-publication.schema.json")),
        ("artifact-closure", ["--artifact-closure"], ("internal/runtime/artifact-closure.schema.json", "schemas/core/artifact-closure.schema.json")),
        ("publication-subscription", ["--publication-subscription"], ("internal/runtime/publication-subscription.schema.json", "schemas/core/publication-subscription.schema.json")),
        ("publication-checks", ["--publication-checks"], ("internal/runtime/publication-checks.schema.json", "schemas/core/publication-checks.schema.json")),
        ("publication-new-only", ["--publication-new-only"], ("internal/runtime/publication-new-only.schema.json", "schemas/core/publication-new-only.schema.json")),
		("publication-failure", ["--publication-failure"], ("internal/runtime/publication-failure.schema.json", "schemas/core/publication-failure.schema.json")),
		("action-intent", ["--action-intent"], ("internal/runtime/action-intent.schema.json", "schemas/core/action-intent.schema.json")),
		("action-admission", ["--action-admission"], ("internal/runtime/action-admission.schema.json", "schemas/core/action-admission.schema.json")),
		("action-grant-admission", ["--action-grant-admission"], ("internal/runtime/action-grant-admission.schema.json", "schemas/core/action-grant-admission.schema.json")),
		("action-delivery", ["--action-delivery"], ("internal/runtime/action-delivery.schema.json", "schemas/core/action-delivery.schema.json")),
		("fork", ["--fork"], ("internal/runtime/fork.schema.json", "schemas/core/fork.schema.json")),
		("workspace", ["--workspace"], ("internal/runtime/workspace.schema.json", "schemas/core/workspace.schema.json")),
		("workspace-tree", ["--workspace-tree"], ("internal/runtime/workspace-tree.schema.json", "schemas/core/workspace-tree.schema.json")),
		("decision-state", ["--decision-state"], ("internal/runtime/decision-state.schema.json", "schemas/core/decision-state.schema.json")),
		("run-decisions", ["--run-decisions"], ("internal/runtime/run-decisions.schema.json", "schemas/core/run-decisions.schema.json")),
        ("step-definition-v3", ["--step-definition-v3"], ("schemas/core/step-definition-v3.schema.json",)),
        ("step-definition-v4", ["--step-definition-v4"], ("schemas/core/step-definition-v4.schema.json",)),
		("step-definition-v5", ["--step-definition-v5"], ("schemas/core/step-definition-v5.schema.json",)),
        ("workflow-revision-v3", ["--workflow-revision-v3"], ("schemas/core/workflow-revision-v3.schema.json",)),
        ("publication-source", ["--publication-source"], ("schemas/core/publication-source-v1.schema.json",)),
        ("publication-source-v2", ["--publication-source-v2"], ("schemas/core/publication-source-v2.schema.json",)),
        ("publication-source-v3", ["--publication-source-v3"], ("schemas/core/publication-source-v3.schema.json",)),
        ("publication-source-v4", ["--publication-source-v4"], ("schemas/core/publication-source-v4.schema.json",)),
		("publication-source-v5", ["--publication-source-v5"], ("schemas/core/publication-source-v5.schema.json",)),
		("publication-source-v6", ["--publication-source-v6"], ("schemas/core/publication-source-v6.schema.json",)),
		("publication-source-v7", ["--publication-source-v7"], ("schemas/core/publication-source-v7.schema.json",)),
		("publication-source-v8", ["--publication-source-v8"], ("schemas/core/publication-source-v8.schema.json",)),
    ):
        content = subprocess.check_output([args.go, "run", "./cmd/schema-gen", *options], cwd=ROOT)
        if profile in IMMUTABLE and hashlib.sha256(content).hexdigest() != IMMUTABLE[profile]:
            raise SystemExit(f"Immutable {profile} schema hash changed; version the new contract separately")
        for path in paths:
            target = ROOT / path
            if profile in IMMUTABLE and target.read_bytes() != content:
                raise SystemExit(f"Immutable {profile} schema drift: {path}; version the new contract separately")
            if args.write:
                target.parent.mkdir(parents=True, exist_ok=True)
                with tempfile.NamedTemporaryFile(dir=target.parent, delete=False) as output:
                    output.write(content)
                    output.flush()
                    os.fsync(output.fileno())
                    temporary = output.name
                os.replace(temporary, target)
            elif not target.exists() or target.read_bytes() != content:
                raise SystemExit(f"Schema drift: {path}; run make schemas")
        print(f"{profile} public schemas match: {len(content)} bytes, sha256:{hashlib.sha256(content).hexdigest()}")


if __name__ == "__main__":
    main()
