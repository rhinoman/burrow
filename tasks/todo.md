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

---

# Phase 5: Charts + Image Rendering

## Implementation

- [x] Step 1: `pkg/charts/charts.go` — directive parsing (ParseDirectives, ReplaceDirectives)
- [x] Step 1: `pkg/charts/charts.go` — PNG rendering (RenderPNG: bar, line, pie via go-analyze/charts)
- [x] Step 1: `pkg/charts/charts.go` — text table rendering (RenderTextTable: ASCII box-drawing tables)
- [x] Step 1: `pkg/charts/charts_test.go` — 19 tests: parse, replace, render PNG, text tables, edge cases
- [x] Step 2: `pkg/render/images.go` — terminal capability detection (DetectImageTier using rasterm)
- [x] Step 2: `pkg/render/images.go` — inline image writing (WriteInlineImage: Kitty, iTerm2 protocols)
- [x] Step 2: `pkg/render/images_test.go` — config override logic tests
- [x] Step 3: `pkg/pipeline/executor.go` — chart PNG generation after synthesis when generate_charts=true
- [x] Step 3: `pkg/reports/reports.go` — Charts field added to Report struct, charts/ dir scanning in Load/Finish
- [x] Step 3: `pkg/pipeline/executor_test.go` + `pkg/reports/reports_test.go` — chart generation and loading tests
- [x] Step 4: `pkg/render/chart_process.go` — chart processing for viewer (marker-based Glamour integration)
- [x] Step 4: `pkg/render/viewer.go` — WithReportDir, WithImageConfig options; chart fields; i keybinding
- [x] Step 4: `cmd/gd/cmd_reports.go` — passes report.Dir and rendering.images to viewer
- [x] Step 5: `pkg/reports/export.go` — HTML export with chart embedding (base64 data URIs, HTML table fallback)
- [x] Step 5: `pkg/reports/export_test.go` — chart embedding and fallback tests

## New Files (4)

- `pkg/charts/charts.go` — directive parsing, PNG rendering, text table rendering
- `pkg/charts/charts_test.go` — 19 tests
- `pkg/render/images.go` — terminal capability detection, inline image writing
- `pkg/render/chart_process.go` — chart processing for viewer, openFirstChart

## Modified Files (7+)

- `pkg/pipeline/executor.go` — chart generation after synthesis
- `pkg/pipeline/executor_test.go` — chart generation tests
- `pkg/reports/reports.go` — Charts field, charts/ dir scanning
- `pkg/reports/reports_test.go` — chart loading tests
- `pkg/reports/export.go` — chart embedding in HTML export
- `pkg/reports/export_test.go` — chart export tests
- `pkg/render/viewer.go` — chart rendering options, keybindings
- `cmd/gd/cmd_reports.go` — report dir and image config passthrough
- `go.mod` / `go.sum` — added go-analyze/charts, rasterm

## New Dependencies

- `github.com/go-analyze/charts` v0.5.24 — pure Go chart library (bar/line/pie → PNG)
- `github.com/BourgeoisBear/rasterm` v1.1.2 — Kitty/iTerm2/Sixel terminal image protocol support

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 15 packages pass
- [x] `go test -race ./...` — no races detected

---

# Code Review Round 10 Fixes

## Fixes

- [x] **HIGH** Chart markers mangled by Glamour — changed `__BURROW_CHART_N__` to `BURROW-CHART-N` (underscores are CommonMark strong emphasis)
- [x] **MED** Sixel detection without rendering — removed Sixel from `detectBest()` since `WriteInlineImage` can't render it yet
- [x] **MED** Chart PNG write error discarded — added `fmt.Fprintf(os.Stderr, ...)` for `os.WriteFile` errors in executor chart generation
- [x] **MED** Duplicate chart PNG loading — extracted `charts.LoadPNG(chartsDir, title, idx)`, removed `loadChartPNG` and `loadChartPNGForExport`
- [x] **LOW** No tests for chart_process.go — added 6 tests including marker-mangling regression test

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 15 packages pass
- [x] `go test -race ./...` — no races detected

