"""Two ordinary deterministic steps; no network, AI or project integration."""

import json
import sys

from prifly_step import Step


def main():
    step = Step()
    step.publish("progress_changed", "state", {"phase": "working", "completed": 0})
    operation = sys.argv[1]
    if operation == "transform":
        original = step.input_bytes("source").decode("utf-8")
        transformed = original.upper().encode("utf-8")
        changed = transformed != original.encode("utf-8")
        step.output_bytes("text", transformed)
        step.output_json("report", {"bytes": len(transformed), "changed": changed})
        if not changed:
            step.publish("warning_raised", "event", {"reason": "unchanged_text"}, event_key="unchanged_text")
        verdict = "pass"
    elif operation == "check":
        document = json.loads(step.input_bytes("document"))
        ok = bool(document["text"].strip())
        notes = [] if ok else ["Document text is empty."]
        step.output_json("report", {"ok": ok, "notes": notes})
        if not ok:
            step.publish("warning_raised", "event", {"reason": "empty_document"}, event_key="empty_document")
        elif len(document["text"]) < 12:
            step.publish("warning_raised", "event", {"reason": "short_document"}, event_key="short_document")
        verdict = "pass" if ok else "fail"
    else:
        raise ValueError("expected transform or check")
    step.publish("progress_changed", "state", {"phase": "finished", "completed": 1})
    step.complete(verdict, "Declared outputs are ready for core validation.")


if __name__ == "__main__":
    main()
