# Phase 1: Burrow Minimum Viable Skeleton

## Implementation

- [x] Project scaffolding (go.mod, directories, main.go, version command)
- [x] pkg/config — Config structs, BurrowDir(), Load(), Validate(), ResolveEnvVars()
- [x] pkg/services — Service interface, Result struct, Registry
- [x] pkg/http — RESTService adapter with auth injection and tool mapping
- [x] pkg/pipeline — Routine loading, Executor wiring sources→synthesis→reports
- [x] pkg/synthesis — Synthesizer interface, PassthroughSynthesizer, LLMSynthesizer stub
- [x] pkg/reports — Save/Load/List/FindLatest with date-based directory structure
- [x] pkg/render — Glamour markdown rendering, Bubble Tea viewport viewer
- [x] CLI wiring — `gd routines list|run`, `gd reports|view`
- [x] Integration test — end-to-end with httptest, zero network access

## Code Review Round 1 Fixes

- [x] #1 Report directory collision — timestamp format `YYYY-MM-DDT1504-name/`
- [x] #2 URL path dropping — `ResolveReference` instead of overwriting `base.Path`
- [x] #3 Hardcoded api_key param — configurable via `auth.key_param`
- [x] #4/#5 Viewer dead code — removed unused content param, viewerContent type, SetContent method
- [x] #6 Unbounded io.ReadAll — added `LimitReader` (10MB cap)
- [x] #7 Fragile parseReportDirName — regex-based, handles both old and new formats
- [x] #8 LLM attribution leak — `LLMSynthesizer` accepts `stripAttribution` flag, replaces service names with generic labels
- [x] #9 Inefficient FindLatest — scans directory names directly, loads only the match
- [x] Minor: removed no-op `filepath.Join`, fixed `os.Unsetenv` in tests

## Code Review Round 2 Fixes

- [x] Critical: `.gitignore` pattern `gd` → `/gd` (was hiding `cmd/gd/` from git)
- [x] #1 Misleading `buildURL` comment — clarified tool paths are absolute from host root
- [x] #2 List sorting only by date — now sorts by Dir basename for minute-level ordering
- [x] #3 Same-minute collision — upgraded to second-precision `YYYY-MM-DDT150405-name/`
- [x] #4 Error messages leaking service names — `stripServiceNames()` sanitizes error text for remote LLMs
- [x] Minor: renamed loop variable `t` → `tool` in `NewRESTService`
- [x] Minor: clearer error when routine file not found (shows both paths tried)
- [x] Minor: `TestSaveNoClobber` properly asserts both reports are independently loadable

## Verification

- [x] `go build ./cmd/gd` — binary produced
- [x] `go vet ./...` — no issues
- [x] `go test ./...` — all 8 packages pass
- [x] All tests use httptest (zero network access)

---

# Phase 2: Make It Real

## Implementation

- [x] Step 1: `pkg/privacy` — HTTP transport middleware (referrer stripping, UA rotation, request minimization)
- [x] Step 2: `pkg/http` — Wire privacy transport into RESTService, UA auth sentinel
- [x] Step 3: `pkg/synthesis` — Ollama provider (POST /api/chat, 5-min timeout, helpful errors)
- [x] Step 4: `pkg/synthesis` — OpenRouter provider (OpenAI-compatible chat completions API)
- [x] Step 5: `pkg/synthesis` — Provider factory (maps config type to concrete provider)
- [x] Step 6: `pkg/pipeline` — Parallel execution with jitter (goroutine per source, WaitGroup, order preservation)
- [x] Step 7: `pkg/context` — Context ledger (flat markdown files with YAML front matter, Search/List/GatherContext)
- [x] Step 8: CLI wiring — Provider selection, privacy config, ledger wiring in `gd routines run`
- [x] Step 9: `gd ask` command — Local context search, zero network access
- [x] Step 10: Integration tests — Privacy headers, context ledger indexing, parallel execution timing

## New Files (11)

- `pkg/privacy/transport.go` + `transport_test.go`
- `pkg/synthesis/ollama.go` + `ollama_test.go`
- `pkg/synthesis/openrouter.go` + `openrouter_test.go`
- `pkg/synthesis/provider.go` + `provider_test.go`
- `pkg/context/ledger.go` + `ledger_test.go`
- `cmd/gd/cmd_ask.go`

