---
name: ui-keybindings
description: Add or change a key binding in the cloudssh TUI without breaking precedence, sort keys, help, hints or docs. Use when adding any shortcut, dialog or mode.
---

Key handling lives in `internal/ui/model.go` (`handleKey` → `updateKey`).

**Precedence (first match wins):** ctrl+c always quits → splash (any key skips) → open modal (`modal.key`) → search input → normal keys. A new dialog must implement the `modal` interface (`key`, `view`) so it swallows every key; never read `q`, `d`, digits or `?` outside `updateKey`.

**Reserved shift+letter keys:** `S` ssh, `F` filters, `R` reset, `G` bottom. Column sort keys are derived in `sortkeys.go` (`reservedKeys`); add a new shift+letter binding there too or a column will collide with it.

**Prefer context over chords.** Do not add ctrl+key shortcuts. Inside a dialog, make the context the interface: tab / shift+tab / up / down move between fields, left/right (or space) change a choice on a focused row, enter confirms, esc cancels, and typing only goes into a focused text box (see `ssh_modal.go`, `filters_modal.go`). A new option becomes a new field, not a new key.

**Other fixed keys:** `d 0` dashboard, `1-9` modes, `[ ]` back/forward through drills, `{ }` previous/next view, `? q / f s r n enter esc j k h l g`, ctrl+b/f page. ctrl+h is backspace in most terminals: never bind it.

**Checklist for a new binding**
1. Handle it in `updateKey` (or the modal's `key`).
2. Header hint in `Model.hints()` only for *action* keys (ssh, filters, search, refresh, reset, next page, back, views, dashboard, help, quit). Navigation (move/page/top/bottom/scroll), column sorting and the per-screen drill `enter` are intentionally NOT in the header: they live in `help.go` (`?`). `? Help` is always the first hint (first column) so it survives narrow terminals; it is not repeated in the footer.
3. Entry in `help.go` (`newHelp`).
4. README key table.
5. Tests in `nav_test.go`/`model_test.go`: the key works, it does not leak from search/modals, and `TestEveryStateFitsTheScreen` still passes.
6. Verify the look with the `tui-preview` skill.


## Colours and themes
Never write a colour literal in `internal/ui`. Every colour is a role of `Theme` (`theme.go`); styles are rebuilt from it by `applyTheme`. Use the existing styles (`hintKeyStyle`, `helpKeyStyle`, …) or add a role to *every* theme (`TestThemesAreCompleteAndDistinct` checks). The help page deliberately uses its own roles (`HelpKey`, `HelpHead`) so it does not look like the header.
