# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for significant architectural decisions in the DeckFS project.

## What is an ADR?

An ADR documents:
- **Context**: What problem are we solving?
- **Decision**: What did we decide to do?
- **Consequences**: What are the trade-offs?

ADRs help future contributors understand **why** the code is structured the way it is.

## Format

We use the following format:
- **Status**: Proposed / Accepted / Deprecated / Superseded
- **Date**: When was this decision made?
- **Context**: What's the background and problem?
- **Decision**: What did we decide?
- **Consequences**: What are the trade-offs?

## Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](001-deck-processing-architecture.md) | Deck Processing Architecture Refactoring | Proposed | 2026-01-27 |
| [002](002-auto-rebuild-on-file-changes.md) | Auto-Rebuild on File Changes | Accepted | 2026-01-27 |

## How to Propose a New ADR

1. Copy template from `000-template.md` (create it if needed)
2. Number it sequentially (next is 002)
3. Fill in all sections with detail
4. Submit PR for team review
5. Update this README index
6. Update status to "Accepted" when merged

## Decision Process

1. **Proposed** - Under discussion
2. **Accepted** - Team consensus, ready to implement
3. **Deprecated** - No longer recommended
4. **Superseded** - Replaced by newer ADR

## Questions?

See [CLAUDE.md](../../CLAUDE.md) for architectural overview and current state.
| [002](002-auto-rebuild-on-file-changes.md) | Auto-Rebuild on File Changes | Accepted | 2026-01-27 |
