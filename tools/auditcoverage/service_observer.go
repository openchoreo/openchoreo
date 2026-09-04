// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

// observerKnownNonEvents is observer's closing narrative — see
// apiKnownNonEvents.
const observerKnownNonEvents = "## Known non-events\n\n" +
	"Cases where a real state-modifying request produces no audit event, or a diminished one, " +
	"by design or by a documented, unfixed gap:\n\n" +
	"- **Reads are out of scope.** Every GET operation, and every `Query*` POST-as-read operation " +
	"(see RESTExemptions).\n" +
	"- **Port 8081 (alert-rule CRUD, the alert webhook) has no authentication**, so every " +
	"operation there is exempted rather than audited-as-anonymous — see RESTExemptions. Coverage " +
	"becomes automatic the moment that port gets auth; the middleware is already wired there.\n" +
	"- **A missing or unparseable FinOps `startTime`/`endTime` never reaches audit.** " +
	"oapi-codegen's generated wrapper binds query parameters and calls `ErrorHandlerFunc` before " +
	"`HandlerMiddlewares` run, so such a 400 bypasses both audit middlewares as well as the " +
	"access log. Both FinOps operations are GETs and unaudited, so no coverage is lost — but a " +
	"rejection there leaves only the `ParamBindingErrorHandler` log line.\n" +
	"- **An authentication rejection carries no `action`, `category` or `operation_id`.** " +
	"`NewUnauthenticatedMiddleware` emits with a nil Operation, so every operation-derived policy " +
	"selector short-circuits. Such an event is selectable only by `origins`, `results`, " +
	"`actor_types` and `actors`.\n" +
	"- **observer's MCP server has no mutating tools** (see MCPToolNames' doc comment), so unlike " +
	"openchoreo-api there is no MCP audit wiring or MCP section in this matrix — only an " +
	"unauthenticated-rejection audit on `/mcp`, stamped `origin: mcp`.\n"
