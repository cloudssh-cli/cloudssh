---
name: add-resource
description: Add a new browsable cloud resource (or provider) to cloudssh with tests. Use when asked to support ECS, EKS, ASG, VPC, Azure VMs, etc.
---

1. Create `internal/provider/<cloud>/<resource>.go` implementing `provider.Resource` (`Name`, `Columns`, `List`). `List` takes/returns the service's native pagination token via `provider.Page.NextToken`.
2. Keep SDK→row conversion in a pure function (`xxxRow(types.X) provider.Row`) and set `Row.Ref` to a typed value the UI can act on (e.g. `provider.Host` for SSH-able rows).
3. Register it in the provider's `Resources()` slice.
4. Tests: `<resource>_test.go` for the mapping function (full + missing optional fields, cells length == `len(Columns())`). Add an `integration`-tagged test only if a real call adds value.
5. A new cloud: new package implementing `provider.Provider`, add a `case` in `internal/cli/root.go`, extend the `--cloud` help text and README.
6. `make check`, then commit: `feat(<resource>): list <resource>` (+ `test:`/`docs:` as separate commits if large). No co-author trailer.

Known AWS API limits to respect: ECS `DescribeServices` ≤10 ARNs, `DescribeTasks` ≤100, EKS/ECS list calls use `nextToken`.
