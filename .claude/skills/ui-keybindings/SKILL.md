---
name: ui-keybindings
description: Add or change a key binding in the cloudssh TUI without breaking precedence, sort keys, help, hints or docs. Use when adding any shortcut, dialog or mode.
---

Key handling lives in `internal/ui/model.go` (`handleKey` → `updateKey`).

**Precedence (first match wins):** ctrl+c always quits → splash (any key skips) → open modal (`modal.key`) → search input → normal keys. A new dialog must implement the `modal` interface (`key`, `view`) so it swallows every key; never read `q`, `d`, digits or `?` outside `updateKey`.

**Reserved shift+letter keys:** `S` ssh, `F` filters, `R` reset, `G` bottom. Column sort keys are derived in `sortkeys.go` (`reservedKeys`); add a new shift+letter binding there too or a column will collide with it.

**Other fixed keys:** `d 0` dashboard, `1-9` modes, `? q / f s r n enter esc j k h l g`, ctrl+b/f page. ctrl+h is backspace in most terminals: never bind it.

**Checklist for a new binding**
1. Handle it in `updateKey` (or the modal's `key`).
2. Header hint in `Model.hints()` (list is uncapped; most important first; the header drops trailing columns on narrow terminals). `<?> Help` stays on the footer so it is always visible.
3. Entry in `help.go` (`newHelp`).
4. README key table.
5. Tests in `nav_test.go`/`model_test.go`: the key works, it does not leak from search/modals, and `TestEveryStateFitsTheScreen` still passes.
6. Verify the look with the `tui-preview` skill.