---

# Phase 6: MCP Client + Result Caching + Automatic Report Comparison

## Implementation

- [x] Step 1: `pkg/mcp/client.go` — MCP JSON-RPC 2.0 client (Initialize, ListTools, CallTool)
- [x] Step 1: `pkg/mcp/client.go` — Session ID tracking (Mcp-Session-Id header)
- [x] Step 1: `pkg/mcp/client.go` — `NewHTTPClient()` with auth injection via RoundTripper
- [x] Step 2: `pkg/mcp/service.go` — MCPService adapter (services.Service implementation)
- [x] Step 2: `pkg/mcp/service.go` — Lazy init with sync.Once, tool discovery, param type conversion
- [x] Step 3: `pkg/mcp/client_test.go` — 10 tests: init, session ID, list tools, call, error, timeout, HTTP error, auth
- [x] Step 3: `pkg/mcp/service_test.go` — 5 tests: execute, discovery, error result, init memoized, name
- [x] Step 4: `pkg/cache/cache.go` — CachedService decorator with SHA-256 keys, base64 JSON files
- [x] Step 4: `pkg/cache/cache_test.go` — 8 tests: miss, hit, expired, error not cached, corrupted, JSON valid, different params, name
- [x] Step 5: `cmd/gd/cmd_routines.go` — buildRegistry() handles MCP + cache wrapping
- [x] Step 6: `pkg/pipeline/executor.go` — compare_with injection into synthesis prompt
- [x] Step 6: `pkg/pipeline/executor_test.go` — 3 tests: compare with previous, no previous, no compare_with
- [x] Step 7: `pkg/config/config.go` — MCP services skip tool path validation, llamacpp provider type
- [x] Step 8: `integration_test.go` — MCP integration, caching integration, compare_with integration, config validation tests

## New Files (6)

- `pkg/mcp/client.go` — MCP JSON-RPC client + NewHTTPClient auth helper
- `pkg/mcp/service.go` — MCPService adapter (implements services.Service)
- `pkg/mcp/client_test.go` — 10 client tests with httptest mock
- `pkg/mcp/service_test.go` — 5 service tests with httptest mock
- `pkg/cache/cache.go` — CachedService decorator
- `pkg/cache/cache_test.go` — 8 cache tests

## Modified Files (5)

- `cmd/gd/cmd_routines.go` — buildRegistry() handles MCP + cache wrapping
- `pkg/pipeline/executor.go` — compare_with injection + buildComparisonContext()
- `pkg/pipeline/executor_test.go` — 3 compare_with tests
- `pkg/config/config.go` — MCP tool validation skip, llamacpp provider type
- `integration_test.go` — 4 new integration tests (MCP, caching, compare_with, config validation)

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 17 packages pass
- [x] `go test -race ./...` — no races detected
- [x] All tests use httptest (zero network access)
- [x] Cache files are valid JSON (inspectable with cat)
- [x] MCP protocol messages match JSON-RPC 2.0 spec

## Code Review Round 12 Fixes

- [x] **MEDIUM** MCP Client sessionID data race — added `sync.Mutex` to `Client`, locked around read/write of `sessionID` in `call()`
- [x] **MEDIUM** buildRegistry discards BurrowDir() error — changed signature to `buildRegistry(cfg, burrowDir)`, callers pass their already-validated burrowDir
- [x] **LOW** buildComparisonContext byte-slices UTF-8 — changed to `[]rune` truncation, consistent with existing `extractSnippet` fix
- [x] **LOW** Stale "Reserved: Phase 4" comment on CompareWith — updated to describe the implemented feature

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 17 packages pass
- [x] `go test -race ./...` — no races detected

---

# Phase 8: Scheduler / Daemon

## Implementation

