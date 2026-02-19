# Burrow Complexity Budget

This document defines the permanent boundaries of Burrow. It is not a roadmap of features to be added later. It is a list of things Burrow will never do.

The agent platforms are in an arms race to do more on the user's behalf. Every feature is another integration, another credential, another attack surface, another entity that sees your data. Burrow goes the other direction. It does the research. You do the acting.

## What Burrow Does

This is the complete list.

- **Collects** data from configured services on a schedule
- **Isolates** services from each other — separate credentials, separate connections, no shared context
- **Synthesizes** collected data into structured reports using a local or remote LLM
- **Renders** beautiful markdown reports in the terminal with inline images and charts
- **Suggests** actions with draft generation for emails, posts, and communications
- **Hands off** drafts to system applications via clipboard or URI — never sends them
- **Queries** local context interactively for longitudinal analysis
- **Stores** everything as plain text files the user can inspect, edit, and delete

## What Burrow Will Never Do

Each of the following has been explicitly considered and permanently rejected.

### Act on Behalf of the User

Burrow will never send an email, post to social media, modify a file on a remote service, make a purchase, or perform any outbound action. It produces drafts and opens system applications. The human presses send. This is the foundational constraint. Every agent platform that crosses this line becomes a liability. Burrow will not cross it.

### Hold Credentials for Write Access

Burrow holds API keys for reading from services. It will never hold OAuth tokens, session cookies, or credentials that grant write access to any external system. If it can't write, a compromise can't write. The attack surface is bounded by design.

### Become a Platform

Burrow will never have an account system, a cloud sync feature, a hosted version, a marketplace, a plugin store, or a social component. It is a local tool that reads from the network and writes to your disk. There is nothing between you and your data.

### Phone Home

Burrow will never send telemetry, analytics, crash reports, usage statistics, or update checks to any central service. The binary runs on your machine and talks to the services you configured. It talks to no one else.

### Run a Background Service with Network Access

The pipeline scheduler runs as a lightweight daemon or cron job. It makes outbound requests to configured services on schedule. It will never listen on a port, accept inbound connections, or expose any network surface. Burrow is a client. It will never be a server.

### Bundle or Recommend a Default LLM Provider

Burrow will never ship with a default remote LLM configuration, bundle API keys for a cloud provider, or steer users toward any specific LLM service. The user chooses their model. Burrow provides the interface. If the user configures nothing, synthesis is unavailable and reports contain raw results.

### Store Data in Opaque Formats

Everything is YAML, markdown, JSON, or plain text. No SQLite, no binary blobs, no proprietary formats. If you can't `cat` it, it doesn't belong in `~/.burrow/`. This constraint survives even if it means worse performance. Inspectability is more important than efficiency.

### Operate Without User-Configured Sources

Burrow will never scrape, discover, or connect to services the user has not explicitly configured. No default feeds, no suggested sources that auto-connect, no background discovery. Every outbound connection is one the user chose.

### Cache Credentials in Memory Beyond a Session

When a session or routine completes, credentials are released. No credential caching across sessions. No keychain integration that persists tokens the user didn't ask to persist. Environment variables are read on demand.

## How to Evaluate Proposed Additions

1. **Is it on the never list?** If yes, the discussion is over.

2. **Does it break compartmentalization?** If it sends data from one service to another, leaks cross-service context, or lets any single external entity see the combined picture — reject it. Compartmentalization is the core architectural guarantee.

3. **Does it give Burrow the ability to act externally?** If yes, reject it. The read-only boundary is inviolable.

4. **Does it require trusting a central service?** If yes, reject it. The user trusts their configured services and their local machine. Nothing else.

5. **Does it store data the user can't inspect?** If yes, find a way to make it inspectable or reject it.

6. **Does it connect to something the user didn't configure?** If yes, reject it.

7. **Could it be done by the LLM during synthesis instead of as a client feature?** If yes, it doesn't belong in the client. Teach the LLM to do it through the system prompt.

## The Principle

Agent platforms compete on capability. They want to do more, access more, automate more. Each new capability is a new vector for compromise and a new entity that sees your data. A centralized agent that sends every query through one provider can reconstruct your entire strategy from their logs.

Burrow competes on restraint. Each service sees one question. No service sees the answers to the others. No service sees the synthesis. The combined picture — the strategy, the pattern, the advantage — exists only on your machine.

The less Burrow can do, the less damage a compromise can cause, and the more the user can trust it. The absence of capability is itself a capability. It is the capability to be trusted.

This document is the guarantee.
