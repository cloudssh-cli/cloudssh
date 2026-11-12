---
name: tui-preview
description: Render cloudssh TUI screens as plain text to check layout without a real terminal or AWS. Use after any change to internal/ui (table, header, footer, dialogs, dashboard, splash).
---

The TUI cannot be driven from this environment, so verify layout by rendering `Model.View()` in a throwaway test inside the dev container.

1. Create `internal/ui/zz_preview_test.go` (never commit it; delete it afterwards):
   ```go
   package ui

   import ("fmt"; "testing")

   func TestZZPreview(t *testing.T) {
       m := newModelWith(testCfg(), filterable(&recorder{}), fakeResource{key: "o"})
       m = sized(settle(m, m.Init()), 110, 22)   // width, height
       fmt.Println(m.View())
       f, _ := send(m, key("F"))                   // any key press / overlay
       fmt.Println(f.View())
   }
   ```
   Helpers live in `helpers_test.go` (`newModelWith`, `settle`, `send`, `key`, `typeKeys`, `sized`, `fakeResource`, `drillRes`, `fakeProvider`) and `nav_test.go` (`twoLevel`, `filterable`).
2. Run it and strip colour codes:
   `docker compose run --rm dev go test -run ZZPreview -v ./internal/ui 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -v Container`
3. Check at least 120x30 and 80x24; also 50x12 for overlays.
4. Delete the preview file.

For a permanent guard, extend `TestEveryStateFitsTheScreen` in `nav_test.go`: every state must render exactly `height` lines, none wider than `width`.
