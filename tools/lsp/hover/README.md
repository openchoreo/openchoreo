# Context-aware field hover

Field prose is resolved by API version, resource kind and full schema path.
The embedded `field_descriptions.json` is generated from the repository CRDs:

```sh
# From the OpenChoreo Go repository root:
go run ./tools/lsp/hover/cmd/generate
go test ./tools/lsp/hover
```

The generator traverses properties, array items (`[]`) and map-value schemas
(`*`). Its output is deterministic and a consistency test checks it against the
CRDs. Regenerate it when API documentation changes. The existing versioned lint
templates still supply types, constraints and required-field information and
remain unchanged. Catalog text describes the repository's matching API version;
it is not a separately versioned documentation archive for every template release.

Exact paths take precedence over wildcard paths; wildcard ties use sorted paths.
Unknown fields never borrow descriptions by leaf name. Standard root/metadata
paths have neutral fallbacks; otherwise missing prose is omitted. Type information
remains available where the selected lint template describes the field.

The YAML document under the cursor determines the resource identity. Array paths
are normalized using the selected structural schema. Both direct field prose and
child-field tables use the same contextual lookup. Existing kind overview links
remain unchanged. Free-form fields without a structural hover schema may still
have no hover; their documentation is not guessed.

Changes are limited to hover lookup, generated documentation, generator and
regression tests. CLI, validation, completion, navigation, RuleEngine and versioned
templates are unchanged. The previous hover test expecting secret-specific text
on generic metadata now expects neutral resource metadata text.

## Delivery verification

- Catalog: 37 API-kind entries, 1,552 described paths; consistency test passed.
- `go test ./tools/lsp/hover/... ./tools/lsp/... -count=1 -timeout 120s`:
  passed, including completion, navigation, CodeLens and diagnostic lifecycle
  integration coverage (integration suite: 12.151s).
- `go build -o /tmp/lsp-context-hover ./cmd/openchoreo-lsp`: passed.
- Real `textDocument/hover` requests to the deployed extension LSP passed for
  ComponentType, Trait and Workflow reference values; unrelated Workflow prose
  was absent from ComponentType/Trait results.
- `git diff --check` and protected RuleEngine/template diff checks passed.
- All three deployed LSP copies were replaced atomically. Extension TypeScript
  was unchanged, so no extension rebuild was required.
- Interactive VS Code hover appearance was not manually verified. Reload the
  VS Code window to restart its LSP process.

Modified/added files: `hover.go`, `hover_test.go`, `context_test.go`,
`field_descriptions.json`, `internal/catalog/catalog.go`,
`cmd/generate/main.go` and this README. Tests cover contextual lookup, unknown
fallbacks, array entries, keys/values, multi-document selection and reproducible
catalog generation. Early tests exposed the obsolete secret-specific metadata
expectation and missing array-path normalization; both were corrected before the
successful final run. Sandbox-denied cache/build attempts were rerun with approved
cache access and are not counted as successful builds.

## CEL field suggestions and hover

CEL member hover now runs through the existing validator's expression walk,
using its actual environment at the cursor (including `forEach` and post-render
`resource` targets). For example, `metadata.podSelectors` shows its CEL map type
and the production MetadataContext documentation. Misspelled known fields such as
`metadata.podSelectores` retain `cel-type-error` and add `Did you mean
"podSelectors"?`. Root-variable suggestions retain `cel-unknown-name`.

Candidates come from the active CEL provider/declarations; there is no manual
list of allowed fields. Kubernetes' declaration provider does not enumerate
fields through `FindStructFieldNames`, so the checker reads its existing
`DeclType.Fields` when necessary. Suggestions require a unique nearby spelling;
open map keys and dynamic subtrees are not restricted.

Production context descriptions are generated from Go comments and JSON tags:

```sh
go run ./tools/lint/cel-validator/cmd/generate-docs
```

The generated catalog is embedded in the shared Go CEL package and checked for
consistency by tests. Parameter/environment-configuration descriptions come from
the active user schema; post-render resource descriptions come from the resolved
Kubernetes schema. Missing prose is omitted instead of invented. No rendering,
expression evaluation or network lookup is performed by hover.

The LSP uses current editor text and supplies its workspace schema resolver to
hover, including unsaved workspace CRDs. The tooltip highlights the identifier
under the cursor and maps Unicode columns to LSP UTF-16 positions. Quoted, block,
CRLF and multi-document YAML retain the existing parser/source handling.

Current symbol selection covers contiguous identifier/dotted-member paths,
including typed `forEach` variables. Indexed/optional selections, comprehension
local variables, and repeated or escaped noncontiguous paths may have no specific
CEL tooltip or spelling hint; they keep existing CEL validation and ordinary YAML
hover. Function-call names are not misrepresented as object fields, and text
inside CEL string literals/comments is excluded. A tooltip is an ordinary VS Code
hover, not a modal message box or an automatic edit/Quick Fix.

