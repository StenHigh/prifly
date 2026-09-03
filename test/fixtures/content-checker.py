#!/usr/bin/env python3
"""Deterministic check-request/1 fixture, never a Step or publication client.

Pass is limited to fixture-value/1 JSON or its producer's exact StepResult.
The PDF mode only rejects definitely malformed bytes; it never qualifies PDF.
"""

import hashlib
import json
import os
from pathlib import Path
import sys


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def unique_object(pairs):
    value = {}
    for key, item in pairs:
        require(key not in value, "duplicate JSON key")
        value[key] = item
    return value


def parse(data):
    def invalid_constant(value):
        raise ValueError("invalid JSON constant: " + value)
    return json.loads(data, object_pairs_hook=unique_object, parse_constant=invalid_constant)


def read_slot(slot):
    path = Path(slot["path"])
    require(not path.is_absolute() and ".." not in path.parts, "nonlocal source path")
    data = path.read_bytes()
    require(digest(data) == slot["ref"]["digest"], "source bytes differ from exact reference")
    return data


def main():
    raw = sys.stdin.buffer.read((1 << 20) + 1)
    require(len(raw) <= 1 << 20, "oversized check request")
    request = parse(raw)
    required = set("schema_version check_execution_id run_id workflow_invocation_id boundary check_ref workflow_ref policy_ref admission_id admitted_run_version control_epoch package_lock_digest subjects context_manifest_ref dispatch_not_after check_deadline".split())
    optional = {"stage_activation_id", "producer_attempt_id", "port", "candidate_ref"}
    require(required <= request.keys() <= required | optional, "request is not the closed check contract")
    require(request["schema_version"] == "check-request/1", "unsupported request")
    require(request["check_execution_id"] == os.environ["PRIFLY_CHECK_EXECUTION_ID"], "foreign check")
    require(request["run_id"] == os.environ["PRIFLY_RUN_ID"], "foreign Run")
    require(digest(raw) == os.environ["PRIFLY_REQUEST_DIGEST"], "request bytes changed")
    # The shared process launcher also exposes an unprivileged stdin hash under
    # its historical PRIFLY_ENVELOPE_DIGEST name. This checker never uses it or
    # FD3; its own request digest and stdout CheckResult are the contract.
    require(not any(os.environ.get(key) for key in ("PRIFLY_STEP_ID", "PRIFLY_ATTEMPT_ID", "PRIFLY_SOCKET", "PRIFLY_TOKEN")), "checker received Step credentials")
    boundary = request["boundary"]
    require(boundary in {"workflow_input", "step_input", "step_output", "step_result", "workflow_output"}, "unknown boundary")
    require(("stage_activation_id" in request) == (boundary != "workflow_input"), "wrong activation ownership")
    require(("producer_attempt_id" in request) == (boundary in {"step_output", "step_result"}), "wrong producer ownership")
    require(("candidate_ref" in request) == (boundary == "step_result"), "wrong candidate ownership")
    require(("port" in request) == (boundary != "step_result"), "wrong content port")
    require(len(request["subjects"]) <= 256 and (boundary == "step_result" or len(request["subjects"]) == 1), "wrong subject count")
    for name in ("check_ref", "workflow_ref", "policy_ref"):
        require(set(request[name]) == {"id", "version", "digest"}, "non-exact definition reference")
    refs = request["subjects"] + [request["context_manifest_ref"]]
    if "candidate_ref" in request:
        refs.append(request["candidate_ref"])
    for ref in refs:
        require(set(ref) == {"artifact_id", "revision", "digest"} and ref["revision"] >= 1, "non-exact artifact reference")
    transport = parse(Path(os.environ["PRIFLY_CONTEXT_FILE"]).read_bytes())
    require(transport["schema_version"] == "local-context/2" and not transport["outputs"], "wrong checker transport")
    manifest = parse(read_slot(transport["manifest"]))
    rendering = parse(read_slot(transport["rendering"]))
    require(transport["manifest"]["ref"] == request["context_manifest_ref"], "foreign manifest")
    require(rendering["schema_version"] == "context-rendering/1" and "envelope" not in rendering, "fabricated execution envelope")
    require(rendering["check_request"] == request and rendering["check_request_digest"] == digest(raw), "rendered request identity mismatch")
    require(rendering["manifest"] == manifest, "foreign rendered manifest")
    require(len(manifest["entries"]) == len(transport["sources"]), "incomplete source list")
    for entry, slot in zip(manifest["entries"], transport["sources"]):
        read_slot(slot)
        require(entry["artifact_ref"] == slot["ref"], "manifest source mismatch")
        require(entry["role"] == "data" and entry["trust"] != "trusted_instruction", "checked data became instructions")
    subjects = []
    for index, ref in enumerate(request["subjects"]):
        slot = transport["inputs"][f"subject_{index:03d}"]
        require(slot["ref"] == ref, "wrong check subject")
        subjects.append(read_slot(slot))
    if "candidate_ref" in request:
        slot = transport["inputs"]["candidate"]
        require(slot["ref"] == request["candidate_ref"], "wrong candidate input")
        candidate = parse(read_slot(slot))
        require(candidate["schema_version"] == "1" and candidate["run_id"] == request["run_id"] and candidate["attempt_id"] == request["producer_attempt_id"], "foreign producer candidate")
        require(list(candidate["outputs"].values()) == request["subjects"], "unbound candidate outputs")
        require(candidate["verdict"] == "pass", "unexpected fixture producer verdict")
    mode = sys.argv[1]
    if mode == "malformed-pdf":
        require(boundary == "step_output", "PDF fixture used at another boundary")
        status = "fail" if not subjects[0].startswith(b"%PDF-") else "inconclusive"
        limitations = ["Only a missing PDF header is definitely rejected. No PDF parser, conformance, safety or general content qualification is implemented."]
    else:
        require(mode in {"fixture-json", "inconclusive"}, "unknown fixture method")
        values = [parse(data) for data in subjects]
        valid = all(set(value) == {"schema_version", "value", "data"} and value["schema_version"] == "fixture-value/1" and value["value"] == "ok" and isinstance(value["data"], str) for value in values)
        status = "pass" if valid else "fail"
        if mode == "inconclusive":
            require(boundary == "step_result" and valid, "inconclusive fixture requires a valid producer candidate")
            status = "inconclusive"
        limitations = ["Pass applies only to the declared fixture-value/1 test format and exact bound candidate; it is not a general semantic review."]
    result = {
        "schema_version": "check-result/1", "check_execution_id": request["check_execution_id"],
        "run_id": request["run_id"], "request_digest": digest(raw), "status": status,
        "summary": "Deterministic fixture method: " + mode, "limitations": limitations,
    }
    sys.stdout.buffer.write(json.dumps(result, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode() + b"\n")


if __name__ == "__main__":
    main()
