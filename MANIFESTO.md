# The Burrow Manifesto

## The Problem

You're building something alone. You're the founder, the salesperson, the analyst, and the engineer. You need to track your market, watch competitors, find customers, monitor regulations, and stay current in your field. A funded startup would hire analysts for this. You have a browser with forty tabs and a growing sense that you're missing things.

The AI agent platforms offer to help. Give them your email, your calendar, your social media, your CRM credentials. Let them act on your behalf — send outreach, post updates, schedule meetings. One agent, total access, total convenience. Also total visibility into your business by a single provider. Your customer list, your pricing strategy, your competitive research, your pipeline — all sitting on someone else's infrastructure.

You don't need an agent that acts as you. You need an analyst that works for you.

## The Idea

Burrow is a personal research assistant that performs daily intelligence tasks for you, but never acts on your behalf.

It queries services on a schedule — contract databases, SEC filings, news feeds, academic papers, job postings, whatever you configure. It enforces privacy boundaries between them so no single service sees your complete picture. It synthesizes results with a local LLM and produces structured reports with suggested actions. Then it drafts the emails, composes the posts, identifies the contacts. You review, edit, and act. The system never sends, posts, or publishes anything.

No single service knows your strategy. The only entity that sees the full picture is your own machine.

## The Solo Bootstrapper

You're building a SaaS product for construction project management. Here's Tuesday morning:

You wake up. Burrow ran overnight. The brief is waiting.

Three new companies in your space filed Delaware incorporations last month — one has YC funding. A construction industry newsletter mentioned your product category in a trends piece. Two potential customers posted on LinkedIn about pain points your product solves. The building code update you've been tracking passed committee. A customer you onboarded last month posted a complaint on a forum about a competitor's product.

Suggested actions: reach out to the LinkedIn posters, draft a comment on the forum thread, email your contact at the trade association about the building code change, update your competitive landscape doc.

You didn't research any of this. You didn't open forty tabs. You read one document, spent fifteen minutes drafting follow-ups, and got back to building your product.

A funded competitor has a three-person BD team doing this manually. You have Burrow running on your laptop overnight.

## Principles

**No service sees the whole picture.** Burrow queries many sources. Each one sees only the question asked of it. A job board knows you're tracking construction tech roles. A news service knows you follow building codes. Neither knows about the other. Neither knows you're building a SaaS product. Only your machine holds the full picture.

**Intelligence at the edges.** Services provide data. Your local model provides analysis. The cross-referencing — "this LinkedIn post matches a pain point your product solves" — happens on hardware you control. No cloud provider reasons over your complete competitive intelligence.

**Suggest, never act.** The system tells you what to do. It drafts the email, composes the post, identifies the contact. It never sends the email, publishes the post, or makes the call. The human stays in the loop. Every action requires a human hand. This is not a limitation. It is the design.

**Configuration is conversation.** Tell the system what you care about. It builds the pipeline. "I'm building construction management software, I want to track competitors, find potential customers, and stay current on building codes." That's the setup. The YAML is underneath for power users, but nobody has to write it by hand.

**Reports are the product.** Not chat responses. Not streaming tokens. Documents. Structured, beautiful, actionable markdown reports with inline charts, expandable sections, and suggested next steps. Reports that accumulate over time into a searchable archive. "What changed in my competitive landscape this quarter?" is a question you can ask your own data.

**Everything is a file you can read.** Configuration is YAML. Reports are markdown. Context is plain text. No binary databases, no proprietary formats. Your intelligence archive is yours in the most literal Unix sense. `cat`, `grep`, `rm -rf`. Your data, your machine, your call.

## Architecture

Burrow has four components:

**The pipeline** queries services on a schedule. Each service gets separate credentials, separate network paths. Timing is jittered so services can't correlate requests. Results are stored locally.

**The synthesis engine** feeds collected results to a local or remote LLM with a system prompt that defines your priorities and context. The model produces a structured markdown report with suggested actions. When a remote LLM is used, source attribution can be stripped so even your synthesis provider doesn't know where the data came from.

**The report viewer** renders markdown in the terminal with inline images, charts, and expandable sections. Reports are files on disk. The viewer makes them beautiful. But `cat` always works.

**The interactive mode** lets you pull threads during the day. Query a service, explore a lead, draft an email, ask your local model about patterns in your collected data. Interactive queries feed back into your context, so tomorrow's report can reference what you explored today.

## Privacy

Burrow cannot guarantee anonymity. You're using HTTPS services that require API keys. You have an IP address. Services log what they log. Each service knows something about you.

What Burrow guarantees is **compartmentalization**. SAM.gov knows you searched for NAICS codes. A news service knows you track certain topics. A job board knows you monitor certain roles. None of them know about the others. None of them can reconstruct your strategy. The only place all of this comes together is on your local machine, in a synthesis step that no external service participates in.

This is the architectural difference between Burrow and a centralized agent platform. A centralized platform sends every query through one provider. That provider can reconstruct your entire strategy from their logs — what you're bidding on, who your competitors are, who your contacts are. With Burrow, you'd need to compromise every individual service *and* your machine to assemble the same picture.

The second guarantee is the **read-only boundary**. A compromised Burrow can show you wrong information. It cannot send emails on your behalf, post to your social media, access your accounts, or act as you. It was never given the ability. The worst case is bounded by design.

## The Audience

Burrow is for people who need to stay informed but can't afford a research team — and don't trust any single provider with the full picture.

Solo founders. Independent consultants. Small firm operators. Researchers. Anyone who's currently doing competitive intelligence in browser tabs and wishes they had an analyst.

The barrier to entry is willingness to run a local model and spend five minutes telling Burrow what you care about. The conversational setup handles the rest. The reports make it worth the effort.

## Begin

Install it. Tell it what you care about. Wait until morning.

Read the brief. Pull a thread. Draft an email. Get back to work.

That's the workflow. Every day, a little more context. A little more signal. A little less time lost to research you should have done but didn't.

Your research assistant works overnight. You work during the day. Neither of you pretends to be the other.
