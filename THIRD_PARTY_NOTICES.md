# Third-party source notices

## GoNavi

- Project: <https://github.com/YogeLiu/GoNavi>
- Revision: `60ea586c674259d4214ad7d7e75dddfa706e577a`
- Copyright: 2026 Syngnat
- License: Apache License 2.0

Modified portions used by this project:

| This project | GoNavi source | Modification |
|---|---|---|
| `internal/target/database.go` | `internal/db/mysql_impl.go`, `internal/db/sql_pool.go` | Removed desktop, SSH, multi-database and compatibility retry dependencies; kept the MySQL adapter lifecycle and pool policy; forced `multiStatements=false`. |
| `internal/target/registry.go` | `internal/app/app.go` | Extracted cache, health check, stale connection eviction, singleflight connect and return-time validation; added resource version, read/write mode and generation invalidation. |
| `internal/target/registry_test.go` | `internal/app/app_db_cache_concurrency_test.go` | Adapted concurrent cold-connect and invalidated-flight tests to the gateway registry. |

The original GoNavi tree was not copied wholesale because its database package
also contains Wails UI integration, SSH/proxy support and many unrelated database
drivers. Derived files carry prominent modification notices as required by
Apache-2.0.

