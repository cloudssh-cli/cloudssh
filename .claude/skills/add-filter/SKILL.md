---
name: add-filter
description: Add a server-side filter to an AWS resource in cloudssh (shown in the F dialog). Use when asked to filter EC2/VPC/subnet lists by another field.
---

Filters are table-driven in `internal/provider/aws/filters.go`.

1. Add a `filterSpec{key, name, api, exact}` row to the resource's slice (`ec2Specs`, `vpcSpecs`, `subnetSpecs`):
   - `key`: stable id used by drills and defaults (kebab-case).
   - `name`: label shown in the dialog.
   - `api`: the EC2 `Filter.Name` (e.g. `tag:Name`, `instance-type`).
   - `exact`: true for IDs; otherwise the value is wrapped as `*value*`.
2. The resource's `Filters()` already returns `defsOf(specs)`, so the dialog lists it automatically (one row per filter, no cycling).
3. Defaults go in the resource's `DefaultFilters()` (EC2: `state=running`). A drill supplies its own filters in `provider.Drill.Filters`, which replace the defaults on that screen; every key a drill uses must exist in the *target* resource's specs (see `TestEveryDrilledFilterKeyIsKnownToTarget`).
4. Test with `buildFilters` in `resources_test.go` (wildcard vs exact, empty value skipped).
5. Services other than EC2/VPC/subnets (ASG, ECS, EKS) have no server-side filters; the UI shows "no filters available" for them.
