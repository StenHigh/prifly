"""Wrapper/process contract checks, independent of Pri-Fly runtime E2E tests."""

import hashlib
import http.server
import json
import os
from pathlib import Path
import socketserver
import subprocess
import sys
import tempfile
import threading
import unittest


ROOT = Path(__file__).resolve().parents[2]
EXAMPLES = ROOT / "examples"
FOUNDATION = ROOT / "test" / "fixtures" / "foundation"
AUTHORING_SCHEMAS = ROOT / "schemas" / "authoring"
BOOTSTRAP = "import os,sys; fd=int(os.environ['TEST_RESULT_FD']); os.dup2(fd,3); os.execv(sys.argv[1],sys.argv[1:])"


class WrapperContractTest(unittest.TestCase):
    def run_step(self, mode, data=None):
        with tempfile.TemporaryDirectory(prefix="prifly-wrapper-", dir="/tmp") as temporary:
            root = Path(temporary)
            publications = []

            class Handler(http.server.BaseHTTPRequestHandler):
                def log_message(self, *_):
                    pass

                def do_POST(self):
                    if self.path != "/publish" or self.headers.get("Authorization") != "Bearer test-token":
                        self.send_error(403)
                        return
                    command = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                    publications.append(command)
                    body = json.dumps({"publication": {"state_version": command.get("expected_state_version", 0) + 1}}).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)

            socket_path = root / "publish.sock"
            with socketserver.UnixStreamServer(str(socket_path), Handler) as server:
                thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)
                thread.start()
                try:
                    envelope = {"schema_version": "1", "run_id": "run:test", "step_instance_id": "step:test", "attempt_id": "attempt:test"}
                    inputs = {}
                    outputs = {}
                    if mode != "shell":
                        input_path = root / "input"
                        input_path.write_bytes(data)
                        port = "source" if mode == "transform" else "document"
                        inputs[port] = {"ref": {"artifact_id": "artifact:input", "revision": 1, "digest": "sha256:" + hashlib.sha256(data).hexdigest()}, "path": str(input_path)}
                        ports = ["text", "report"] if mode == "transform" else ["report"]
                        outputs = {name: {"artifact_id": "artifact:" + name, "revision": 1, "path": str(root / "outputs" / name)} for name in ports}
                    context = {"schema_version": "local-context/1", "inputs": inputs, "outputs": outputs, "dependencies": []}
                    context_path = root / "context.json"
                    context_path.write_text(json.dumps(context))
                    read_fd, write_fd = os.pipe()
                    env = {"PATH": "/usr/bin:/bin", "PRIFLY_CONTEXT_FILE": str(context_path), "PRIFLY_SOCKET": str(socket_path), "PRIFLY_TOKEN": "test-token",
                           "PRIFLY_RUN_ID": "run:test", "PRIFLY_STEP_ID": "step:test", "PRIFLY_ATTEMPT_ID": "attempt:test", "PRIFLY_ENVELOPE_DIGEST": "sha256:" + "a" * 64,
                           "TEST_RESULT_FD": str(write_fd), "PYTHONDONTWRITEBYTECODE": "1"}
                    command = ["/bin/sh", str(FOUNDATION / "empty-step.sh")] if mode == "shell" else [sys.executable, str(FOUNDATION / "demo-step.py"), mode]
                    try:
                        process = subprocess.Popen([sys.executable, "-c", BOOTSTRAP, *command], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                                   env=env, cwd=root, pass_fds=(write_fd,))
                    finally:
                        os.close(write_fd)
                    try:
                        stdout, stderr = process.communicate(json.dumps(envelope).encode(), timeout=10)
                    except BaseException:
                        process.kill()
                        process.wait()
                        raise
                    with os.fdopen(read_fd, "rb") as result_channel:
                        result = result_channel.read(1 << 20)
                    self.assertEqual(process.returncode, 0, stderr.decode())
                    self.assertEqual(stdout, b"", "result must be carried on fd3, not stdout")
                    result = json.loads(result)
                    output_bytes = {name: Path(slot["path"]).read_bytes() for name, slot in outputs.items()}
                    for name, content in output_bytes.items():
                        self.assertEqual(result["outputs"][name]["digest"], "sha256:" + hashlib.sha256(content).hexdigest())
                    return result, output_bytes, publications
                finally:
                    server.shutdown()
                    thread.join(timeout=2)

    def test_transform_and_progress(self):
        result, outputs, publications = self.run_step("transform", "Hello, Pri-Fly!\n".encode())
        self.assertEqual(result["verdict"], "pass")
        self.assertEqual(outputs["text"], b"HELLO, PRI-FLY!\n")
        self.assertTrue(json.loads(outputs["report"])["changed"])
        self.assertEqual([item["expected_state_version"] for item in publications], [0, 1])
        self.assertEqual(publications[-1]["value"], {"phase": "finished", "completed": 1})

    def test_warning_does_not_change_pass(self):
        result, _, publications = self.run_step("transform", b"ALREADY UPPERCASE\n")
        self.assertEqual(result["verdict"], "pass")
        events = [item for item in publications if item["kind"] == "event"]
        self.assertEqual([item["event_key"] for item in events], ["unchanged_text"])

    def test_negative_verdict_is_valid_result(self):
        result, outputs, publications = self.run_step("check", json.dumps({"key": "empty", "text": ""}).encode())
        self.assertEqual(result["verdict"], "fail")
        self.assertFalse(json.loads(outputs["report"])["ok"])
        self.assertEqual([item["event_key"] for item in publications if item["kind"] == "event"], ["empty_document"])

    def test_shell_uses_same_result_contract(self):
        result, outputs, publications = self.run_step("shell")
        self.assertEqual(result["verdict"], "pass")
        self.assertEqual(outputs, {})
        self.assertEqual(publications, [])

    def test_setup_preserves_existing_directory(self):
        with tempfile.TemporaryDirectory(prefix="prifly-demo-", dir="/tmp") as temporary:
            user_file = Path(temporary) / "user.txt"
            user_file.write_text("untouched")
            result = subprocess.run([sys.executable, str(FOUNDATION / "create-demo.py"), "--binary", sys.executable, "--target", temporary], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)
            self.assertIn("empty directory", result.stderr)
            self.assertEqual(user_file.read_text(), "untouched")


