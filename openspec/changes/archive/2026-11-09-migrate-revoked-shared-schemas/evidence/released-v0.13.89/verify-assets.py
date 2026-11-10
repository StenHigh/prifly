import base64
import datetime
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile

root = Path(__file__).resolve().parent
assets = root / 'assets'
public_key = '521250b29cb38544a7ec7fc1a2ee12b738cbcc3aca78d3bdc860ea438b5435b4'
manifest_bytes = (assets / 'release-manifest.json').read_bytes()
manifest = json.loads(manifest_bytes)
assert set(manifest) == {'schema_version', 'version', 'stable', 'assets'}
assert manifest['schema_version'] == 'prifly-release/1'
assert manifest['version'] == '0.13.89' and manifest['stable'] is True
assert {(a['os'], a['arch']) for a in manifest['assets']} == {('linux', 'amd64'), ('darwin', 'arm64')}
for asset in manifest['assets']:
    assert set(asset) == {'os', 'arch', 'archive', 'binary', 'sha256'}
    assert all(type(v) is str and v.isascii() for v in asset.values())
# This closed manifest contains ASCII strings, arrays and a boolean only.
# Sorted compact JSON is its RFC 8785 form; no numbers or Unicode sorting occur.
jcs = json.dumps(manifest, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()
(root / 'public-key.der').write_bytes(bytes.fromhex('302a300506032b6570032100' + public_key))
for signature_name, data in [('release-manifest.jcs.sig', jcs), ('release-manifest.sig', manifest_bytes.rstrip(b'\n'))]:
    signed = root / (signature_name + '.input')
    signature = root / (signature_name + '.raw')
    signed.write_bytes(data)
    signature.write_bytes(base64.b64decode((assets / signature_name).read_bytes().strip(), validate=True))
    subprocess.run(['openssl', 'pkeyutl', '-verify', '-pubin', '-keyform', 'DER', '-inkey', str(root / 'public-key.der'), '-rawin', '-in', str(signed), '-sigfile', str(signature)], check=True)
asset = next(a for a in manifest['assets'] if (a['os'], a['arch']) == ('linux', 'amd64'))
assert asset['archive'] == 'prifly-linux-amd64.tar.gz' and asset['binary'] == 'prifly'
archive = assets / asset['archive']
assert hashlib.sha256(archive.read_bytes()).hexdigest() == asset['sha256']
with tarfile.open(archive, 'r:gz') as bundle:
    members = bundle.getmembers()
    assert len(members) == 1 and members[0].name == 'prifly'
    assert members[0].isfile() and 0 < members[0].size <= 96 * 1024 * 1024
    binary_bytes = bundle.extractfile(members[0]).read()
binary = root / 'prifly'
with binary.open('xb') as output:
    output.write(binary_bytes)
binary.chmod(0o755)
version = json.loads(subprocess.run([str(binary), 'version', '--json'], check=True, capture_output=True, text=True).stdout)
record = dict(status='passed', release_tag='v0.13.89', release_sha='30c3635081f8f012aef3953072dcd45d0807d618', checked_at=datetime.datetime.now(datetime.timezone.utc).isoformat(), public_key_hex=public_key, public_key_source='https://api.github.com/repos/StenHigh/prifly/actions/variables/PRIFLY_RELEASE_PUBLIC_KEY', signatures=['release-manifest.jcs.sig', 'release-manifest.sig'], manifest_sha256=hashlib.sha256(manifest_bytes).hexdigest(), archive=asset['archive'], archive_sha256=asset['sha256'], binary_sha256=hashlib.sha256(binary_bytes).hexdigest(), binary_version=version, asset_url='https://github.com/StenHigh/prifly/releases/download/v0.13.89/' + asset['archive'])
(root / 'provenance.json').write_text(json.dumps(record, indent=2) + '\n')
print(json.dumps(record, indent=2))
