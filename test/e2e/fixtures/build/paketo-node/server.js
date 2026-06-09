// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
//
// Minimal HTTP server used by the e2e paketo-buildpacks-builder spec. The
// paketo node buildpack auto-detects this via `npm start`. The server replies
// 200 on every path so the in-cluster reachability probe is deterministic.

const http = require('http');
const port = parseInt(process.env.PORT || '8080', 10);

http.createServer((_req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/plain' });
  res.end('paketo-node e2e ok\n');
}).listen(port, () => {
  console.log(`paketo-node e2e listening on :${port}`);
});
