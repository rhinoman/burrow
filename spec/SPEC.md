# Burrow Specification

**Version:** 0.1 (Draft)

---

## 1. Overview

Burrow is a personal research assistant that performs daily intelligence tasks for the user, but never acts on their behalf. It queries external services on a schedule, enforces privacy boundaries between them, synthesizes results with a local or remote LLM, produces structured markdown reports with suggested actions, and provides an interactive mode for ad-hoc exploration. A single binary called `gd` provides all functionality.

## 2. Pipeline

### 2.1 Routines

A routine is a scheduled collection of source queries that produces a report. Routines are defined in `~/.burrow/routines/` as YAML files.

```yaml
# ~/.burrow/routines/morning-intel.yaml
schedule: "05:00"
timezone: "America/Anchorage"
jitter: 300                    # spread queries over 5 minutes randomly
llm: local/qwen-14b

report:
  title: "Market Intelligence Brief"
  style: executive_summary
  generate_charts: true
  max_length: 2000

synthesis:
  system: |
    You are a business development analyst writing a daily 
    brief. Prioritize by contract value and win probability. 
    Flag competitive threats. Be direct, no filler.

sources:
  - service: sam-gov
    tool: search_opportunities
    params: { naics: "541370", status: "active", posted_since: "yesterday" }
    context_label: "SAM.gov Postings"

  - service: edgar
    tool: company_filings
    params: { keywords: "geospatial" }
    context_label: "SEC Filings"
```

### 2.2 Routine Execution

When a routine executes:

- MUST query each source independently
- MUST use separate network connections per service
- MUST apply configured jitter to request timing
- MUST store raw results locally before synthesis
- MUST NOT share credentials or context between services during collection
- MUST generate a report even if some sources fail (noting failures)

### 2.3 Routine Management

```
gd routines list                   List configured routines
gd routines test <name>            Dry run — verify sources, check connectivity
gd routines run <name>             Execute immediately
gd routines history <name>         Show past executions
```

### 2.4 Manual Triggering

A routine MAY be triggered manually at any time. Manual execution follows the same rules as scheduled execution.

## 3. Services

### 3.1 Service Registry

Services are external data sources the client queries. Each service is configured with its own credentials, endpoint, and privacy settings. Services are stored in `~/.burrow/config.yaml`.

```yaml
services:
  - name: sam-gov
    type: rest
    endpoint: https://api.sam.gov
    auth:
      method: api_key
      key: ${SAM_API_KEY}

  - name: edgar
    type: rest
    endpoint: https://efts.sec.gov
    auth:
      method: user_agent
      value: "burrow/1.0 contact@example.com"

  - name: arxiv-mcp
    type: mcp
    endpoint: https://mcp.arxiv.example.com
    auth:
      method: bearer
      token: ${ARXIV_TOKEN}

  - name: local-crm
    type: mcp
    endpoint: http://localhost:8080
    auth:
      method: none
```

### 3.2 Service Specification Discovery

A service MAY declare a `spec` field pointing to machine-readable or human-readable API documentation. When present, the conversational configuration interface SHOULD fetch and interpret the spec to auto-generate tool mappings.

```yaml
services:
  - name: sam-gov
    type: rest
    endpoint: https://api.sam.gov
    auth:
      method: api_key
      key: ${SAM_API_KEY}
    spec: https://api.sam.gov/openapi.json
    tools:
      # auto-generated from spec, user can override
```

Supported spec formats:

| Format | Description |
|--------|-------------|
| OpenAPI/Swagger (JSON or YAML) | Structured, unambiguous — best case for auto-generation |
| API documentation page (HTML) | LLM interprets the docs and generates mappings — good enough |
| Any structured description | GraphQL schema, RAML, API Blueprint, etc. |

The `spec` field is optional. When absent, tool mappings must be defined manually or generated through conversational configuration without reference documentation.

When a spec is available, the conversational setup SHOULD:

1. Fetch the spec
2. Present available capabilities to the user
3. Generate tool mappings for the endpoints the user selects
4. Allow the user to review and modify the generated mappings

### 3.3 Service Types

The client MUST support the following service types:

| Type | Description |
|------|-------------|
| `mcp` | MCP-compatible endpoint with tool discovery and invocation |
| `rest` | Generic REST API with user-defined tool mappings |

The client SHOULD support additional service types as needed (RSS feeds, GraphQL, etc.) through a pluggable adapter interface.

