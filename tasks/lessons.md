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
