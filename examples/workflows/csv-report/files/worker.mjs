import { readFileSync, writeFileSync } from 'node:fs';
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
const read = port => readFileSync(context.inputs[port].path, 'utf8');
const output = (port, text) => {
  const slot = context.outputs[port];
  const bytes = Buffer.from(text);
  writeFileSync(slot.path, bytes);
  result.outputs[port] = {
    artifact_id: slot.artifact_id, revision: slot.revision,
    digest: 'sha256:' + createHash('sha256').update(bytes).digest('hex'),
  };
};

try {
  switch (process.argv[2]) {
    case 'parse': {
      // Deliberately small example format, not a general-purpose CSV parser:
      // exact header, unquoted ASCII names, nonnegative decimal integers.
      const lines = read('csv').replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n');
      if (lines.shift() !== 'name,amount' || lines.length === 0 || lines.length > 10000) {
        throw new Error('Expected name,amount header and 1–10000 rows');
      }
      const rows = lines.map(line => {
        const match = /^([A-Za-z][A-Za-z0-9 _-]*),(0|[1-9][0-9]*)$/.exec(line);
        if (!match || !Number.isSafeInteger(Number(match[2]))) {
          throw new Error('Expected an unquoted name and a nonnegative safe integer');
        }
        return { name: match[1], amount: Number(match[2]) };
      });
      output('rows', JSON.stringify({ rows }) + '\n');
      result.summary = `Parsed ${rows.length} rows`;
      break;
    }
    case 'validate': {
      const data = JSON.parse(read('rows'));
      const names = new Set(data.rows.map(row => row.name));
      const total = data.rows.reduce((sum, row) => sum + row.amount, 0);
      if (names.size !== data.rows.length || !Number.isSafeInteger(total)) {
        throw new Error('Names must be unique and the total must be a safe integer');
      }
      output('rows', JSON.stringify(data) + '\n');
      result.summary = 'Unique names and safe total verified';
      break;
    }
    case 'report': {
      const { rows } = JSON.parse(read('rows'));
      const total = rows.reduce((sum, row) => sum + row.amount, 0);
      output('report', `Pri-Fly CSV report\nRows: ${rows.length}\nTotal: ${total}\n`);
      result.summary = 'Report produced';
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
