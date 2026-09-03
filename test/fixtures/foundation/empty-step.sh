#!/bin/sh
# A process in another language. Core-provided IDs/digest use the closed ASCII
# identifier grammar; this command reads no arbitrary shell text from input.
set -eu
cat >/dev/null
printf '{"schema_version":"1","run_id":"%s","step_instance_id":"%s","attempt_id":"%s","envelope_digest":"%s","verdict":"pass","outputs":{},"evidence_refs":[],"effect_receipt_refs":[],"summary":"Shell process completed."}\n' \
  "$PRIFLY_RUN_ID" "$PRIFLY_STEP_ID" "$PRIFLY_ATTEMPT_ID" "$PRIFLY_ENVELOPE_DIGEST" >&3