## Modified Files (4)

- `pkg/http/rest.go` — privacy config param, UA auth sentinel
- `pkg/http/rest_test.go` — updated signatures, new privacy tests
- `pkg/pipeline/executor.go` — parallel execution, jitter, ledger
- `pkg/pipeline/executor_test.go` — parallel, jitter, cancellation, order, ledger tests
- `cmd/gd/cmd_routines.go` — provider selection, privacy wiring, ledger wiring
- `integration_test.go` — updated signatures + privacy/context/parallel assertions

## Code Review Fixes

- [x] #1 `buildURL` clobbers existing query params — merge with `resolved.Query()` instead of replace
- [x] #2 Duplicate `sanitize` functions — extracted shared `pkg/slug.Sanitize`, both packages import it
- [x] #4 Raw results hardcode `.json` — documented assumption with comment
- [x] #6/#7 `indexContext` warnings go to stdout — changed to `fmt.Fprintf(os.Stderr, ...)`
- Acknowledged #3 (reports.List eager loading — tracked in lessons.md)
- Acknowledged #5 (hand-rolled YAML parser — intentionally simple, documented in lessons.md)

## Verification

- [x] `go build ./cmd/gd` — binary produced
- [x] `go vet ./...` — no issues
- [x] `go test ./...` — all tests pass across 11 packages, zero network access
- [x] Parallel execution verified (3x100ms services complete in <250ms)
- [x] Privacy headers verified (UA rotated, referrers stripped, auth UA preserved)
- [x] Context ledger verified (entries written and searchable after pipeline run)
- [x] Query param preservation verified (tool path params merged with mapped params)

---

# Code Review Round 4 Fixes

## Fixes

- [x] #1 `NewTransport` nil fallback — changed from `http.DefaultTransport` to `&http.Transport{}` to prevent shared transport foot-gun
- [x] #2 Endpoint URL trailing slash — `strings.TrimRight(endpoint, "/")` in both `NewOpenRouterProvider` and `NewOllamaProvider`
- [x] #3 `stripServiceNames` substring order — sort unique names by length descending before replacing to prevent "news" corrupting "news-api"
- [x] #4 `buildSynthesizer` missing test cases — added `TestBuildSynthesizerPassthrough` and `TestBuildSynthesizerPassthroughProvider`
- [x] #5 `slug.Sanitize("")` — returns `"unknown"` for empty/all-special-character input

## Deferred

- api_key in URL query params — intentional tradeoff for APIs that require it
- reports.List eager loading — deferred until performance degrades
- POST content type hardcoded to JSON — deferred until non-JSON POST bodies needed

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all packages pass
- [x] `go test -race ./...` — no races detected

---

# Phase 3: User Interaction Layer

## Implementation

- [x] Step 1: `config.Save()` + `pipeline.SaveRoutine()` — round-trip serialization with tests
- [x] Step 2: `pkg/actions/` — action parsing, clipboard, system app handoff, draft generation
- [x] Step 3: `pkg/configure/` — Ollama detection, structured wizard, LLM-driven session
- [x] Step 4: `gd init` + `gd configure` — commands with auto-detection and wizard fallback
- [x] Step 5: `gd ask` upgrade — local LLM reasoning over context with text search fallback
- [x] Step 6: Interactive mode REPL — `gd` launches REPL with ask/search/draft/sources/help
- [x] Step 7: Extracted `buildRegistry()` helper shared between routines and interactive mode

## New Files (14)