Changed sources for this addition: shared CEL `validate.go`, `symbols.go`,
`symbol_descriptions.go`, generated `context_descriptions.json`, its
`cmd/generate-docs`/`internal/contextdocs` generator and `symbols_test.go`;
hover `hover.go`, `cel_hover.go`, `cel_hover_test.go`, `verify_cel_hover.py` and
this README; and `tools/lsp/server.go` for the workspace resolver. The hover
selection fallback also covers content lines in a final YAML block scalar,
because the parser's existing document range ends at the scalar header.

Binary verification:

```sh
python3 tools/lsp/hover/verify_cel_hover.py --cli /path/to/occ --lsp /path/to/openchoreo-lsp
```

This tests the actual CLI diagnostic, automatic LSP diagnostic, typo tooltip,
corrected field type/documentation, and an unsaved schema-description update.
It uses temporary fixtures and never invokes Play or modifies the demo project.

### CEL addition verification

Executed successfully:

```sh
env NO_COLOR= go test ./internal/occ/cmd/lint ./tools/lint/cel-validator/... ./tools/lint/pre-render-validator ./tools/lint/ruleengine/... ./tools/lsp/... -count=1 -timeout 180s
go test -race ./tools/lint/cel-validator ./tools/lsp/hover -run 'TestConcurrentSymbol|TestContextField|TestCELSymbol|TestCELIdentifier|TestPostRenderCEL|TestInspectionAndSuggestion' -count=1 -timeout 120s
go build -o /tmp/occ-cel-symbols ./cmd/occ
go build -o /tmp/lsp-cel-symbols ./cmd/openchoreo-lsp
python3 tools/lsp/hover/verify_cel_hover.py --cli ../bin/occ --lsp ../../vscode-extension/OpenChoreoToolkit/bin/openchoreo-lsp
```

The full regression command exited zero (CLI 112.411s, LSP integration 30.354s).
The race tests exited zero for both packages. The actual deployed binaries passed
all five protocol checks: CLI suggestion, automatic single LSP diagnostic, typo
hover, corrected field/schema documentation, and unsaved-description updates.
Unit coverage includes preserved map keys, quoted strings, schema fields, typed
loop variables, block scalars, CRLF, UTF-16 ranges and documentation regeneration.
Concurrent inspections and validations share cached environments without mutating
provider-owned candidate slices.

Both CLI and LSP were atomically replaced in all three deployment directories:
`occ_with_linter/bin`, `occ_with_linter/openchoreo/bin` and
`vscode-extension/OpenChoreoToolkit/bin`. Copies of each executable have identical
SHA256 checksums. Protected RuleEngine/template and whitespace diff checks passed.
No extension TypeScript changes were needed. A sandbox-denied LSP cache access was
rerun with approved build-cache access; denied attempts were not counted as
successful builds. The initial regression failures exposed Kubernetes provider
field-enumeration behavior and the final-block-scalar source boundary; these were
resolved before the successful runs above.

Interactive VS Code UI behavior was not manually verified. Reload the VS Code
window to restart its LSP and try hovering over `${metadata.podSelectors}`.

### Discovering CEL children

CEL tooltips now include a sorted table of all declared direct children, with
qualified field paths and types. Hover over `metadata`
to discover `metadata.name`, `metadata.namespace`, `metadata.podSelectors` and
its other declared fields. Hover over a leaf such as `metadata.name` to see its
parent's children. Nested object parents show their own children. Typed lists of
objects show element fields using `[]` paths. Open maps explain that keys are
dynamic instead of inventing a finite set of valid keys.

These are schema/type definitions, not actual rendered runtime values. Child
names come from the same active CEL provider used for validation. There is no
hardcoded metadata field list or arbitrary row cap. Expand another nested parent
by hovering its identifier; the tooltip lists direct children, not an unbounded
recursive tree. The child table contains only Field and Type columns; the
selected field retains its full description above it.

Child-list verification passed:

- `go test ./tools/lint/cel-validator/... ./tools/lsp/... -count=1 -timeout 120s`
  (including LSP integration: 21.508s).
- Focused CEL/hover tests passed again after adding typed-list element description
  coverage. Metadata child counts and order are compared against the active CEL
  provider, rather than merely checking three hardcoded examples.
- `go test -race ./tools/lint/cel-validator ./tools/lsp/hover -run 'TestSymbolChildren|TestCELHoverLists|TestConcurrentSymbol' -count=1 -timeout 120s` passed.
- Actual requests to the deployed LSP verified child lists on both parent and
  leaf identifiers, plus the existing typo, correction and unsaved-schema cases.
- CLI/LSP builds passed and all six deployed executable copies were atomically
  updated. A sandbox-denied build-cache access was rerun with approved access.
- RuleEngine/template and whitespace diff checks passed. Interactive VS Code UI
  verification remains unperformed.

This child-list change extends `symbols.go`, `symbol_descriptions.go`,
`symbols_test.go`, `cel_hover.go`, `cel_hover_test.go`, `verify_cel_hover.py` and
this README. No resource-specific child names were added to production code.
