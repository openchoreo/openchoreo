# Language server

Build the unified executable with `make go.build.occ` or `go build -o occ ./cmd/occ`.

Authenticate with `occ login`, then start the server with `occ lsp --stdio`.
Editors should launch `occ` with arguments `["lsp", "--stdio"]`.
The standalone `openchoreo-lsp` entry point has been removed.

LSP startup and `occ lint vali` (also `occ linter validate`) use the existing CLI authentication check and token refresh. Missing or unrefreshable credentials require `occ login`; login is never launched automatically. Existing security-disabled environments remain supported. Help and version commands do not require authentication.

Logs default to stderr; use `--log-level` and `--log-file` to configure them. Stdout carries only JSON-RPC messages. `--clientProcessId` remains accepted for client compatibility.

Implementations remain separate under `tools/lsp` and `tools/lint`.
