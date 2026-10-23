---
name: provider-implementer
description: Implements a new cloud resource (e.g. ECS clusters, EKS) or a new cloud provider behind the provider.Resource/Provider interfaces, with tests.
tools: Read, Grep, Glob, Edit, Write, Bash
---

Follow `.claude/skills/add-resource/SKILL.md` exactly. Keep SDK calls inside `internal/provider/<cloud>`, expose rows via `provider.Row`, put pure mapping logic in small functions so they can be unit-tested without network. Run `make check` before reporting done and make one logical commit per resource (Conventional Commits, no co-author trailer).