class EditorContractTest(unittest.TestCase):
    def test_local_schemas_and_reference_modelines_are_published_together(self):
        manifest = json.loads((AUTHORING_SCHEMAS / "manifest.json").read_text())
        self.assertEqual(manifest["schema_version"], "prifly-yaml-editor-contract/1")
        expected = {
            "project-profile",
            "project-profile-v2",
            "project-profile-v3",
            "project-workflow-folder-v1",
            "extension-v1",
            "run-decision-v1",
            "workflow-v1",
            "step-v1",
            "check-v1",
            "context-v1",
            "workflow-catalog-v1",
        }
        self.assertEqual({item["document"] for item in manifest["schemas"]}, expected)
        for item in manifest["schemas"]:
            self.assertEqual(Path(item["path"]).name, item["path"])
            if item["document"] in {"project-profile-v2", "project-profile-v3"}:
                self.assertEqual(item["patterns"], [])
            else:
                self.assertTrue(item["patterns"])
            schema = json.loads((AUTHORING_SCHEMAS / item["path"]).read_text())
            self.assertEqual(schema["$schema"], "https://json-schema.org/draft/2020-12/schema")
            self.assertEqual(schema["$id"], item["id"])
            self.assertTrue(schema["title"])
        self.assertEqual(
            (EXAMPLES / "authoring" / "workflow-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/workflow-v1.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "step-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/step-v1.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "project-profile-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/project-profile-v3.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "check-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/check-v1.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "execution-bindings-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/project-workflow-folder-v1.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "context-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/context-v1.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "run-decision-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/run-decision-v1.schema.json",
        )
        self.assertEqual(
            (EXAMPLES / "authoring" / "workflow-catalog-authoring-reference.yaml").read_text().splitlines()[0],
            "# yaml-language-server: $schema=../../schemas/authoring/workflow-catalog-v1.schema.json",
        )


if __name__ == "__main__":
    unittest.main()
