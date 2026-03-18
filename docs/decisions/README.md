# Decision Log

This directory stores architecture and product decisions for DARE Platform.

## Format
Each decision is one markdown file named:

`YYYY-MM-DD-<slug>.md`

Each file should include these sections:

1. `Status`: Proposed | Accepted | Superseded
2. `Context`: why the decision is needed
3. `Decision`: the selected option
4. `Alternatives Considered`: options evaluated
5. `Consequences`: tradeoffs, follow-up work, and risks
6. `References`: source docs/sections used

## Precedence
When conflicts exist, use this order:

1. `DECISIONS.md` (root)
2. `docs/decisions/*.md` (this folder)
3. `docs/planning/Developer Handoff Master Document.txt`
4. `docs/planning/Plan.txt`
5. `TODO.md`

## Update Rules
- Do not silently change an accepted decision; create a new decision file that supersedes the old one.
- Link superseding decisions in both files.
- Keep decisions implementation-oriented and concrete.