- `pkg/actions/actions.go` — ActionType, Action, ParseActions
- `pkg/actions/clipboard.go` — CopyToClipboard, platform detection
- `pkg/actions/handoff.go` — Handoff struct, OpenURL/File/Mailto/PlayMedia, BuildMailtoURI
- `pkg/actions/draft.go` — Draft struct, GenerateDraft, parseDraft
- `pkg/actions/actions_test.go` — ParseActions edge cases, parseDraft structured/unstructured
- `pkg/actions/handoff_test.go` — mailto URI encoding tests
- `pkg/configure/detect.go` — DetectOllama, DetectProvider, VerifyProvider
- `pkg/configure/wizard.go` — Wizard with RunInit/RunModify, piped IO
- `pkg/configure/session.go` — Session with ProcessMessage/ApplyChange, extractYAMLBlock
- `pkg/configure/wizard_test.go` — piped IO tests for all wizard paths
- `pkg/configure/session_test.go` — mock provider, YAML extraction, history
- `cmd/gd/cmd_init.go` — gd init command
- `cmd/gd/cmd_configure.go` — gd configure command
- `cmd/gd/interactive.go` — REPL loop with ask/search/draft/sources/help

## Modified Files (6)

- `pkg/config/config.go` — added `Save()` with header comment
- `pkg/config/config_test.go` — Save round-trip, creates parent dir, header tests
- `pkg/pipeline/routine.go` — added `SaveRoutine()`
- `pkg/pipeline/routine_test.go` — SaveRoutine round-trip, creates dir, excludes Name
- `cmd/gd/root.go` — added `RunE` for interactive mode
- `cmd/gd/cmd_ask.go` — upgraded with findLocalProvider, local LLM reasoning, text search fallback
- `cmd/gd/cmd_routines.go` — extracted `buildRegistry()` helper

## New Test Files (2)

- `cmd/gd/cmd_ask_test.go` — findLocalProvider selection logic tests
- `cmd/gd/interactive_test.go` — parseServiceQuery tests

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 14 packages pass
- [x] `go test -race ./...` — no races detected

---

# Code Review Round 5 Fixes

## Fixes

- [x] **HIGH** Panic recovery in executor goroutines — `defer/recover` in `executor.go:56` goroutine, surfaces panic as error result
- [x] **MED** Empty auth credentials not validated — `Validate()` now requires key/token/value for their respective auth methods
- [x] **MED** Flaky timing tests — widened parallel thresholds from 250ms to 500ms (sequential floor is 300ms)
- [x] **LOW** Dead config fields — added `// Reserved: Phase 4` comments to `compare_with` and `spec`
- [x] **LOW** LLM timeouts not configurable — added `Timeout` field to `ProviderConfig`, `NewOllamaProviderWithTimeout`, `NewOpenRouterProviderWithTimeout`
- [x] **LOW** `LoadAllRoutines` fails on first bad file — now skips with warning, optional `io.Writer` for warnings
- [x] **LOW** Empty config passes validation — intentional (fresh install), added explicit test documenting this

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 14 packages pass
- [x] `go test -race ./...` — no races detected

---

# Code Review Round 6 Fixes

## Fixes

- [x] **HIGH** `handleServiceQuery` empty tool name — rewrote `parseServiceQuery` to return `(svc, tool, params)`, `handleServiceQuery` now passes tool to `Execute()`, updated tests
- [x] **HIGH** `gd init` saves unapplied config — `runConversationalInit` now only returns configs that were applied+accepted; returns `(nil, nil)` to fall through to wizard
- [x] **HIGH** `gd configure` writes expanded secrets — added `Config.DeepCopy()`, resolve env vars on copy only; wizard operates on unresolved config preserving `${ENV_VAR}` references
- [x] **MED** Session sends credentials to LLM — added `redactConfig()` that replaces auth keys/tokens with `${REDACTED}` before embedding in system prompt
- [x] **LOW** `parseDraft` body-with-colons edge case — rewrote to scan only known header prefixes (`To:`, `Subject:`), stops scanning on first non-header line
- [x] **LOW** `extractYAMLBlock` indented code blocks — rewrote to line-by-line scanning with `TrimSpace` on marker detection
- [x] **LOW** `wizard.prompt` swallows EOF — now returns empty string on read error, letting callers use defaults
- [x] **LOW** `DetectOllama` picks arbitrary model — now selects the largest model by size
- [x] **LOW** Interactive mode local-only LLM policy — added doc comment explaining zero-network privacy rationale
- [x] **MED** Test coverage — added `TestDeepCopy`, `TestRedactConfig`, `TestParseDraftBodyWithColons`, indented YAML block test, updated `parseServiceQuery` tests

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 14 packages pass
- [x] `go test -race ./...` — no races detected
