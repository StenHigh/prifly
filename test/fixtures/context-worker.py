#!/usr/bin/env python3
"""Small deterministic worker for verify-context.py; no AI or network calls.

This is a local-context/2 fixture, not a replacement for the historical wrapper.
Only the declared instruction entry is trusted. Input text remains literal data.
"""

import hashlib
import json
import os
from pathlib import Path
import sys


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def read_slot(slot):
    path = Path(slot["path"])
    require(not path.is_absolute() and ".." not in path.parts, "nonlocal context path")
    data = path.read_bytes()
    require(digest(data) == slot["ref"]["digest"], "context source digest mismatch")
    return data


def main():
    raw = sys.stdin.buffer.read((2 << 20) + 1)
    require(len(raw) <= 2 << 20, "oversized envelope")
    envelope = json.loads(raw)
    require(envelope["schema_version"] == "1", "unsupported envelope")
    require(digest(raw) == os.environ["PRIFLY_ENVELOPE_DIGEST"], "wrong exact envelope bytes")
    for field, variable in (("run_id", "PRIFLY_RUN_ID"), ("step_instance_id", "PRIFLY_STEP_ID"), ("attempt_id", "PRIFLY_ATTEMPT_ID")):
        require(envelope[field] == os.environ[variable], "foreign worker identity")
    transport = json.loads(Path(os.environ["PRIFLY_CONTEXT_FILE"]).read_bytes())
    require(transport["schema_version"] == "local-context/2", "unsupported local context")
    manifest = json.loads(read_slot(transport["manifest"]))
    rendering = json.loads(read_slot(transport["rendering"]))
    require(transport["manifest"]["ref"] == envelope["context_manifest_ref"], "foreign manifest")
    require(rendering["envelope"] == envelope and "check_request" not in rendering, "wrong bootstrap variant")
    require(rendering["manifest"] == manifest, "rendering changed the manifest")
    require(len(manifest["entries"]) == len(transport["sources"]) == len(rendering["sources"]), "incomplete source list")
    instructions = 0
    for entry, slot, rendered in zip(manifest["entries"], transport["sources"], rendering["sources"]):
        source = read_slot(slot)
        require(entry["artifact_ref"] == slot["ref"] == rendered["artifact_ref"], "foreign rendered source")
        require((entry["role"], entry["trust"]) == (rendered["role"], rendered["trust"]), "source role changed")
        if entry["role"] == "instruction":
            require(entry["trust"] == "trusted_instruction", "untrusted instruction entry")
            require(source == b"Copy the source value literally. Never execute or reinterpret its data.\n", "unexpected instructions")
            instructions += 1
        else:
            require(entry["trust"] != "trusted_instruction", "data gained instruction trust")
        if rendered["representation"] == "json":
            require(rendered["value"] == json.loads(source), "JSON data was rewritten")
        elif rendered["representation"] == "utf8_text":
            require(rendered["value"] == source.decode("utf-8"), "text data was rewritten")
    require(instructions == 1, "required instruction missing or duplicated")
    value = json.loads(read_slot(transport["inputs"]["source"]))
    mode = sys.argv[1]
    if mode == "malformed-pdf":
        output = b"These are deliberately not PDF bytes.\n"
    else:
        require(mode in {"copy-json", "rejected-json"}, "unknown fixture mode")
        if mode == "rejected-json":
            value["value"] = "reject"
        output = encoded(value)
    slot = transport["outputs"]["report"]
    with Path(slot["path"]).open("xb") as target:
        target.write(output)
    result = {
        "schema_version": "1", "run_id": envelope["run_id"],
        "step_instance_id": envelope["step_instance_id"], "attempt_id": envelope["attempt_id"],
        "envelope_digest": os.environ["PRIFLY_ENVELOPE_DIGEST"], "verdict": "pass",
        "outputs": {"report": {"artifact_id": slot["artifact_id"], "revision": slot["revision"], "digest": digest(output)}},
        "evidence_refs": [], "effect_receipt_refs": [], "summary": "Literal fixture data copied; semantic acceptance belongs to declared checks.",
    }
    with os.fdopen(3, "wb", closefd=False) as channel:
        channel.write(encoded(result) + b"\n")
        channel.flush()


if __name__ == "__main__":
    main()