### 3.4 Tool Mapping for REST Services

REST services that are not MCP-compatible require tool definitions that map to API calls:

```yaml
services:
  - name: sam-gov
    type: rest
    endpoint: https://api.sam.gov
    auth:
      method: api_key
      key: ${SAM_API_KEY}
    tools:
      - name: search_opportunities
        description: "Search active contract opportunities by NAICS code and keywords"
        method: GET
        path: /opportunities/v2/search
        params:
          - name: naics
            type: string
            maps_to: api.ncode
          - name: keywords
            type: string
            maps_to: api.title
          - name: posted_since
            type: string
            maps_to: api.postedFrom
```

### 3.5 Local Services

A local service runs on the user's own machine and is accessible via localhost. Local services:

- MAY use any supported service type
- Are exempt from privacy routing (no Tor, no proxy)
- Are exempt from timing jitter
- Typically have no authentication requirements
- Common uses: CRM exports, contact databases, local knowledge bases, file indexes

### 3.6 Credential Isolation

- Each service MUST have its own credentials
- Credentials MUST NOT be shared or leaked across services
- Credentials SHOULD be stored as environment variable references, not plaintext
- The client MUST NOT transmit credentials for one service to any other service

## 4. Synthesis

### 4.1 LLM Providers

The client MUST support multiple LLM backends for synthesis:

```yaml
llm:
  providers:
    - name: local/qwen-14b
      type: ollama
      endpoint: http://localhost:11434
      model: qwen2.5:14b
      privacy: local

    - name: local/mistral
      type: llamacpp
      model: ~/.burrow/models/mistral-7b.gguf
      privacy: local

    - name: remote/sonnet
      type: openrouter
      endpoint: https://openrouter.ai/api/v1
      api_key: ${OPENROUTER_KEY}
      model: anthropic/claude-sonnet
      privacy: remote

    - name: none
      type: passthrough
      privacy: local
```

### 4.2 Privacy Levels

| Level | Meaning | Behavior |
|-------|---------|----------|
| `local` | LLM runs on user's machine | No data leaves the machine during synthesis |
| `remote` | LLM runs on third-party infrastructure | Data is sent to the provider. Source attribution MAY be stripped (see 4.3) |

When a user first configures a remote LLM provider, the client MUST warn:

```
⚠ LLM provider 'remote/sonnet' sends synthesis data 
  to openrouter.ai. Collected results will leave your 
  machine during synthesis.

  For maximum privacy, use a local LLM provider.

  [Acknowledge] [Switch to local]
```

### 4.3 Source Attribution Stripping

When using a remote LLM, the client SHOULD strip source-identifying information before sending data for synthesis. This means the remote LLM receives the data but does not know which services it came from.

```
With attribution (local LLM):
  [Source: SAM.gov Postings]
  {contract data}

  [Source: EDGAR Filings]
  {filing data}

Without attribution (remote LLM):
  [Government Contract Data]
  {contract data}

  [Corporate Filing Data]
  {filing data}
```

This is configurable:

```yaml
privacy:
  strip_attribution_for_remote: true    # default
```

### 4.4 Synthesis Process

1. Collect all routine results from local storage
2. Assemble a synthesis context with source labels and data
3. Combine with the routine's system prompt
4. Send to configured LLM provider
5. Parse the LLM's output as markdown
6. Extract chart directives and generate chart images
7. Extract suggested actions
8. Save as a report file

### 4.5 Chart Generation

The LLM MAY request chart generation by emitting chart directives in its output:

````markdown
```chart
type: bar
title: "Contract Postings by Agency"
x: ["NGA", "NRO", "DIA", "CISA"]
y: [12, 4, 2, 1]
```
````

The client MUST render chart directives into images and embed them in the report. If chart rendering is unavailable, the client MUST fall back to displaying the data as a text table.

### 4.6 Passthrough Mode

When the LLM provider is set to `none` or `passthrough`, the client skips synthesis and produces a report containing raw results from each source, separated by source label. No interpretation, no suggested actions.

## 5. Reports

### 5.1 Format

Reports are CommonMark markdown files stored in `~/.burrow/reports/`. Each report is a directory containing the markdown file and any generated assets:

```
~/.burrow/reports/
  2026-02-19-morning-intel/
    report.md
    charts/
      contracts-by-agency.png
    data/
      sam-gov-raw.json
      edgar-raw.json
```

### 5.2 Report Structure

A report MUST include:

