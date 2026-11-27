# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

This is a **single-context** repo: one `GLOSSARY.md` and one `docs/adr/` at the repo root.

## Before exploring, read these

- **`GLOSSARY.md`** at the repo root.
- **`docs/adr/`**: read ADRs that touch the area you're about to work in.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

```
/
├── GLOSSARY.md
├── docs/adr/
│   ├── 0001-example-decision.md
│   └── 0002-another-decision.md
└── internal/, web/, chatbot/, local/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `GLOSSARY.md`. Don't drift to synonyms the glossary explicitly avoids.

LoopForge's own vocabulary is load-bearing and defined in this repo's `AGENTS.md` and `docs/`. Notably: a **LOOP** is the reusable delivery workflow (Agent / script / human-gate / sub-LOOP nodes); a **run** is one execution of a LOOP or conversation workspace; a **role** is a reusable agent definition (prompt, executor, model, rules, skills, MCP tools); a **sandbox** is the isolated execution environment (Docker or native). Prefer these terms over generic synonyms like "workflow" (which upstream uses for a different concept: `Workflow` rows in `CatalogRepo`).

Note that these upstream terms describe capabilities the personal build removes. `GLOSSARY.md` gives the terms MuseForge actually uses; where the two disagree, `GLOSSARY.md` wins.

If the concept you need isn't in the glossary yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (event-sourced orders), but worth reopening because…_
