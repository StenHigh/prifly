import json, os, sys
from pathlib import Path
raw = sys.stdin.buffer.read((2 << 20) + 1)
if len(raw) > 2 << 20: raise RuntimeError("oversized envelope")
e = json.loads(raw)
c = json.loads(Path(os.environ["PRIFLY_CONTEXT_FILE"]).read_text())
if json.loads(Path(c["inputs"]["left"]["path"]).read_text()) != {"value": 7}: raise RuntimeError("left input mismatch")
if json.loads(Path(c["inputs"]["right"]["path"]).read_text()) != {"enabled": True}: raise RuntimeError("right input mismatch")
r = dict(schema_version="1", run_id=e["run_id"], step_instance_id=e["step_instance_id"],
         attempt_id=e["attempt_id"], envelope_digest=os.environ["PRIFLY_ENVELOPE_DIGEST"],
         verdict="pass", outputs={}, evidence_refs=[], effect_receipt_refs=[],
         summary="Both exact shared schemas were consumed by a native process")
with os.fdopen(3, "w", closefd=False) as f:
    json.dump(r, f, separators=(",", ":")); f.write("\n"); f.flush()
