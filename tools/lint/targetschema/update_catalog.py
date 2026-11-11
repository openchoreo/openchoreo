#!/usr/bin/env python3
"""Reproduce or verify the pinned offline Kubernetes OpenAPI catalog.

Validation never runs this development tool. Downloads require --download;
--input regenerates from an already available artifact, and --verify is offline.
"""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import urllib.request


def main():
    directory = Path(__file__).resolve().parent
    existing = json.loads((directory / 'catalog.json').read_text())
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--verify', action='store_true')
    mode.add_argument('--input', type=Path)
    mode.add_argument('--download', action='store_true')
    parser.add_argument('--version', default=existing['version'])
    parser.add_argument('--sha256', help='Required when changing the pinned release')
    args = parser.parse_args()
    version = args.version if args.version.startswith('v') else 'v' + args.version
    if not re.fullmatch(r'v1\.\d+\.\d+', version):
        parser.error('Expected an exact Kubernetes v1.x.y release')
    source = 'https://raw.githubusercontent.com/kubernetes/kubernetes/%s/api/openapi-spec/swagger.json' % version
    expected = args.sha256 or (existing['sha256'] if version == existing['version'] else None)
    if not expected:
        parser.error('Changing releases requires an independently verified --sha256 pin')
    artifact = directory / 'kubernetes-openapi.json.gz'
    if args.verify:
        raw = gzip.decompress(artifact.read_bytes())
    elif args.input:
        raw = args.input.read_bytes()
    else:
        with urllib.request.urlopen(source, timeout=60) as response:
            raw = response.read()
    actual = hashlib.sha256(raw).hexdigest()
    if actual != expected:
        raise SystemExit('Catalog SHA256 mismatch: expected %s, received %s' % (expected, actual))
    document = json.loads(raw)
    if document.get('swagger') != '2.0' or not document.get('definitions'):
        raise SystemExit('Expected the upstream Kubernetes Swagger 2.0 definitions catalog')
    keys = sorted({(gvk['group'], gvk['version'], gvk['kind'])
                   for definition in document['definitions'].values()
                   for gvk in definition.get('x-kubernetes-group-version-kind', [])})
    if not keys:
        raise SystemExit('No Kubernetes GVK declarations were found')
    mod = (directory.parents[2] / 'go.mod').read_text()
    dependency = re.search(r'k8s.io/apimachinery\s+v0\.(\d+\.\d+)', mod)
    if not dependency or version != 'v1.' + dependency.group(1):
        raise SystemExit('Catalog release must match the pinned k8s.io/apimachinery dependency')
    if not args.verify:
        compressed = bytearray(gzip.compress(raw, mtime=0))
        compressed[9] = 255  # Stable OS header across Python/platform versions.
        artifact.write_bytes(compressed)
        manifest = {'version': version, 'source': source, 'sha256': actual,
                    'gvkCount': len(keys), 'definitions': len(document['definitions'])}
        (directory / 'catalog.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print('PASS %s: %d indexed GVKs, %d definitions, SHA256 %s' %
          (version, len(keys), len(document['definitions']), actual))


if __name__ == '__main__':
    main()
