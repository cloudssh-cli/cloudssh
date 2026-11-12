---
name: add-resource
description: Add a new browsable cloud resource (or provider) to cloudssh with tests. Use when asked to support ECS, EKS, ASG, VPC, Azure VMs, etc.
---

1. Create `internal/provider/<cloud>/<resource>.go` implementing `provider.Resource` (`Key`, `Label`, `Name`, `Crumb`, `Columns`, `Filters`, `DefaultFilters`, `List`). `List` receives a `provider.Query{Token, Filters}` and returns the service's native pagination token via `provider.Page.NextToken`. Mark count columns `Numeric` and set `DefaultSort` on one column. Rows that open another resource: implement `provider.Drillable` (`DrillHint`, `Drill`); `Drill` may call the API and returns the child resource plus its starting filters (guard "nothing to open" cases with an error).
2. Keep SDK→row conversion in a pure function (`xxxRow(types.X) provider.Row`) and set `Row.Ref` to a typed value the UI can act on (e.g. `provider.Host` for SSH-able rows).
3. Top-level resources go in the provider's `Resources()` slice (their order is the 1..9 shortcut order); drill-only children are only constructed by their parent's `Drill`. Filters: keep a `[]filterSpec` table (key, label, API name, exact) and use `buildFilters`; every key a drill applies must also appear in the target's `Filters()`.
4. Tests: `<resource>_test.go` for the mapping function (full + missing optional fields, cells length == `len(Columns())`). Add an `integration`-tagged test only if a real call adds value.
5. A new cloud is its own package that registers with `internal/cloud`: see `.claude/skills/add-cloud` (no `switch` to edit anywhere).
6. **Demo data and tests are part of the resource:** add the resource to `allResources()` in `resources_test.go`, give the demo data rows for it, a `FilterSamples` entry per filter, and an e2e flow; see `.claude/skills/demo-data`. `providertest.Conformance` fails the build otherwise.
7. `make check`, `make build`, then commit: `feat(<resource>): list <resource>` (+ `test:`/`docs:` as separate commits if large). No co-author trailer.

Known AWS API limits to respect: ECS `DescribeServices` ≤10 ARNs, `DescribeTasks` ≤100, EKS/ECS list calls use `nextToken`.
