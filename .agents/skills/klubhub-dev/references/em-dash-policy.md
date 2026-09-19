# Em dash policy — KlubHub

Em dashes (`—`, U+2014) are forbidden in some places, allowed in others. This file is the rule.

## Forbidden (recipient-facing)

Replace em dashes with `, ` (comma + space) in:

- `apps/site/public/index.html` — KlubHub.io body copy, page title, og:title, newsletter lede
- `apps/site/emails/*.html` — every email template (`welcome.html`, `onboarding.html`, future templates)
- Any future recipient-facing HTML — landing pages, transactional emails, error pages

Why: em dashes don't render reliably across email clients (some mobile clients substitute a hyphen, some omit entirely), and they add visual weight to short marketing copy that reads better with a comma. The email-client rendering issue alone is enough to forbid them.

## Allowed (dev-facing)

Em dashes are fine in:

- Code comments (`//`, `/* */`, `#`)
- HTML comments (`<!-- -->`)
- `README.md`, `CLAUDE.md`, `AGENTS.md`, `.hermes.md`
- `docs/*.md`
- `SECURITY.md`, `CONTRIBUTING.md`
- Commit messages, PR descriptions, issue bodies
- Any file a developer reads but a recipient doesn't

## Replacement

When replacing, the convention is **comma + space**, not just comma. Examples:

| Before | After |
|---|---|
| `KlubHub — Tools for the independent music scene` | `KlubHub, Tools for the independent music scene` |
| `lands — no marketing noise.` | `lands, no marketing noise.` |
| `KlubHub DJ v1 is shipping — KlubHub Promoter and Label are next.` | `KlubHub DJ v1 is shipping, KlubHub Promoter and Label are next.` |

Exception: if a sentence naturally calls for a colon or semicolon, use that instead. Don't force a comma where it reads awkwardly. The rule is "no em dashes in recipient-facing copy"; the substitution is at your discretion.

## How to verify

```bash
# In recipient-facing files only:
grep -n ' — ' apps/site/public/index.html apps/site/emails/*.html
# Should return 0 matches.
```

In dev-facing files (comments, docs), em dashes are allowed and don't need to be flagged.

## Originals kept

The very first em-dash sweep was applied surgically — the README, docs, and HTML comments were preserved even when the body copy was changed. This is the right call: the original copy (in git history) is the source of truth, and the cleaned-up copy is what recipients see. Replacements are documented per commit.