- Title and date
- Source summary (which services were queried, any failures)
- Synthesized content organized by sections defined in the routine
- Suggested actions with draft capability
- Source attribution for all claims

A report SHOULD include:

- Charts generated from collected data
- Links to raw source data for drill-down
- Comparison with previous report when `compare_with` is configured or when relevant trends exist

### 5.3 Report Comparison

A routine MAY declare `compare_with` to generate a delta report against another routine's latest report:

```yaml
# ~/.burrow/routines/afternoon-catchup.yaml
schedule: "13:00"
timezone: "America/Anchorage"

report:
  title: "Afternoon Update"
  compare_with: morning-intel    # highlight what changed since morning

sources:
  # same sources, narrowed timeframe
```

When `compare_with` is set, the synthesis prompt includes the referenced report's content and instructs the LLM to focus on changes, new items, and updates rather than repeating the full analysis.

Reports can also be compared ad-hoc:

```
gd reports compare 2026-02-18 2026-02-19
```

This generates an LLM-produced diff highlighting what changed between two reports.

### 5.4 Suggested Actions

Suggested actions appear in reports and interactive sessions. Each action has a type and a set of available operations:

```markdown
Suggested actions:
▸ Email Janet Liu at CISA about this opportunity [Draft]
▸ Notify contracts team about March 5 deadline [Draft]
▸ Review past proposal W911NF-24-R-0312 [Open]
▸ Start tracking DIA postings [Configure]
```

Action types:

| Type | Operations |
|------|-----------|
| `email` | Draft → Copy / Open in mail client |
| `social` | Draft → Copy / Open in browser |
| `internal` | Draft → Copy |
| `open` | Open report / file / URL in configured app |
| `configure` | Modify pipeline configuration |

### 5.5 Report Management

```
gd reports                         List recent reports
gd reports view [date] [routine]   View a report in the terminal viewer
gd reports search <query>          Full-text search across all reports
gd reports compare <date1> <date2> LLM-generated diff between two reports
gd reports export <date> <format>  Export as PDF, HTML, or plain markdown
```

### 5.6 Report Accumulation

Reports accumulate as a personal intelligence archive. The context ledger (Section 8) indexes report content for search and longitudinal analysis. The `gd ask` command queries this archive.

## 6. Interactive Mode

### 6.1 Overview

Interactive mode provides ad-hoc access to services and the local context. It is started with `gd` (no arguments) or by following a suggested action from a report.

```
$ gd

  Burrow v0.1 · 5 sources configured

>
```

### 6.2 Service Queries

The user can query any configured service directly:

```
> search sam-gov "cybersecurity NAICS 541512"
> query edgar "Leidos 10-K 2025"
> ask arxiv-mcp "recent papers on synthetic aperture radar"
```

### 6.3 Context Queries

The user can query their local context — all past reports, cached results, and interactive session data — using the `ask` command:

```
> ask "Which agencies increased geospatial spending this quarter?"

  🧠 Based on your collected data (47 reports, Oct-Feb):
  ...
```

Context queries go to the configured LLM. They do NOT make any network requests. The LLM reasons over data already on the user's machine.

### 6.4 Drafting

The user can draft communications at any time:

```
> draft email to Janet Liu about the CISA contract opportunity

  ──────────────────────────────────────────────
  To: janet.liu@cisa.dhs.gov
  Subject: ...
  ...
  ──────────────────────────────────────────────

  [Copy] [Open in mail] [Edit] [Discard]
```

Draft generation uses the LLM with relevant context from the current session and the context ledger.

### 6.5 Session Context

All interactive queries and results are logged to the context ledger. Morning reports can reference interactive activity. Interactive sessions can reference report content. Everything feeds the same local context.

## 7. Privacy

### 7.1 The Core Guarantee: Compartmentalization

Burrow does not guarantee anonymity. Services receive API keys, IP addresses, and query content. Each service knows something about you.

What Burrow guarantees is **compartmentalization**: no single service or provider sees your complete picture. The combined picture exists only on your machine.

SAM.gov knows you searched for NAICS 541370. EDGAR knows you pulled filings on a competitor. A news service knows you track a certain industry. A job board knows you monitor certain roles. None of them know about the others. None of them can reconstruct your strategy. That reconstruction — "this person is positioning to bid on a specific contract, their main competitor is X, and they have a contact at the agency" — exists only in your local synthesis.

