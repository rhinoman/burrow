# Burrow

A personal research assistant that performs daily intelligence tasks for you, but never acts on your behalf.

Queries services overnight. Synthesizes locally. Produces a report by morning. Suggests what to do. Drafts the emails. You press send.

## The Problem

You're building something alone. You need to track your market, watch competitors, find customers, monitor industry changes. A funded startup would hire analysts for this. You have forty browser tabs and a nagging feeling you're missing things.

The AI agent platforms offer to help — if you hand over your email, your CRM, your social media credentials. One agent, total access, total convenience. Also total visibility into your business by a single provider.

You don't need an agent that acts as you. You need an analyst that works for you.

## What It Looks Like

```
$ gd morning

╭──────────────────────────────────────────────────╮
│  Daily Brief                                     │
│  February 19, 2026 · 5 sources                    │
╰──────────────────────────────────────────────────╯

## New in Your Space

Two companies filed Delaware incorporations in 
construction management SaaS last month. One has 
YC funding (BuildFlow, W26 batch).

  ┌────────────────────────────────────────────────┐
  │  [chart: competitor funding rounds, 12 months] │
  └────────────────────────────────────────────────┘

## Potential Customers

A project manager at Turner Construction posted 
on LinkedIn about scheduling pain points your 
product solves. A similar post from a PM at Skanska.

  Suggested actions:
  ▸ Reach out to Sarah Chen at Turner [Draft]
  ▸ Reach out to Mike Okafor at Skanska [Draft]

## Industry Changes

The IBC 2027 building code update passed committee 
yesterday. Your compliance module may need updates.

  Suggested actions:
  ▸ Email your contact at ICC about the timeline [Draft]
  ▸ Create a task to review compliance impact [Draft]
```

Every `[Draft]` generates text and hands it to your email client or clipboard. Burrow never sends anything. You review, edit, send.

## Quickstart

See a working pipeline in 60 seconds. No API keys needed.

```
$ go build ./cmd/gd
$ ./gd quickstart

  Created ~/.burrow/config.yaml
  Created ~/.burrow/routines/weather.yaml

  Testing weather.gov connectivity...
    OK    weather-gov/forecast  (230ms)
    OK    weather-gov/alerts    (180ms)

  Generating report...
  Report saved: ~/.burrow/reports/2026-02-20T063012-weather/

  View the report:
    gd weather
    gd reports view weather

  Customize the location:
    Edit ~/.burrow/config.yaml to change the NWS grid point
    Find your grid point: https://api.weather.gov/points/{lat},{lon}

  Ready for real services?
    gd init
```

