#!/usr/bin/env python3
"""Observer preset resolution and pinned consumer fixture integrity."""
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('light_projection', ROOT / 'scripts/light-projection.py')
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class LightProjectionTests(unittest.TestCase):
    def test_pinned_consumer_bytes(self):
        root = ROOT / 'test/fixtures/light/consumer'
        manifest = json.loads((root / 'manifest.json').read_text())
        self.assertEqual(manifest['source_sha'], '3499115956903abd264fbde2be2f432d07bc2c43')
        self.assertGreaterEqual(len(manifest['files']), 13)
        for name, record in manifest['files'].items():
            body = (root / name).read_bytes()
            self.assertEqual(hashlib.sha256(body).hexdigest(), record['sha256'], name)
            self.assertEqual(hashlib.sha1(b'blob ' + str(len(body)).encode() + b'\0' + body).hexdigest(), record['git_blob'], name)

    def test_resolver_uses_exact_pins_and_declared_versions(self):
        presets = json.loads((ROOT / 'examples/light/projection-presets.json').read_text())
        pairs = {(row['workflow_id'], row['workflow_version']) for row in presets['workflows']}
        defaults = presets['review_defaults']
        for row in presets['workflows']:
            for review in row.get('reviews', []):
                item = {**defaults, **review}
                for key in ['report', 'gate']:
                    pairs.add((item[key + '_schema_id'], item[key + '_schema_version']))
        refs = [{'ref': {'id': identity, 'version': version, 'digest': 'sha256:' + hashlib.sha256(identity.encode()).hexdigest()}}
                for identity, version in sorted(pairs)]
        result = MODULE.build({'run': {'definitions': refs}}, presets)
        self.assertEqual(len(result['workflows']), len(presets['workflows']))
        for row in result['workflows']:
            self.assertTrue(row['workflow_ref']['digest'].startswith('sha256:'))
            for review in row['reviews']:
                self.assertIn('report_schema', review)
                self.assertIn('gate_schema', review)
                self.assertNotIn('report_schema_id', review)
        with self.assertRaisesRegex(ValueError, 'no exact supported'):
            MODULE.build({'run': {'definitions': []}}, presets)
        derived = [{'ref': {**entry['ref'], 'version': '0.0.0-b1.' + 'a' * 52}} for entry in refs]
        provenance = {'schema_version': 'prifly-build-provenance/1', 'algorithm': 'b1',
                      'components': [{'author_ref': source['ref'], 'compiled_ref': dest['ref']}
                                     for source, dest in zip(refs, derived)]}
        resolved = MODULE.build({'run': {'definitions': derived}}, presets, provenance)
        self.assertTrue(all(row['workflow_ref']['version'].startswith('0.0.0-b1.') for row in resolved['workflows']))
        composed = {**provenance, 'schema_version': 'prifly-build-provenance/2', 'composition': {}}
        self.assertEqual(resolved, MODULE.build({'run': {'definitions': derived}}, presets, composed))
        duplicate = {'ref': {**refs[0]['ref'], 'digest': 'sha256:' + 'f' * 64}}
        with self.assertRaisesRegex(ValueError, 'ambiguous'):
            MODULE.build({'run': {'definitions': refs + [duplicate]}}, presets)


if __name__ == '__main__':
    unittest.main()
