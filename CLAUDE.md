# CLAUDE.md

You are working on Burrow, a personal research assistant that performs daily intelligence tasks for the user, but never acts on their behalf. It queries services on a schedule, synthesizes results with a local LLM, produces beautiful actionable reports, suggests next steps, and drafts communications. The human reviews, edits, and acts. The system never sends, posts, or publishes anything.

The primary audience is solo founders, independent consultants, small firm operators, and researchers who need competitive intelligence but can't afford a research team — and don't trust any single provider with the full picture.

Read the manifesto (`MANIFESTO.md`) before your first session. It is not marketing. It is the design document.

## What This Is

Burrow is four things:

1. **Pipeline** — scheduled queries to external services with privacy boundaries between them
2. **Synthesis engine** — local or remote LLM produces structured markdown reports with suggested actions
3. **Report viewer** — terminal renderer with inline images, charts, expandable sections, media handoff
4. **Interactive mode** — ad-hoc queries, context search, draft generation

The reference implementation is a single Go binary called `gd`.

## Architecture

```
cmd/gd/              Single binary entry point, subcommand router
pkg/
  pipeline/           Routine scheduling, jitter, execution
  services/           Service registry, auth, credential isolation
  mcp/                MCP client for MCP-compatible services
  http/               REST client for generic API services
  privacy/            Proxy routing, request minimization, timing, user-agent rotation
  synthesis/          LLM integration, report generation, chart directives
  charts/             Chart rendering from data
  reports/            Report storage, indexing, search, comparison, export
  context/            Longitudinal context ledger, full-text search
  contacts/           Contact data import and lookup
  actions/            Suggested actions, draft generation, system app handoff
  render/             Terminal markdown renderer, inline images, media
  config/             YAML config management
  configure/          Conversational configuration (gd init, gd configure)
```

## Key Documents

- `MANIFESTO.md` — why Burrow exists
- `spec/SPEC.md` — what a conforming implementation must do
- `spec/COMPLEXITY-BUDGET.md` — what Burrow will never do (read this twice)

## The Complexity Budget

The most important document in the repository.

Burrow will **never**:

- Send emails, post to social media, or perform any outbound action
- Hold credentials for write access to external systems
- Have accounts, cloud sync, a hosted version, or a marketplace
- Phone home, send telemetry, or check for updates
- Listen on a port or accept inbound connections
- Bundle or recommend a default LLM provider
- Store data in opaque formats (no SQLite, no binary blobs)
- Connect to services the user hasn't configured
- Cache credentials across sessions

If you are about to write code that does any of these, stop. Read `spec/COMPLEXITY-BUDGET.md`.

## The Read-Only Boundary

Burrow reads from the network and writes to the local disk. That's it.

It produces drafts and hands them off via clipboard or mailto: URI. It never authenticates for write access. It never sends anything on the user's behalf. A compromised Burrow cannot act as the user because it was never given the ability.

Every PR should be evaluated against this boundary. If it moves data outward to an external service in a way the user didn't explicitly configure as a read operation, it violates the boundary.

## Privacy Architecture

Burrow's privacy model has a clear hierarchy. Understand it before writing any code that touches services or data.

**1. Compartmentalization (the architecture)**

This is the defining privacy property. No single service or provider sees the user's complete picture. The combined picture exists only on the user's machine.

- Each service has its own credentials. Never leak one service's credentials to another.
- During pipeline collection, services are queried independently. No query to one service may contain information obtained from another service.
- No service participates in synthesis. The step where all data comes together happens locally.
- The context ledger never leaves the machine. `gd ask` makes zero network requests.

If you're writing code that sends data from one service to another, or includes cross-service context in an outbound request, you are breaking compartmentalization. Stop.

**2. Read-only boundary (the constraint)**

Covered above. No write access to external services. No outbound actions. Drafts hand off to system apps.

**3. Defense in depth (the hardening)**

These are valuable but secondary. They harden the edges. Compartmentalization does the heavy lifting.

- Timing jitter: spread scheduled queries randomly so services can't correlate simultaneous requests.
- Request minimization: send only required parameters. Strip referrers. Rotate user agents.
- Source attribution stripping: when synthesizing with a remote LLM, strip service names and endpoint URLs.
- Network isolation: per-service proxy/Tor routing.

## Conversational Configuration

Users configure Burrow through natural language (`gd init`, `gd configure`). The LLM interprets intent and writes YAML. Users never have to hand-write config files.

When adding a REST service, check for a `spec` field or ask the user if the API has published documentation (OpenAPI, Swagger, docs page). If available, fetch it and use it to auto-generate tool mappings. The LLM reads the spec, presents available capabilities, and maps the endpoints the user wants. This eliminates hand-written REST tool configs for well-documented APIs.

The YAML is always the source of truth. The conversation reads and writes the same files a power user would edit with vim. Both paths must produce identical results.

When implementing conversational configuration:

