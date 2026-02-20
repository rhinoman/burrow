# Lessons Learned

## .gitignore patterns match at all directory levels
- Pattern `gd` matches `cmd/gd/main.go` — use `/gd` to anchor to repo root
- Always verify with `git check-ignore -v <path>` after writing .gitignore rules

## url.ResolveReference with absolute paths
- `url.ResolveReference` with an absolute path (e.g., `/search`) replaces the base path entirely
- This is correct RFC behavior — document the convention (tool paths are absolute from host root) rather than fighting the semantics

## Timestamp precision in filenames
- Minute-precision (`T1504`) still allows collisions in tests and edge cases
- Second-precision (`T150405`) is cheap and eliminates practical collisions

## Attribution stripping must cover error messages
- Error strings like `service not found: "sam-gov"` leak service names
- When stripping attribution for remote LLMs, sanitize error text too, not just labels

## Sort by full identifier, not extracted subfield
- Sorting reports by `Date` (YYYY-MM-DD) loses intra-day ordering
- Sort by directory basename which has full timestamp for free

## Future: attribution stripping scope
- Currently strips service names from labels and error messages
- Tool names (e.g., `search_opportunities`) could also be identifying — decide policy before wiring real LLM
- Raw response data (`r.Data`) is passed verbatim — spec likely means labels, not data content
- `stripServiceNames` does substring replacement — safe for typical names like "sam-gov", but short names like "api" could corrupt text

## Future: reports.List performance
- Currently loads every report.md from disk — fine for Phase 1
- Consider lazy loading or metadata cache before users accumulate hundreds of reports

## Future: injectable clock for tests
- `TestSaveNoClobber` sleeps 1s for second-precision uniqueness — correct but slow
- Inject a clock interface if test suite grows

## buildURL must merge, not replace, query params
- `resolved.RawQuery = query.Encode()` clobbers query params from the tool path
- Tool paths can contain static query params (e.g., `/search?type=active`)
- Fix: use `resolved.Query()` to get existing params, then `Set()` mapped params on top
- `applyAuth` for `api_key` already does this correctly (calls `req.URL.Query()`)

## Don't duplicate utility functions across packages
- Two packages (`reports` and `context`) had independent `sanitize` functions with different behavior
- `reports` allowed underscores; `context` collapsed double-dashes
- Names from one package wouldn't round-trip through the other
- Extracted shared `pkg/slug.Sanitize` — single behavior, both packages import it

## Warnings belong on stderr, not stdout
- `fmt.Printf` for warning messages in `indexContext` pollutes stdout
- stdout is for report output; stderr is for diagnostics
- Use `fmt.Fprintf(os.Stderr, ...)` for all warning/diagnostic messages
- Be consistent: CLI wiring uses stderr for ledger init warnings, so executor should too

## Raw result storage assumes JSON
- `reports.Save` hardcodes `.json` extension for raw results
- REST services overwhelmingly return JSON, but this is an assumption not a guarantee
- Documented with comment; if non-JSON sources are added, detect content type at save time

## Hand-rolled YAML front matter parsing is intentionally simple
- `context.parseEntry` uses line-by-line scanning instead of a YAML library
- This avoids a YAML dependency in the context package
- The parser will break on multi-line values or labels containing `: `
- Acceptable because Burrow controls all writes — just don't put colons in labels without quoting

## Goroutines in pipelines need panic recovery
- A panicking service adapter (nil pointer, type assertion) takes down the process
- Every user-facing goroutine needs `defer func() { if r := recover() ... }()`
- Surface panics as error results, not crashes — the pipeline should degrade gracefully

## Auth method validation should check credential presence
- `api_key` with empty key silently sends broken requests
- Validate at config load time, before any network requests
- `${ENV_VAR}` references are non-empty strings and correctly pass validation (resolved later)

## Timing tests need generous margins
- 250ms ceiling for 3×100ms parallel tasks fails on loaded CI machines
- 500ms still proves parallelism (sequential floor is 300ms) while being robust
- Prefer structural assertions (goroutine count, ordering) over wall-clock timing

## LoadAllRoutines should be fault-tolerant
- One bad YAML file shouldn't prevent loading all other routines
- Skip with warning, let the user fix the bad file independently
- Use optional `io.Writer` parameter to keep the API testable

## Empty config is intentionally valid
- Represents a fresh install before user configuration
- `gd init` produces an empty config, then the wizard adds to it
- Document with an explicit test rather than adding a validation error

## Never resolve env vars on config that will be saved
- `config.ResolveEnvVars()` expands `${SAM_API_KEY}` in memory — mutates the struct
- If you then call `config.Save()`, the actual secret value gets written to disk
- Always use `Config.DeepCopy()` to create a throwaway copy for env var resolution
- The saved config must always contain `${ENV_VAR}` references, never raw secrets

## Redact credentials before sending config to LLM
- Conversational config (`Session.ProcessMessage`) embeds current config YAML in the system prompt
- Auth keys, tokens, and API keys must be replaced with `${REDACTED}` before embedding
- User-agent values are not secrets — leave them visible for the LLM to understand the config
- Use `DeepCopy()` to avoid mutating the working config