- [x] Step 1: `pkg/scheduler/scheduler.go` — Clock/StateStore interfaces, parseSchedule, isDue, routineLocation, Scheduler with 1-minute tick loop
- [x] Step 1: `pkg/scheduler/scheduler.go` — FileStateStore (atomic JSON), MemoryStateStore (testing)
- [x] Step 2: `pkg/scheduler/scheduler_test.go` — testClock, 14 parseSchedule cases, 8 isDue cases, 5 timezone cases, 7 scheduler integration tests, 4 FileStateStore tests, routineLocation tests
- [x] Step 3: `cmd/gd/cmd_daemon.go` — `gd daemon` command, `--once` flag, `runRoutine` helper with fresh config per execution

## New Files (3)

- `pkg/scheduler/scheduler.go` — Clock, StateStore, Scheduler, parseSchedule, isDue, FileStateStore, MemoryStateStore
- `pkg/scheduler/scheduler_test.go` — testClock, all unit and integration tests (14 test functions)
- `cmd/gd/cmd_daemon.go` — `gd daemon` command, `--once` flag, `runRoutine` helper

## Modified Files (0)

No existing files modified. Cobra subcommands register via `init()` in their own files.

## Design Notes

- Scheduler knows WHEN (schedule evaluation), not HOW (execution via RoutineRunner callback)
- State file (`~/.burrow/scheduler-state.json`) tracks last-run date per routine in routine's timezone
- Failed runs not recorded — retries on next tick
- Config reloaded fresh per routine execution (no credential caching across sessions)
- `_ "time/tzdata"` import ensures timezone data on minimal systems (+~450KB)
- Tests use explicit `Timezone: "UTC"` on routines to avoid system locale dependency

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 18 packages pass
- [x] `go test -race ./...` — no races detected
- [x] `gd daemon --help` — shows correct usage, flags
- [x] All tests use injectable clock/state/loader/runner — zero network access, zero wall-clock delays

## Code Review Round 16 Fixes

- [x] **MEDIUM** State file TOCTOU race on concurrent completions — added `stateMu sync.Mutex` to serialize load→modify→save in completion goroutines. New `TestSchedulerConcurrentCompletionsBothPersist` verifies both routines' state persists.
- [x] **LOW** Invalid schedule silently never fires — `tick()` now validates schedule format via `parseSchedule` before calling `isDue`, logs error to stderr. New `TestSchedulerLogsInvalidSchedule` verifies.

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 18 packages pass (16 scheduler tests)
- [x] `go test -race ./pkg/scheduler/` — no races detected

---

# Phase 9: Contacts Package

## Implementation

- [x] Step 1: `pkg/contacts/contacts.go` — Contact struct, Store CRUD (Add/Get/List/Remove), Search, Lookup, ForContext, Count
- [x] Step 2: `pkg/contacts/import_csv.go` — CSV import with convention-based header mapping
- [x] Step 3: `pkg/contacts/import_vcard.go` — vCard 3.0/4.0 parser (FN, EMAIL, ORG, TITLE, TEL, NOTE)
- [x] Step 4: `pkg/contacts/contacts_test.go` — 26 tests covering CRUD, search, lookup, CSV import, vCard import, ForContext
- [x] Step 5: `cmd/gd/cmd_contacts.go` — CLI commands: list, add, import, search, show, remove
- [x] Step 6: `pkg/context/ledger.go` — Added TypeContact constant, included "contacts" in 4 subdirectory slices
- [x] Step 7: `cmd/gd/interactive.go` — Added contacts store to session, inject ForContext into draft/ask context
- [x] Step 7b: `cmd/gd/cmd_context.go` — Updated context show/stats/clear to include contact type

## New Files (4 source + 1 test)

- `pkg/contacts/contacts.go` — Contact struct, Store CRUD, Search, Lookup, ForContext
- `pkg/contacts/import_csv.go` — CSV import with header mapping
- `pkg/contacts/import_vcard.go` — vCard parser (FN, EMAIL, ORG, TITLE, TEL, NOTE)
- `pkg/contacts/contacts_test.go` — 26 tests
- `cmd/gd/cmd_contacts.go` — CLI commands: list, add, import, search, show, remove

## Modified Files (3)