This is the architectural difference between Burrow and a centralized agent platform. A centralized platform sends every query through one provider. That provider sees every source, every question, every synthesis. They could reconstruct your entire strategy from their logs. With Burrow, you would need to compromise every individual service *and* your local machine to assemble the same picture.

Compartmentalization is enforced at every layer:

**Credential isolation.** Each service MUST have its own credentials. Credentials for one service MUST NEVER be transmitted to another service.

**No cross-service context.** During pipeline collection, services are queried independently. No query to one service contains information obtained from another service. No service learns what other services you use.

**Synthesis isolation.** The synthesis step — where all data is combined — happens only on your local machine (or with a remote LLM that receives stripped attribution; see Section 4.3). No service participates in synthesis. No service sees the combined output.

**Context stays local.** The context ledger — your accumulated reports, results, and interactive history — MUST NEVER be transmitted to any service. The `gd ask` command makes zero network requests.

### 7.2 The Read-Only Boundary

The second architectural guarantee. Burrow reads from the network and writes to the local disk. That's it.

The client MUST NOT have the capability to:

- Send emails
- Post to social media
- Modify external services
- Authenticate to services for write operations
- Execute actions on behalf of the user

The client produces drafts and hands off to system applications. The boundary is always the clipboard or a URI. This is a security property: a compromised Burrow client can show you wrong information. It cannot send emails on your behalf, post to your social media, or act as you. The worst case is bounded by design.

### 7.3 Defense in Depth

The following measures harden the edges. They are valuable but secondary. Compartmentalization and the read-only boundary do the heavy lifting.

**Network isolation.** Each service connection SHOULD be independently routable:

```yaml
privacy:
  default_proxy: none

  routes:
    - service: sam-gov
      proxy: tor
    - service: edgar
      proxy: tor
    - service: local-crm
      proxy: direct
```

**Request minimization.** Send only what's required.

```yaml
privacy:
  minimize_requests: true       # send only required fields
  strip_referrers: true         # never send origin/referrer headers
  randomize_user_agent: true    # rotate generic user agents
```

When `minimize_requests` is enabled, the client MUST send only parameters explicitly provided by the user or routine configuration. The client MUST NOT add optional parameters, tracking headers, or metadata beyond what is required for the request to succeed.

**Timing decorrelation.** Scheduled routines MUST support a `jitter` parameter that spreads queries randomly over a time window. This prevents services from correlating simultaneous requests to the same user.

**Result caching.** The client SHOULD cache results with a configurable TTL. Fewer requests means fewer fingerprinting opportunities.

```yaml
services:
  - name: sam-gov
    cache_ttl: 3600          # results valid for 1 hour
```

**Source attribution stripping.** When using a remote LLM for synthesis, the client SHOULD strip service names and endpoint URLs so the LLM provider cannot reconstruct your source topology (see Section 4.3).

### 7.4 Threat Model

| Threat | Layer | Mitigation |
|--------|-------|-----------|
| Service sees queries from other services | Compartmentalization | Credential isolation, no cross-service context |
| Provider reconstructs your strategy | Compartmentalization | No single provider sees combined picture |
| Remote LLM sees source topology | Compartmentalization | Source attribution stripping |
| Compromised client acts on your behalf | Read-only boundary | No outbound action capability by design |
| Network observer correlates requests | Defense in depth | Per-service proxy/Tor routing, timing jitter |
| Service fingerprints requests | Defense in depth | Request minimization, user-agent rotation |
| Timing correlation across services | Defense in depth | Configurable jitter on scheduled queries |
| Stale requests reveal patterns | Defense in depth | Result caching with configurable TTL |

## 8. Context Ledger

### 8.1 Purpose

The context ledger is a local, searchable record of everything Burrow has collected and produced. It enables longitudinal analysis — questions like "what changed since last month" and "what patterns emerge across 30 days of reports."

### 8.2 Storage

- MUST be stored in `~/.burrow/context/`
- MUST be plain text or YAML, inspectable with standard tools
- MUST NEVER be transmitted to any service
- MUST be searchable by the client
- MUST be deletable by the user in whole or in part

### 8.3 Contents

The context ledger indexes:

- All routine results (raw source data)
- All generated reports
- All interactive session queries and results
- Contact data imported by the user
- User-provided notes and annotations

### 8.4 Context Queries

```
gd ask "..."                 Query context with LLM reasoning
gd context search <query>    Full-text search without LLM
gd context show              Show current session context
gd context clear             Clear all context
gd context stats             Show context size, date range, source breakdown
```

