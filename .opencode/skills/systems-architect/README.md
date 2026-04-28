# systems-architect

Principal-level software architecture skill. Turns Claude into a pragmatic senior advisor for design, modernization, and architectural decision-making.

Works in both **Claude.ai** and **Claude Code**. Same files, different install paths.

---

## What it does

When triggered, the skill puts Claude into "principal architect" mode:

- Uses a consistent decision framework: goal → constraints → options → trade-offs → one recommendation → phased rollout → risks
- Defaults to pragmatic choices (modular monolith over microservices unless justified; incremental modernization over rewrites)
- Produces structured output with Mermaid diagrams when useful
- Pushes back on bad premises rather than going along with them
- In Claude Code: reads the actual repo (CLAUDE.md, ADRs, package files, deployment configs) before prescribing architecture

---

## Files

```
systems-architect/
├── SKILL.md               # Entry point, invocation logic, core behavior
├── principles.md          # Operating principles with reasoning
├── workflows.md           # Step-by-step playbooks per request type
├── patterns.md            # Reusable architecture and diagram patterns
├── decision-framework.md  # Comparison templates, trade-off logic
├── examples.md            # Sample prompts and model outputs
├── microservices.md       # Service decomposition heuristics
├── ddd.md                 # Bounded contexts, context maps
├── api.md                 # REST/GraphQL/gRPC/event API design and governance
├── scalability.md         # Caching, messaging, DB scaling playbooks
└── anti-patterns.md       # Common mistakes, named
```

Only `SKILL.md` is always in context. The other files load on demand when the conversation touches their domain.

---

## Install in Claude.ai

1. Go to **Customize → Skills** in the sidebar
2. Click the upload / "+" button
3. Upload `systems-architect.skill` (the packaged zip)
4. Toggle it on

The skill triggers automatically when you ask architecture-shaped questions. No slash command in Claude.ai.

---

## Install in Claude Code

Two options.

### Personal (all projects)

```bash
# Extract into your personal skills directory
mkdir -p ~/.claude/skills
unzip systems-architect.skill -d ~/.claude/skills/

# Or clone/copy the directory directly
cp -r systems-architect ~/.claude/skills/
```

Available in every Claude Code session. Restart if Claude Code was already running when you installed.

### Project-scoped (this repo only, checked in)

```bash
mkdir -p .claude/skills
cp -r systems-architect .claude/skills/
git add .claude/skills/systems-architect
git commit -m "Add systems-architect skill"
```

Everyone who clones the repo gets it automatically. Good for teams that want consistent architectural reasoning across the codebase.

### Verify install

```bash
ls ~/.claude/skills/systems-architect/SKILL.md    # personal
ls .claude/skills/systems-architect/SKILL.md      # project
```

### Invoke

Either let Claude load it automatically ("should we split this service?") or call it directly:

```
/systems-architect review the current service topology
```

The skill is pre-approved for `Read`, `Grep`, and `Glob` via `allowed-tools` in the frontmatter — it can read the repo without prompting for each file, but it cannot modify code. Architecture advice, not agentic refactoring.

---

## Uninstall

- **Claude.ai:** toggle off in **Customize → Skills**, or delete
- **Claude Code:** `rm -rf ~/.claude/skills/systems-architect/` (or from `.claude/skills/` for project-scoped)

---

## Known behavior to be aware of

- **Opinionated by design.** The skill will push back on "let's go microservices" or "let's rewrite the legacy system" unless the context clearly justifies it. This is deliberate — architecture advice that hedges on everything is useless. If you want pure neutrality, this isn't the right skill.
- **Mermaid diagrams.** The skill defaults to Mermaid for diagrams. If you're rendering somewhere that doesn't support it, ask for ASCII.
- **Reference files are loaded on demand.** If the skill triggers but doesn't seem to have the depth you expected on a specific topic (e.g., API governance), mention the topic explicitly — that cues loading of the relevant reference file.
- **Claude Code repo reading.** When invoked in Claude Code, it will read a handful of repo files (CLAUDE.md, README, package files, deployment configs) before answering. If you want a pure theoretical answer divorced from your codebase, say so explicitly.
