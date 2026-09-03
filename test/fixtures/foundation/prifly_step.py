"""Optional stdlib-only wrapper for the local cooperative process contract.

The core does not import or require this module. IDs and paths come from the
sealed envelope/local context. Publications are observations, not lifecycle
changes. The worker never opens Pri-Fly's state database.
"""

import hashlib
import http.client
import json
import os
from pathlib import Path
import socket
import sys
import uuid


def json_bytes(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode()


class _LocalHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=5)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


class Step:
    def __init__(self):
        raw = sys.stdin.buffer.read((2 << 20) + 1)
        if len(raw) > 2 << 20:
            raise ValueError("execution envelope is too large")
        self.envelope = json.loads(raw)
        if self.envelope["schema_version"] != "1":
            raise ValueError("unsupported execution envelope")
        self.context = json.loads(Path(os.environ["PRIFLY_CONTEXT_FILE"]).read_text())
        if self.context["schema_version"] != "local-context/1":
            raise ValueError("unsupported local context")
        for field, variable in (
            ("run_id", "PRIFLY_RUN_ID"),
            ("step_instance_id", "PRIFLY_STEP_ID"),
            ("attempt_id", "PRIFLY_ATTEMPT_ID"),
        ):
            if self.envelope[field] != os.environ[variable]:
                raise ValueError("execution identity mismatch")
        self.outputs = {}
        self.state_versions = {}

    def input_bytes(self, port):
        """Read a declared materialized input; absent optional input is None."""
        item = self.context["inputs"].get(port)
        if item is None:
            return None
        data = Path(item["path"]).read_bytes()
        if "sha256:" + hashlib.sha256(data).hexdigest() != item["ref"]["digest"]:
            raise ValueError("materialized input digest mismatch")
        return data

    def output_bytes(self, port, data):
        slot = self.context["outputs"][port]
        path = Path(slot["path"])
        path.parent.mkdir(parents=True, exist_ok=True)
        # Each output is written once. Core independently seals and validates it.
        with path.open("xb") as target:
            target.write(data)
        self.outputs[port] = {
            "artifact_id": slot["artifact_id"],
            "revision": slot["revision"],
            "digest": "sha256:" + hashlib.sha256(data).hexdigest(),
        }

    def output_json(self, port, value):
        self.output_bytes(port, json_bytes(value))

    def publish(self, hook, kind, value, *, event_key=None):
        command = {
            "schema_version": "1",
            "command_id": "command:" + uuid.uuid4().hex,
            "run_id": self.envelope["run_id"],
            "step_instance_id": self.envelope["step_instance_id"],
            "attempt_id": self.envelope["attempt_id"],
            "envelope_digest": os.environ["PRIFLY_ENVELOPE_DIGEST"],
            "hook": hook,
            "kind": kind,
            "value": value,
        }
        if kind == "state":
            command["expected_state_version"] = self.state_versions.get(hook, 0)
        elif kind == "event" and event_key:
            command["event_key"] = event_key
        else:
            raise ValueError("state or event with a stable event_key is required")
        connection = _LocalHTTP(os.environ["PRIFLY_SOCKET"])
        try:
            connection.request("POST", "/publish", json_bytes(command), {
                "Content-Type": "application/json",
                "Authorization": "Bearer " + os.environ["PRIFLY_TOKEN"],
            })
            response = connection.getresponse()
            body = response.read((1 << 20) + 1)
            if len(body) > 1 << 20 or response.status < 200 or response.status >= 300:
                raise RuntimeError("publication rejected with HTTP status " + str(response.status))
            receipt = json.loads(body)
        finally:
            connection.close()
        if kind == "state":
            self.state_versions[hook] = command["expected_state_version"] + 1
        return receipt

    def complete(self, verdict="pass", summary=""):
        if verdict not in {"pass", "fail", "needs_revision", "no_work"}:
            raise ValueError("unsupported StepResult verdict")
        result = {
            "schema_version": "1",
            "run_id": self.envelope["run_id"],
            "step_instance_id": self.envelope["step_instance_id"],
            "attempt_id": self.envelope["attempt_id"],
            "envelope_digest": os.environ["PRIFLY_ENVELOPE_DIGEST"],
            "verdict": verdict,
            "outputs": self.outputs,
            "evidence_refs": [],
            "effect_receipt_refs": [],
            "summary": summary,
        }
        with os.fdopen(3, "wb", closefd=False) as channel:
            channel.write(json_bytes(result) + b"\n")
            channel.flush()