Uses the free [NWS weather API](https://www.weather.gov/documentation/services-web-api) — no signup, no credentials. The quickstart creates a real config, a real routine, tests connectivity, and generates a real report you can view immediately.

Ready to configure your own services? Run `gd init`.

## Setup

```
$ brew install burrow    # or go install, or build from source
$ gd init

  Tell me about yourself and what you want to track.

> I'm building a SaaS product for construction project 
  management. Track competitors, find potential customers, 
  stay current on building codes and industry news.

  ✓ Created routine: daily-brief (runs 5:00 AM)
  ✓ Configured 5 sources
  ✓ Ready. Run `gd routines test daily-brief` to try it.
```

Five minutes of conversation. No YAML. No documentation. The config files are generated for you, inspectable and editable if you want.

## How It Works

**1. Collect.** The pipeline queries your configured services on a schedule. Each service gets separate credentials, separate network connections. No service sees your other services. Timing is jittered so services can't correlate requests.

**2. Synthesize.** A local LLM reads all the collected data and produces a structured report. It cross-references sources, identifies patterns, generates charts, and suggests actions. Remote LLM providers are supported with automatic source attribution stripping.

**3. Report.** You wake up to a beautiful markdown document rendered in your terminal with inline images, charts, and expandable sections. Reports accumulate as a searchable archive.

**4. Act.** Pull threads interactively. Draft emails. Query your past reports. Every action is a suggestion you choose to follow. The system never acts for you.

## Interactive Mode

```
$ gd

> search linkedin-osint "construction management software fundraise"
  2 results: BuildFlow (YC W26, $4M seed), SitePlan ($2M pre-seed)...

> ask "How does BuildFlow's feature set compare to ours based 
  on what we know?"
  🧠 Based on your collected data...

> draft email introducing our product to the Turner PM
  [email draft generated]
  [Copy] [Open in mail] [Edit] [Discard]
```

## Query Your History

```
$ gd ask "What's changed in my competitive landscape this quarter?"

  🧠 Based on your collected data (62 reports, Dec-Feb):

  Two new entrants: BuildFlow (funded) and SitePlan 
  (pre-seed). PlanGrid has gone quiet — no new hires, 
  no press in 6 weeks. Your product was mentioned 
  positively in the ENR technology roundup on Feb 3...
```

No network requests. The LLM reasons over data already on your machine.

## Privacy Model

Burrow cannot guarantee anonymity — you're using HTTPS services with API keys. Each service knows something about you.

What it guarantees is **compartmentalization**. SAM.gov knows you searched for contracts. A news API knows you track construction tech. LinkedIn knows you're watching competitor hiring. None of them know about the others. The only place all of this comes together is your local machine.

A centralized agent platform sends everything through one provider — they can reconstruct your entire strategy from their logs. With Burrow, you'd need to compromise every individual service *and* your machine to get the same picture.

| Layer | What it does |
|-------|-------------|
| **Compartmentalization** | No service sees your other services. No service sees your synthesis. Only your machine holds the full picture. |
| **Read-only boundary** | Can't act on your behalf — no email, no posting, no write access. Worst case is wrong information, not unauthorized action. |
| **Defense in depth** | Jittered timing, request minimization, user-agent rotation, optional Tor routing, source attribution stripping for remote LLMs. |

A compromised Burrow can show you wrong information. It cannot send emails, post to social media, or act as you. The worst case is bounded.

## vs. AI Agent Platforms

```
Agent platforms:   See everything → Decide → Act on your behalf
Burrow:            See what you configure → Analyze → Suggest → Draft
You:               Review → Edit → Act (or don't)
```

A funded competitor has a three-person BD team doing research manually. An agent platform does it by seeing your entire digital life. Burrow does it overnight on your laptop, and nobody sees the full picture but you.

## Install

```
$ go install github.com/yourorg/burrow/cmd/gd@latest
```

Or build from source:

```
$ git clone https://github.com/yourorg/burrow.git
$ cd burrow
$ go build ./cmd/gd
```

### Requirements

- Go 1.22+
- A local LLM (Ollama recommended) or remote LLM API key (OpenRouter, etc.)

### Recommended Models

```
Synthesis:    Qwen 2.5 14B (sweet spot for daily reports)
Minimum:      Llama 3.1 8B (basic summarization)
Hardware:     16GB RAM or 8GB+ VRAM
```

## Project Structure

```
burrow/
  MANIFESTO.md                 Why this exists
  spec/
    SPEC.md                    What a conforming implementation must do
    COMPLEXITY-BUDGET.md       What Burrow will never do
  cmd/
    gd/                        Single binary entry point
  pkg/
    pipeline/                  Routine scheduling and execution
    services/                  Service registry and credential isolation
    mcp/                       MCP client
    http/                      REST client for generic APIs
    privacy/                   Network isolation, request minimization
    synthesis/                 LLM integration, report generation
    charts/                    Chart rendering
    reports/                   Report storage, search, comparison
    context/                   Longitudinal context ledger
    contacts/                  Contact data management
    actions/                   Suggested actions and draft generation
    render/                    Terminal rendering, inline images
    config/                    YAML configuration management
    configure/                 Conversational setup (gd init, gd configure)
```

## Documentation

| Document | Description |
|----------|-------------|
| [Manifesto](MANIFESTO.md) | Why Burrow exists |
| [Specification](spec/SPEC.md) | What a conforming implementation must do |
| [Complexity Budget](spec/COMPLEXITY-BUDGET.md) | What Burrow will never do |

## What Burrow Will Never Do

Send emails. Post to social media. Act on your behalf. Phone home. Store data in formats you can't read. Connect to services you didn't configure.

These are permanent constraints. Read the [Complexity Budget](spec/COMPLEXITY-BUDGET.md) for why.

## Contributing

Read `CLAUDE.md` for project conventions. Read `spec/COMPLEXITY-BUDGET.md` before proposing features.

The read-only boundary is inviolable. Burrow reads from the network and writes to your disk. That's it.

## License

MIT