## Interactive/ask commands must only use local LLMs
- `gd ask` spec says "zero network requests" — this is a privacy guarantee
- Interactive mode follows the same policy: only local providers, no remote
- This is the correct implementation of compartmentalization — combined context only stays local
- Document the intent with a comment, not just the behavior

## Bubble Tea: never block in Update
- Synchronous network calls (LLM, HTTP) in `Update` freeze the entire TUI
- Return a `tea.Cmd` (closure) that runs async and produces a `tea.Msg`
- Handle the result in `Update` via a custom message type
- Use a `busy` flag to show progress and block conflicting keybindings
- Pass a cancellable context (not `context.Background()`) so quit can abort in-flight work

## Byte slicing vs rune slicing
- `text[start:end]` with byte offsets can split multi-byte UTF-8 characters
- Reports may contain accented names, CJK text, emoji
- Use `[]rune` conversion when doing character-count-based truncation or substring extraction
- `strings.Index` returns byte offsets — convert to rune offset before rune-slicing

## Commands that filter by type should default to "all"
- A `gd context show` that only shows session entries surprises users
- Default to showing all types, with a `--type` filter flag for narrowing
- Apply the same principle to any list/search command with multiple categories

## Size-limit LLM inputs
- Two full reports concatenated can easily exceed a provider's context window
- Truncate and warn when combined input exceeds a threshold (e.g., 50KB)
- Same principle applies to `gd ask` context gathering — already has maxBytes param

## Conversational init must track applied vs proposed state
- LLM proposes configs; user may reject them
- Only return configs that the user explicitly accepted ("y" to apply)
- When user types "done" with no applied config, fall through to wizard (return nil, nil)

## Markers injected into markdown must avoid special characters
- CommonMark treats `__text__` as strong emphasis — `__BURROW_CHART_0__` becomes `**BURROW_****CHART_****0**`
- When injecting markers into markdown that will be rendered, use characters that are inert in CommonMark (hyphens, alphanumeric only)
- Fixed: changed from `__BURROW_CHART_N__` to `BURROW-CHART-N`
- Always test marker survival through the full rendering pipeline, not just string replacement

## Don't detect capabilities you can't use
- `DetectImageTier` detected Sixel terminals but `WriteInlineImage` returned nil for them
- Users with Sixel-capable terminals got silent text fallback with no explanation
- Either implement the capability or don't detect it — don't create false expectations
- When deferring implementation, add a comment explaining what's needed (e.g., "Sixel requires image.Paletted, not raw PNG")

## Don't duplicate utility functions across packages (revisited)
- `chart_process.go:loadChartPNG` and `export.go:loadChartPNGForExport` contained identical slug-based PNG loading logic
- Consolidated into `charts.LoadPNG(chartsDir, title, idx)` — single function, both callers import it
- This is the same pattern as the `slug.Sanitize` consolidation from Phase 2

## Protect shared mutable state in protocol clients
- `Client.sessionID` (MCP) was a plain string read/written in `call()` without synchronization
- `nextID` correctly used `atomic.Int64`, but `sessionID` was missed
- The pipeline executor runs sources in parallel — two sources using the same MCP service race on `sessionID`
- Fix: `sync.Mutex` around read and write of `sessionID` in `call()`
- Rule: any mutable state on a struct that may be called concurrently needs explicit protection

## Don't discard errors from functions that return paths
- `config.BurrowDir()` returns `("", error)` on failure
- Discarding the error with `_` silently produces a relative path ("cache" instead of "~/.burrow/cache")
- This violates the "All file I/O under ~/.burrow/" invariant
- Fix: accept the path as a parameter from callers that already have it, or propagate the error

## Inject time for deterministic pruning/expiry tests
- `PruneExpired(retention, now)` accepts `now time.Time` instead of calling `time.Now()` internally
- Allows testing exact boundary conditions (29 days = keep, 31 days = prune) without depending on wall clock
- Follows the same pattern as the scheduler's `Clock` interface
- Callers in production pass `time.Now()` — trivial

## Zero-value ambiguity in YAML config
- YAML unmarshals missing int fields to 0, so `raw_results: 0` (explicit disable) and missing `raw_results` (not configured) are indistinguishable
- When applying defaults, use the config struct directly when a config file exists; only apply defaults when NO config file exists at all
- Don't overlay defaults with `if value > 0` checks — this silently overrides explicit zeros

## Tests must not depend on system timezone
- `time.Local` varies by machine (e.g., AKST = UTC-9 on this machine)
- `time.Date(2025, 1, 15, 5, 1, 0, 0, time.UTC)` converted to AKST gives Jan 14, not Jan 15
- Any test comparing dates after timezone conversion must use explicit `time.Location`
- For test routines with Schedule fields: always set `Timezone: "UTC"` explicitly
- For isDue tests: pass `time.UTC` as the loc parameter directly