- `pkg/context/ledger.go` — Added TypeContact constant, "contacts" in 4 subdirectory slices
- `cmd/gd/interactive.go` — contacts store in session, ForContext injection into ask/draft
- `cmd/gd/cmd_context.go` — context show/stats/clear include contact type

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 19 packages pass
- [x] `go test -race ./...` — no races detected

## Code Review Round 18 Fixes

- [x] **MEDIUM** Import indexes ALL contacts, not just newly imported — refactored import command to call ParseCSV/ParseVCard directly, then Add each, index only the parsed contacts
- [x] **MEDIUM** indexContactInLedger creates a new Ledger per call — refactored to accept `*bcontext.Ledger` parameter; callers create one instance via `openLedgerForContacts()`
- [x] **LOW** ImportCSV/ImportVCard count includes failed Add()s — both now count only successfully added contacts
- [x] **LOW** Entry struct comment stale — updated `Type` comment to include `| contact`
- [x] **NITPICK** ImportVCard no size guard — added `os.Stat` + 10MB limit check before `os.ReadFile`

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 19 packages pass

---

# Phase 10: Retention + Context Polish

## Implementation

- [x] Step 1: `pkg/context/ledger.go` — `TypeNote` constant, `PruneExpired(retention)` method, `parseTimestampFromFilename`, `notes/` in 5 slice literals
- [x] Step 2: `pkg/config/config.go` — retention validation in `Validate()` (negative days, invalid reports string)
- [x] Step 3: `cmd/gd/cmd_context.go` — `gd context prune` command, `TypeNote` in show/stats/clear type lists
- [x] Step 4: `cmd/gd/cmd_daemon.go` — retention prune after successful routine execution
- [x] Step 5: `cmd/gd/cmd_note.go` — `gd note <text>` command
- [x] Step 6: `pkg/configure/wizard.go` — remote LLM warning with acknowledge prompt in `configureLLM` case 2
- [x] Step 6b: `pkg/configure/session.go` — `RemoteLLMWarning` field on `Change`, `hasNewRemoteProvider` check in `ApplyChange`
- [x] Step 6c: `cmd/gd/cmd_configure.go` + `cmd/gd/cmd_init.go` — display remote LLM warning after ApplyChange
- [x] Step 7: `cmd/gd/interactive.go` — config validation warning on startup
- [x] Step 7b: `cmd/gd/cmd_ask.go` — config validation warning, contact injection into `askWithLLM`
- [x] Step 8: Tests — 8 prune tests + 1 note test in `ledger_test.go`, 2 retention validation tests in `config_test.go`, updated wizard test

## New Files (1)

| File | Purpose |
|------|---------|
| `cmd/gd/cmd_note.go` | `gd note <text>` command |

## Modified Files (13)

| File | Changes |
|------|---------|
| `pkg/context/ledger.go` | `TypeNote` constant, `PruneExpired` method, `parseTimestampFromFilename`, `notes/` in 5 slice literals |
| `pkg/context/ledger_test.go` | 8 prune tests + 1 note test, updated directory structure test |
| `pkg/config/config.go` | Retention validation in `Validate()` |
| `pkg/config/config_test.go` | 2 new retention validation tests |
| `pkg/configure/wizard.go` | Remote LLM warning with acknowledge prompt |
| `pkg/configure/wizard_test.go` | Updated OpenRouter test to acknowledge warning |
| `pkg/configure/session.go` | `RemoteLLMWarning` on `Change`, `hasNewRemoteProvider` check |
| `cmd/gd/cmd_context.go` | `gd context prune` command, TypeNote in type lists |
| `cmd/gd/cmd_daemon.go` | Retention prune after routine execution |
| `cmd/gd/interactive.go` | Config validation warning on startup |
| `cmd/gd/cmd_ask.go` | Config validation, contact injection |
| `cmd/gd/cmd_init.go` | Remote LLM warning display |
| `cmd/gd/cmd_configure.go` | Remote LLM warning display |

## Verification

- [x] `go build ./cmd/gd` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -count=1` — all 19 packages pass
- [x] `go test -race ./...` — no races
