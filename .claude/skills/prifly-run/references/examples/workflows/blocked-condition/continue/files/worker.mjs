import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';

// stdin is an ExecutionEnvelope; context.json supplies materialized port paths.
// FD 3 is the result channel, while stdout/stderr remain diagnostics.
const envelope = JSON.parse(readFileSync(0, 'utf8'));
const context = JSON.parse(readFileSync(process.env.PRIFLY_CONTEXT_FILE, 'utf8'));
const result = {
  schema_version: '1',
  run_id: envelope.run_id,
  step_instance_id: envelope.step_instance_id,
  attempt_id: envelope.attempt_id,
  envelope_digest: process.env.PRIFLY_ENVELOPE_DIGEST,
  verdict: 'pass', outputs: {}, evidence_refs: [], effect_receipt_refs: [],
  summary: '',
};
const read = port => JSON.parse(readFileSync(context.inputs[port].path, 'utf8'));
const output = (port, value) => {
  const slot = context.outputs[port];
  const bytes = Buffer.from(JSON.stringify(value) + '\n');
  writeFileSync(slot.path, bytes);
  result.outputs[port] = {
    artifact_id: slot.artifact_id, revision: slot.revision,
    digest: 'sha256:' + createHash('sha256').update(bytes).digest('hex'),
  };
};

// The condition is whatever this workflow needs before it can act; here, a
// file the owner or a test stand creates. Its path comes from the machine's
// local settings: project local set --env CONDITION_FILE=/absolute/path.
const condition = process.env.CONDITION_FILE;

try {
  switch (process.argv[2]) {
    case 'check': {
      const { subject } = read('request');
      if (!condition) {
        throw new Error('CONDITION_FILE is not set for this machine');
      }
      if (existsSync(condition)) {
        output('result', { subject, condition_ref: condition });
        result.summary = `The condition holds; ${subject} is done`;
      } else {
        result.verdict = 'blocked';
        output('obstacle', {
          reason_code: 'condition_absent',
          summary: `${subject} needs ${condition}, and it does not exist`,
          condition_ref: condition,
          observations: [`checked ${condition} at attempt ${envelope.attempt_id}: absent`],
          remaining_work: `all of ${subject}: nothing was done`,
        });
        result.summary = 'Blocked: the condition this step needs is absent';
      }
      break;
    }
    case 'prepare': {
      const { subject } = read('request');
      output('work', { subject });
      result.summary = `Prepared ${subject}`;
      break;
    }
    case 'remedy': {
      const obstacle = read('obstacle');
      result.summary = `Recorded ${obstacle.reason_code} on ${obstacle.condition_ref}; this step may change nothing`;
      break;
    }
    default:
      throw new Error('Unknown worker operation');
  }
} catch (error) {
  result.verdict = 'fail';
  result.outputs = {};
  result.summary = error.message;
}
writeFileSync(3, JSON.stringify(result) + '\n');
