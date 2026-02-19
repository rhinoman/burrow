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