- Parse user intent, map to config changes
- Show the user what will change before applying
- Validate the resulting config
- Never generate config the user can't understand by reading the file

## Report Generation

Reports are the primary product. Not chat responses. Documents.

The synthesis LLM receives collected data and a system prompt and produces structured markdown. The system prompt defines the user's priorities and report style. Reports include suggested actions with `[Draft]`, `[Open]`, and `[Configure]` affordances.

Charts: the LLM emits chart directives in fenced code blocks. The client renders them as images (inline for Tier 1 terminals) or text tables (Tier 2). Use a Go charting library — no external dependencies for chart generation.

Reports are files on disk. Always markdown. Always inspectable. The viewer makes them beautiful. But `cat` always works.

## Image Rendering

The reference client MUST support Tier 1 (inline images) with graceful fallback to Tier 2 (text only).

```
Tier 1: Kitty, iTerm2, WezTerm, Ghostty, foot, Contour
  → Inline images via Kitty graphics protocol or Sixel

Tier 2: gnome-terminal, alacritty, xterm, Terminal.app, and others
  → Alt text / description display, external viewer on demand
```

Detect terminal capabilities on startup. No configuration required. User can override with `rendering.images` in config (`auto | inline | external | text`). Default is `auto`.

The reference client hands off audio and video to an external player (`xdg-open`, `mpv`, or user-configured). Other implementations may render media inline if capable. The spec requires graceful fallback — if a client can't play media, it must show a description and a way to access the file. Never silently discard content.

## System App Handoff

Burrow produces content. System apps consume it. The boundary is always the clipboard or a URI.

```
[Copy]             → clipboard
[Open in mail]     → mailto: URI with pre-filled fields → system email app
[Open in browser]  → URL → system browser
[Open]             → file path → configured editor or viewer
[Play]             → file path → configured media player
```

Default is `xdg-open` (Linux) or `open` (macOS). User overrides in `config.yaml` under `apps:`.

## Go Conventions

- Single binary: `go build ./cmd/gd`
- Interfaces for swappable backends (LLM providers, service adapters)
- Minimal dependencies. Standard library where possible.
- No frameworks. Charm's Bubble Tea + Lipgloss for TUI rendering.
- Errors are values. Wrap with context, don't panic.
- Test with `go test ./...`. Tests should not require network access.
- All file I/O under `~/.burrow/`. Never write outside this directory without explicit user action.

## When In Doubt

1. Does it leak information between services? → Don't build it. Compartmentalization is the core guarantee.
2. Does it act on behalf of the user? → Don't build it.
3. Does it send data somewhere the user didn't configure? → Don't build it.
4. Can the LLM handle it during synthesis? → Make it a system prompt concern, not a client feature.
5. Does it store something the user can't `cat`? → Find a text format.
6. Would it still work if the user `rm -rf ~/.burrow/` and started over? → It should.

The pipeline should be simple enough to explain in a paragraph. Keep it that way.

## Development practices

### 1.  Plan Mode Default
- Enter plan mode for ANY non-trivial task (#+ steps or architectural decisions)
- If something goes wrong, STOP and re-plan immediately - don't keep pressing on.
- Use plan mode for verification steps, not just building
- Write detailed specs upfront to reduce ambiguity

### 2. Subagent Strategy
- Use subagents liberally to keep main context window clean
- Offload research, exploration, and parallel analysis to subagents
- For complex problems, throw more compute at it via subagents
- One task per subagent for focused execution

### 3. Self improvement Loop 
- After ANY correction from the user: update `tasks/lessons.md` with the Pattern
- Write rules for yourself that prevent the same mistake 
- Ruthlessly iterate on these lessons until mistake rate drops 
- Review lessons at session start for relevant project

### 4. Verification before Done 
- Never mark a task complete without proving it works 
- Diff behavior between main and your changes when relevant 
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness 

### 5. Demand Elegance (Balanced)
- For non-trivial changes: pause and ask "is there a more elegant way"
- If a fix feels hacky: "Knowing everything I know now, implement the elegant solution."
- Skip this for simple, obvious fixes -- don't over-engineer
- Challenge your own work before presenting it.

### 6. Autonomous Bug fixing
- When given a bug report: Just fix it.  Don't ask for hand-holding
- Point at logs, errors, failing tests - then resolve them 
- Zero context switching required from the user 
- Go fix failing CI tests without being told how 

## Task management 

1. **Plan First**: Write plan to 'tasks/todo.md' with checkable items 
2. **Verify Plan**: Check in before starting implementation 
3. **Track Progress**: Mark items complete as you go 
4. **Explain Changes**: High level summary at each step 
5. **Document Results**: Add review section to `tasks/todo.md`
6. **Capture Lessons**: Update `tasks/lessons.md` after corrections

## Core Principles
- **simplicity first**: Make every change as simple as possible.  Impact minimal code.
- **No Laziness**: Find root causes. No temporary fixes.  Senior developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Avoid introducing bugs. 

