# QA #77 · v1 · base `134efa9`

## Commands
- `pnpm nx run api:test --skip-nx-cache`: exit 0 · 43 packages ok · 0 FAIL
- `pnpm nx run api:test --skip-nx-cache --args="-run='TestMarketing|TestEveryTableIsTenantIsolated|TestSchemaDataClasses|TestNoMissingOkTenantSetting' -v"`: exit 0

```
ok   internal/promoter/marketing
ok   internal/promoter/migrations
--- PASS: TestMarketingTables_TenantIsolation (0.02s)
--- PASS: TestMarketingTables_FailClosedWithoutTenant (0.00s)
--- PASS: TestMarketingTables_CannotWriteForOtherTenant (0.00s)
--- PASS: TestMarketingTables_CannotMoveRowToOtherTenant (0.01s)
--- PASS: TestMarketingSends_DeletedWithContact (0.00s)
--- PASS: TestSchemaDataClasses (0.16s)
--- PASS: TestEveryTableIsTenantIsolated (1.93s)
--- PASS: TestNoMissingOkTenantSetting (0.03s)
```

## Red first (mutation, never committed)
- `marketing_sends` policy `WITH CHECK (true)` and contact FK `ON DELETE SET NULL`
- `TestMarketingTables_CannotMoveRowToOtherTenant` FAIL · `TestMarketingSends_DeletedWithContact` FAIL
- the builder's `CannotWriteForOtherTenant` stayed green: it only covered `marketing_suppressions`
- migration restored, both tests green