### 8.5 Retention

```yaml
context:
  retention:
    reports: forever          # keep all reports
    raw_results: 90           # days to keep raw source data
    sessions: 30              # days to keep interactive session logs
```

## 9. Configuration

### 9.1 Conversational Setup

The primary configuration interface is natural language. The user describes what they want and the LLM generates the appropriate YAML.

```
$ gd init          # first-time setup conversation
$ gd configure     # modify existing configuration
```

The client MUST support conversational configuration for:

- Adding and removing services
- Creating and modifying routines
- Importing contacts
- Setting privacy preferences
- Configuring LLM providers
- Configuring system application preferences

### 9.2 YAML Configuration

All configuration is stored as YAML files under `~/.burrow/`. The conversational interface reads and writes these files. Users MAY edit them directly.

```
~/.burrow/
  config.yaml              # main configuration (services, privacy, apps, LLM)
  routines/                # routine definitions
  contacts/                # imported contact data
  reports/                 # generated reports
  context/                 # context ledger
  cache/                   # cached service results
  models/                  # local LLM model files (optional)
```

### 9.3 System Applications

```yaml
apps:
  email: default            # xdg-open / open (macOS)
  browser: default
  editor: default
  media: default
```

Override with any application name. The client uses the configured application for all handoff operations (opening drafts, playing media, viewing URLs).

### 9.4 Configuration Validation

The client MUST validate configuration on startup and report errors clearly. Invalid configuration MUST NOT cause silent failures.

## 10. Rendering

### 10.1 Terminal Rendering

The report viewer and interactive mode MUST render CommonMark markdown in the terminal.

- MUST render headings, paragraphs, lists, links, code blocks, emphasis, block quotes
- MUST support scrollable content
- MUST support expandable/collapsible sections
- MUST support navigation between sections and linked content

### 10.2 Image Rendering

The client MUST detect terminal capabilities on startup and render images accordingly:

**Tier 1** (Kitty, iTerm2, WezTerm, Ghostty, foot, Contour): Render images inline using the appropriate graphics protocol.

**Tier 2** (gnome-terminal, alacritty, xterm, Terminal.app, and others): Display alt text or image description. Offer to open in external viewer via keybinding.

Detection MUST be automatic. Default rendering mode is `auto`.

```yaml
rendering:
  images: auto              # auto | inline | external | text
```

### 10.3 Audio and Video

The client SHOULD support audio and video playback by handing off to the configured media application.

When the client encounters audio or video, it MUST display a description and provide a way to play it (e.g., keybinding to open in external player, save locally). The client MUST NOT silently discard media content.

### 10.4 Charts

Charts generated during synthesis (see Section 4.5) are rendered as inline images when terminal supports it, or as text tables in Tier 2 terminals.

### 10.5 Interactive Elements

Reports may contain interactive elements:

- `[Draft]` — trigger draft generation for a suggested action
- `[Open]` — open a file, report, or URL in configured application
- `[Configure]` — modify pipeline configuration
- Expandable sections — toggle detail visibility

These are keybinding-driven in the terminal viewer, not clickable UI elements.

## 11. Command Line Interface

```
gd                             Launch interactive mode
gd init                        First-time setup conversation
gd configure                   Modify configuration conversationally

gd morning                     View today's morning report (shortcut)
gd <routine-name>              View latest report for a routine

gd routines list               List configured routines
gd routines test <name>        Dry run a routine
gd routines run <name>         Execute a routine now
gd routines history <name>     Show past executions

gd reports                     List recent reports
gd reports view [date]         View a report
gd reports search <query>      Search across reports
gd reports compare <d1> <d2>   Compare two reports
gd reports export <date> <fmt> Export report

gd ask "..."                   Query local context with LLM
gd context search <query>      Full-text search context
gd context clear               Clear context
gd context stats               Context statistics

gd help                        Show help
gd version                     Show version
```

## 12. What Burrow MUST NOT Do

- MUST NOT send emails, post to social media, or perform any outbound action on behalf of the user
- MUST NOT share credentials between services
- MUST NOT share query context between services
- MUST NOT transmit the context ledger to any service
- MUST NOT phone home, send telemetry, or transmit usage data to any party
- MUST NOT store data in formats the user cannot inspect with standard tools
- MUST NOT require an account, registration, or sign-up with any central service
- MUST NOT execute code received from any service
- MUST NOT make network requests during context queries (gd ask)
