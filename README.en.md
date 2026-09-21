# DB Access Gateway

[![CI](https://github.com/YogeLiu/db-access-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/YogeLiu/db-access-gateway/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[简体中文](README.md) · **English**

A MySQL access gateway for AI and MCP clients.

Administrators register database resources and grant access by **principal × resource × action**. The gateway keeps the real database credentials; MCP clients receive personal access tokens and never see DSNs, hosts, usernames, or database passwords.

> This is an operational first release intended for private-network, single-instance deployments. Read [Security boundaries and production checklist](docs/security.md) before exposing it to the public internet or using it in an enterprise identity environment.

## Why this exists

Giving an AI/MCP client a direct database connection makes credential leakage, excessive permissions, unsafe SQL, and missing audit trails easy to create. DB Access Gateway separates the system into three planes:

- Control plane: users, resources, grants, tokens, masking rules, and audit records;
- Data plane: controlled read-only and read-write database accounts execute queries;
- Protocol plane: a Streamable HTTP MCP endpoint exposes a small set of database tools.

Clients submit only a logical `resource_key`. They cannot submit a host, DSN, username, or password.

## Highlights

- Go 1.25 service using the official `modelcontextprotocol/go-sdk` at `/mcp`;
- React + TypeScript administration console with separate admin and user workspaces;
- Personal MCP access tokens with creation, revocation, and expiration support;
- Default-deny resource RBAC: `query_write ⊇ query_read ⊇ schema_read`;
- MySQL single-statement AST guard for controlled `SELECT`, `INSERT`, `UPDATE`, and `DELETE`;
- Rejects DDL, multiple statements, cross-database access, system schemas, locking reads, dangerous functions, and UPDATE/DELETE without `WHERE`;
- Separate read and write database accounts and connection pools;
- Row, result-size, statement-timeout, and write-affected-row limits;
- Re-checks principal status, grants, and resource versions before returning results or committing writes;
- Audit records contain SQL text and a SHA-256 fingerprint, but not parameter values, results, or database passwords;
- Per-resource, per-table, and per-column plain/partial/full masking rules;
- Connection health checks, broken-connection removal, concurrent connection coalescing, and request cancellation propagation.

## Architecture

This is a modular monolith with separate control-plane and data-plane responsibilities. One Go process serves the console, REST API, and MCP handler. The control MySQL stores policy and state; target MySQL instances store business data.

```mermaid
flowchart LR
    Admin[Admin browser] -->|Session Cookie| Gateway[Go Gateway]
    MCP[MCP client] -->|Personal Access Token| Gateway
    Gateway --> Control[(Control MySQL)]
    Gateway --> Policy[Authorization and SQL Guard]
    Policy --> Registry[Target Registry<br/>read/write pools]
    Registry -->|Read account| Target[(Target MySQL)]
    Registry -->|Write account| Target
    Gateway --> Audit[Audit records]
    Control --- Audit
```

The `query_sql` request path is:

1. Resolve the personal token and verify that the principal is active;
2. Load the resource and effective grants, merging the strictest row, timeout, and reason constraints;
3. Parse the SQL and reject statements outside the allowlist;
4. Select the read or write database account;
5. Execute the query and re-check authorization before returning rows or committing a write transaction;
6. Update the final audit status.

## Authorization model

| Grant | `schema_read` | `query_read` | `query_write` |
| --- | ---: | ---: | ---: |
| `schema_read` | allow | deny | deny |
| `query_read` | allow | allow | deny |
| `query_write` | allow | allow | allow |

Authorization is default-deny. When multiple grants apply, `require_reason` is combined with OR semantics, while row and timeout limits use the strictest value.

## Quick start: connect an existing MySQL

This project does not create or store business databases. Prepare an existing MySQL reachable by the Gateway, together with separate read-only and read-write accounts, then start only the control database and Gateway:

```bash
docker compose up --build -d
```

Open <http://127.0.0.1:8080> and sign in with:

```text
Username: admin
Password: admin_123
```

Change the administrator password immediately. In the administration console, register the existing MySQL host, port, database name, read account, and write account, then test the connection. Create ordinary users, grants, and personal MCP tokens afterward.

## Production deployment: single instance, private network, persistent data

Do not use the demo `docker-compose.yml` as a production configuration. The repository includes [docker-compose.prod.yml](docker-compose.prod.yml) and [.env.prod.example](.env.prod.example) for this topology:

- one Gateway instance;
- Control MySQL inside Compose;
- an existing external MySQL for business data, never created by the Gateway or production Compose;
- persistent control data in the `control-data` named volume, with business data backed up by the existing database owner;
- no published Control MySQL port;
- Gateway bound to `127.0.0.1:8080` by default, with no direct public exposure.

Prepare the environment file outside version control:

```bash
cp .env.prod.example .env.prod
chmod 600 .env.prod
openssl rand -hex 32
```

Replace every placeholder in `.env.prod`, then validate and start the stack:

```bash
docker compose \
  --env-file .env.prod \
  -f docker-compose.prod.yml \
  config

docker compose \
  --env-file .env.prod \
  -f docker-compose.prod.yml \
  up -d --build
```

Verify the gateway:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

`/readyz` checks only Control MySQL. Test every target resource separately from the administration console.

### Configure an existing target database

The Gateway does not create business databases, tables, or migrate business data. Before enabling a resource, a database administrator should provision least-privilege accounts on the existing target MySQL. If the accounts already exist, use them directly:

```sql
CREATE USER 'gateway_read'@'%' IDENTIFIED BY '<read-only password>';
GRANT SELECT, SHOW VIEW ON appdb.* TO 'gateway_read'@'%';

CREATE USER 'gateway_write'@'%' IDENTIFIED BY '<read-write password>';
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON appdb.* TO 'gateway_write'@'%';
```

Restrict `'%'` to the gateway's controlled network range in production. A resource can use:

```text
Host:             prod-mysql.internal
Port:             3306
Database:         appdb
Read username:    gateway_read
Read secret ref:  prod_read
Write username:   gateway_write
Write secret ref: prod_write
TLS mode:         required (when TLS is configured on Target MySQL)
```

`prod_read` resolves to `DB_SECRET_PROD_READ`; `prod_write` resolves to `DB_SECRET_PROD_WRITE`. Add more `DB_SECRET_<REFERENCE>` variables for additional resources or credential pairs.

The Gateway applies control-database migrations on first startup. Change `admin / admin_123` before allowing internal users to access the service. Plain `docker compose down` preserves `control-data`; never use `docker compose down -v` in production. Business-data persistence and backups remain the responsibility of the existing target MySQL.

If other private-network machines need access, bind the Gateway port to the server's private IP instead of `127.0.0.1` and restrict the firewall to the private network.

## MCP client configuration

MCP clients must use a normal user's token, never `ADMIN_TOKEN`:

```json
{
  "mcpServers": {
    "database-gateway": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": {
        "Authorization": "Bearer dbag_REPLACE_WITH_USER_TOKEN"
      }
    }
  }
}
```

When an internal HTTPS proxy is used in production, replace the URL with the proxy's `/mcp` address.

Available tools:

- `list_databases`: list logical resources granted to the current principal;
- `list_tables`: requires `schema_read` or higher;
- `describe_table`: requires `schema_read` or higher;
- `query_sql`: `SELECT` requires `query_read`; DML requires `query_write`.

Example arguments:

```json
{
  "resource_key": "orders",
  "sql": "SELECT id, customer, amount FROM orders WHERE id = ?",
  "params": [{"type": "int", "value": "1"}],
  "max_rows": 50,
  "reason": "investigate order 1"
}
```

## Configuration reference

| Variable | Required | Description |
| --- | --- | --- |
| `CONTROL_DSN` | yes | Control MySQL DSN; migrations run at startup |
| `ADMIN_TOKEN` | yes | At least 20 characters; legacy admin API credential, store as a high-sensitivity secret |
| `TOKEN_PEPPER` | yes | At least 32 characters; used for token digests, sessions, and token encryption |
| `DB_SECRET_<REFERENCE>` | per resource | Target database password; references are normalized into environment variable names |
| `HTTP_ADDR` | no | Defaults to `:8080` |
| `WEB_DIR` | no | Defaults to `web/dist`; `/app/web` in the container |

Do not rotate `TOKEN_PEPPER` casually. The current implementation uses it to derive existing token/session digests and decrypt stored token configuration; replacing it invalidates existing credentials.

## Security boundaries and limitations

The current defenses include default-deny authorization, separate read/write accounts, SQL AST validation, result/timeout/write limits, audit records, and field masking. See [docs/security.md](docs/security.md) for the full checklist.

Known boundaries:

- no enterprise SSO, MFA, fine-grained administrator roles, or complete CSRF protection;
- environment-based secret resolution is not a full Secret Manager integration;
- the control-database audit table is not an immutable ledger;
- no general-purpose column-level DLP or row-level security policy;
- single-instance deployment is the simplest mode; multiple replicas require migration locking, MCP session strategy, and connection-pool budgeting.

The first release is therefore best kept on a private network or VPN with restricted administration access.

## Development and verification

Backend:

```bash
go test ./...
go vet ./...
go run ./cmd/gateway
```

Frontend:

```bash
cd web
npm ci
npm run dev
```

Full check:

```bash
make check
```

See [docs/verification.md](docs/verification.md) for the verification record.

## Repository layout

```text
cmd/gateway/        service entrypoint, health checks, static site
internal/api/       admin REST API, sessions, and MCP adapter
internal/authz/     action hierarchy, default deny, constraint merging
internal/sqlguard/  MySQL AST safety checks
internal/query/     authorization, execution, re-checks, and audit
internal/target/    target pools and secret-reference resolution
internal/control/   control-plane models, migrations, and storage
internal/masking/   field masking rules and SQL rewriting
web/                React administration console
docs/               security, research, and verification notes
```

## Contributing and license

Run `make check` before submitting changes. Changes involving authorization, SQL guards, tokens, or secrets should include boundary tests and an updated security note.

This project is licensed under Apache License 2.0. See [NOTICE](NOTICE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for attribution and derived-code details.
