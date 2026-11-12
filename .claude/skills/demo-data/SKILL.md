---
name: demo-data
description: Create or change the built-in demo data (cloudssh --demo) for a cloud without letting it drift from the real provider. Use whenever you add or change a resource, column, filter, drill-down or API call, or add a cloud.
---

Demo mode runs the **real** provider code against fake SDK clients that return in-memory **SDK types**. Nothing is hand-written as table rows, so columns, mapping, filters, paging and drill-downs are the production ones. See `internal/provider/aws/demo_*.go` for the model to copy.

## Rules that prevent drift

1. **Data is the SDK's own types** (`ec2types.Instance`, ...). The compiler enforces the schema. Never fabricate `provider.Row`s for the demo.
2. **Production calls the SDK through small interfaces** (`aws/api.go`); the demo implements them. A new SDK call forces a new interface method, which forces the fake to implement it.
3. **The fake behaves like the real API where code can trip:** page limits and tokens, per-call item limits (ECS describe <= 10), value ranges, wildcard filters, and an **error for any filter name it does not implement**. Add the limit when you meet it in the real docs.
4. **Cross-service consistency:** an ASG's instances are the instances tagged with it; counters are derived from the data, never typed twice; every id points at something that exists. `TestDemoDataIsConsistentAndShapedLikeAWS` enforces this: extend it when you add relations.
5. **Realistic, believable names** (the demo is recorded as the project GIF): `web-prod-1`, `postgres-primary`, `prod-eks`, not `foo`, `test`, `example`, `fake`, `sample`, `dummy`. `TestDemoNamesLookRealistic` enforces it.
6. **Safe values only:** IPv4 from RFC 5737 (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) for public addresses, IPv6 `2001:db8::/32`, private ranges for private ones, made-up 12-digit account ids. Ids/ARNs must have the real shape (`i-0` + 16 hex).
7. **Deterministic:** no `math/rand`, no wall-clock values except relative offsets from the injected `now`.
8. **Cover every state the UI must handle:** stopped/pending/terminated, no public IP, empty accounts, a resource with nothing to open (the guard errors), Fargate-like cases, enough rows for several pages (the demo runs at page size 8), more than one account/profile so switching shows different data.
9. **Demo ssh:** never run real `ssh` in demo mode (the UI swaps in `Target.DemoCommand()`); do not add code paths that contact the network.

## When you change something

- **New resource:** register it in the package's `allResources()` test helper (this makes the conformance test require sample data for it), add data to every relevant demo account, add a `FilterSamples` entry for each filter it advertises (or make a drill-down apply the filter), and add its flow to `internal/e2e/e2e_test.go`.
- **New column/filter/drill:** update the data so it is non-empty and meaningful; `providertest.Conformance` fails if a filter returns nothing or a drill target does not list the filter it applies.
- **New cloud:** build `Demoer` + `Authenticator` demo with at least two profiles/accounts, `NewDemo(pageSize)`, and one `TestDemoConformance` calling `providertest.Conformance`. Add an `internal/e2e` flow set for it.

## Verify

```sh
make check                 # includes conformance + consistency + realistic-name tests + e2e
make build && ./bin/cloudssh --cloud <name> --demo --page-size 5
```
Then look at it (`.claude/skills/tui-preview`) at 80x24 and 120x30.
