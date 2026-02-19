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
