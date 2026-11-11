#!/usr/bin/env python3
"""Verify saved CLI/LSP parity and unsaved CRD lifecycle using real binaries."""
import argparse
import json
import pathlib
import queue
import shutil
import subprocess
import tempfile
import threading
import time


class Client:
    def __init__(self, binary):
        self.process = subprocess.Popen([binary], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        self.messages = queue.Queue()
        threading.Thread(target=self.read, daemon=True).start()

    def read(self):
        while True:
            length = None
            while True:
                line = self.process.stdout.readline()
                if not line:
                    return
                if line == b'\r\n':
                    break
                if line.lower().startswith(b'content-length:'):
                    length = int(line.split(b':', 1)[1])
            if length is not None:
                self.messages.put(json.loads(self.process.stdout.read(length)))

    def send(self, method, params, request=None):
        message = {'jsonrpc': '2.0', 'method': method, 'params': params}
        if request is not None:
            message['id'] = request
        data = json.dumps(message).encode()
        self.process.stdin.write(f'Content-Length: {len(data)}\r\n\r\n'.encode() + data)
        self.process.stdin.flush()

    def wait(self, predicate, timeout=30):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            try:
                message = self.messages.get(timeout=max(.01, deadline-time.monotonic()))
            except queue.Empty:
                break
            if predicate(message):
                return message
        raise AssertionError('Timed out waiting for LSP response/diagnostics')

    def diagnostics(self, uri, count, version=None):
        def matches(message):
            params = message.get('params', {})
            return (message.get('method') == 'textDocument/publishDiagnostics'
                    and params.get('uri') == uri
                    and (version is None or params.get('version') == version)
                    and len([d for d in params.get('diagnostics', []) if d.get('code') in ('cel-type-error', 'cel-unknown-name')]) == count)
        return self.wait(matches)


def cli(binary, root):
    result = subprocess.run([binary, 'linter', 'vali', '-obj', '-detect', str(root)], capture_output=True, text=True)
    # Existing CLI permits presentation text around its JSON payload.
    raw = result.stdout
    start = raw.find('{')
    if start < 0:
        start = raw.find('[')
    if start < 0:
        raise AssertionError((result.returncode, raw, result.stderr))
    payload, _ = json.JSONDecoder().raw_decode(raw[start:])
    def findings(value):
        if isinstance(value, dict):
            if value.get('ruleId') in ('cel-type-error', 'cel-unknown-name'):
                yield {'message': value['message']['text']}
            elif value.get('code') in ('cel-type-error', 'cel-unknown-name'):
                yield value
            else:
                for child in value.values():
                    yield from findings(child)
        elif isinstance(value, list):
            for child in value:
                yield from findings(child)
    return list(findings(payload))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cli', required=True)
    parser.add_argument('--lsp', required=True)
    args = parser.parse_args()
    fixture = pathlib.Path(__file__).parent / 'testdata/workspace'
    with tempfile.TemporaryDirectory(prefix='openchoreo-schema-protocol-') as temp:
        root = pathlib.Path(temp)
        shutil.copytree(fixture, root, dirs_exist_ok=True)
        path = root / 'project/componenttype.yaml'
        crd_path = root / 'platform/widget-crd.yaml'
        original, crd = path.read_text(), crd_path.read_text()
        client = Client(args.lsp)
        try:
            client.send('initialize', {'processId': None, 'rootUri': root.as_uri(), 'capabilities': {}, 'workspaceFolders': [{'uri': root.as_uri(), 'name': 'schema-test'}]}, 1)
            client.wait(lambda m: m.get('id') == 1)
            client.send('initialized', {})
            def open_doc(file, text, version):
                client.send('textDocument/didOpen', {'textDocument': {'uri': file.as_uri(), 'languageId': 'yaml', 'version': version, 'text': text}})
            def change(file, text, version):
                client.send('textDocument/didChange', {'textDocument': {'uri': file.as_uri(), 'version': version}, 'contentChanges': [{'text': text}]})
            open_doc(path, original, 1)
            message = client.diagnostics(path.as_uri(), 1)
            saved = cli(args.cli, root)
            lsp = [d for d in message['params']['diagnostics'] if d.get('code') in ('cel-type-error', 'cel-unknown-name')]
            assert len(saved) == 1, saved
            assert saved[0]['message'] == lsp[0]['message'], (saved, lsp)
            print('PASS saved workspace CRD: CLI/LSP message parity')
            open_doc(crd_path, crd, 1)
            change(crd_path, crd.replace('count:', 'coutn:', 1), 2)
            client.diagnostics(path.as_uri(), 0)
            client.send('textDocument/didClose', {'textDocument': {'uri': crd_path.as_uri()}})
            client.diagnostics(path.as_uri(), 1)
            print('PASS unsaved CRD change clears dependent error; close restores saved schema')
            deployment = original.replace('group: example.com', 'group: apps').replace('kind: Widget\n      rule:', 'kind: Deployment\n      rule:')
            cases = [('resource.spec.replicas <= 2', 0), ('resource.specc.replicas <= 2', 1), ('resource.spec.repclicas <= 2', 1), ('resocurce.spec.replicas <= 2', 1)]
            for version, (expression, count) in enumerate(cases, 2):
                text = deployment.replace('resource.spec.coutn <= 2', expression)
                path.write_text(text)
                change(path, text, version)
                client.diagnostics(path.as_uri(), count, version)
                assert len(cli(args.cli, root)) == count, expression
                print('PASS CLI + automatic LSP:', expression)
        finally:
            client.process.terminate()
            client.process.wait(timeout=10)


if __name__ == '__main__':
    main()
