#!/usr/bin/env python3
"""Resolve an explicit observer preset against a saved, sealed Run view.

Reads only the supplied JSON and bundled preset data. It never queries Git,
starts a workflow, changes producer packages or selects a newest version.
"""
import argparse
import json
from pathlib import Path
import sys


def build(view, presets, provenance=None):
    run = view.get('run', view)
    definitions = list(run.get('definitions') or [])
    if run.get('workflow_ref'):
        definitions.append({'ref': run['workflow_ref'], 'bytes': run.get('workflow')})
    refs = {}
    for entry in definitions:
        ref = entry['ref']
        key = (ref['id'], ref['version'])
        if key in refs and refs[key] != ref:
            raise ValueError(f'ambiguous pinned ref: {key}')
        refs[key] = ref

    if provenance is not None:
        if provenance.get('schema_version') not in ('prifly-build-provenance/1', 'prifly-build-provenance/2') or provenance.get('algorithm') != 'b1':
            raise ValueError('unsupported build provenance')
        for mapping in provenance['components']:
            author, compiled = mapping['author_ref'], mapping['compiled_ref']
            if compiled['id'] != author['id']:
                raise ValueError('build mapping changed component identity')
            actual = refs.get((compiled['id'], compiled['version']))
            if actual is None:
                continue
            if actual != compiled:
                raise ValueError('compiled ref differs from sealed Run')
            key = (author['id'], author['version'])
            if key in refs and refs[key] != compiled:
                raise ValueError(f'ambiguous authored ref: {key}')
            refs[key] = compiled

    def resolve(identity, version):
        if (identity, version) not in refs:
            raise ValueError(f'missing pinned ref: {identity}@{version}')
        return refs[(identity, version)]

    workflows = []
    for preset in presets['workflows']:
        key = (preset['workflow_id'], preset['workflow_version'])
        if key not in refs:
            continue
        reviews = []
        for row in preset.get('reviews', []):
            rule = dict(presets['review_defaults'])
            rule.update(row)
            report_id = rule.pop('report_schema_id')
            report_version = rule.pop('report_schema_version')
            gate_id = rule.pop('gate_schema_id')
            gate_version = rule.pop('gate_schema_version')
            rule['report_schema'] = resolve(report_id, report_version)
            rule['gate_schema'] = resolve(gate_id, gate_version)
            reviews.append(rule)
        workflows.append({'workflow_ref': refs[key], 'phases': preset.get('phases', []), 'reviews': reviews})
    if not workflows:
        raise ValueError('no exact supported workflow/version in the supplied sealed view')
    return {'schema_version': 'prifly-light-projection/1', 'workflows': workflows}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-view', type=Path, required=True)
    parser.add_argument('--presets', type=Path, default=Path(__file__).resolve().parents[1] / 'examples/light/projection-presets.json')
    parser.add_argument('--output', type=Path)
    parser.add_argument('--build-provenance', type=Path, help='Explicit verified build-provenance.json for derived component versions')
    args = parser.parse_args()
    try:
        value = build(json.loads(args.run_view.read_text()), json.loads(args.presets.read_text()),
                      json.loads(args.build_provenance.read_text()) if args.build_provenance else None)
        body = json.dumps(value, ensure_ascii=False, indent=2) + '\n'
        if args.output:
            with args.output.open('x') as file:
                file.write(body)
        else:
            sys.stdout.write(body)
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, f'Light projection: {error}\n')


if __name__ == '__main__':
    main()
