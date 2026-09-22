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
- Data plane: one credential set per resource connects to Target MySQL; Gateway authorization controls read and write actions;
- Protocol plane: a Streamable HTTP MCP endpoint exposes a small set of database tools.

Clients submit only a logical `resource_key`. They cannot submit a host, DSN, username, or password.

## Highlights

- Go 1.25 service using the official `modelcontextprotocol/go-sdk` at `/mcp`;
- React + TypeScript administration console with separate admin and user workspaces;
- Personal MCP access tokens with creation, revocation, and expiration support;
- Default-deny resource RBAC: `query_write ⊇ query_read ⊇ schema_read`;
- MySQL single-statement AST guard for controlled `SELECT`, `INSERT`, `UPDATE`, and `DELETE`;
- Rejects DDL, multiple statements, cross-database access, system schemas, locking reads, dangerous functions, and UPDATE/DELETE without `WHERE`;
- Separate read and write authorization paths in Gateway, sharing the resource connection pool;
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
    Policy --> Registry[Target Registry<br/>resource pools]
    Registry -->|One credential set| Target[(Target MySQL)]
    Gateway --> Audit[Audit records]
    Control --- Audit
```

The `query_sql` request path is:

1. Resolve the personal token and verify that the principal is active;
2. Load the resource and effective grants, merging the strictest row, timeout, and reason constraints;
3. Parse the SQL and reject statements outside the allowlist;
4. Check the `schema_read`, `query_read`, or `query_write` grant for the SQL action and obtain the resource pool;
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

This project does not create or store business databases. Prepare an existing MySQL reachable by the Gateway, together with one existing connection account, then start only the control database and Gateway:

```bash
cp .env.example .env
chmod 600 .env
docker compose up --build -d
```

Open <http://127.0.0.1:8080> and sign in with:

```text
Username: admin
Password: admin_123
```

Change the administrator password immediately. In the administration console, enter the existing MySQL host, port, connection account, and database password, then add one or more databases in the Database section of the resource dialog and test the connection. The Gateway encrypts the password with `TOKEN_PEPPER` before storing it in Control MySQL, and never returns the plaintext through the API. Create ordinary users, grants, and personal MCP tokens afterward.

## Production deployment: single instance, private network, persistent data

Do not use the local development `docker-compose.yml` as a production configuration. The repository includes [docker-compose.prod.yml](docker-compose.prod.yml), [.env.prod.example](.env.prod.example), and [.env.gateway-secrets.prod.example](.env.gateway-secrets.prod.example) for this topology:

- one Gateway instance;
- Control MySQL inside Compose;
- an existing external MySQL for business data, never created by the Gateway or production Compose;
- persistent control data in the `control-data` named volume, with business data backed up by the existing database owner;
- no published Control MySQL port;
- Gateway bound to `127.0.0.1:8080` by default, with no direct public exposure.

Prepare the environment file outside version control:

```bash
cp .env.prod.example .env.prod
cp .env.gateway-secrets.prod.example .env.gateway-secrets.prod
chmod 600 .env.prod .env.gateway-secrets.prod
openssl rand -hex 32
```

Replace the Control MySQL and Gateway placeholders in `.env.prod`, then validate and start the stack. New resources receive their target database passwords through the administration console and store them encrypted; `.env.gateway-secrets.prod` is retained only for legacy `secret_ref` resources:

```bash
docker compose \
  --env-file .env.prod \
  -f docker-compose.prod.yml \
  config --quiet

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

The Gateway does not create business databases, tables, or migrate business data. It also does not require dedicated read-only and read-write MySQL accounts. Use one existing account for the target instance; Gateway grants and the SQL guard control which users can read or write. A resource can use:

```text
Host:             prod-mysql.internal
Port:             3306
Database:         appdb
Username:         app_gateway
Password:         enter directly in the administration console
TLS mode:         required (when TLS is configured on Target MySQL)
```

The account must have the underlying MySQL privileges required by the business operation. Gateway `query_read` and `query_write` grants decide which Gateway user may perform an action; they do not elevate a database account that is read-only.

Legacy resources using a Secret reference can continue to use `.env.gateway-secrets.prod`:

```dotenv
DB_SECRET_APP_PROD=<app connection-account password>
```

Use another entry for another target database without changing the Control MySQL schema:

```dotenv
DB_SECRET_ORDERS_PROD=<orders connection-account password>
```

For example, `secret_ref=orders_prod` resolves to `DB_SECRET_ORDERS_PROD`. New resources should not use `secret_ref`; enter the database password directly and let the Gateway store its AES-GCM ciphertext in Control MySQL. Whether a user can read or write is controlled by the Gateway's `query_read` or `query_write` grant.

When upgrading from an older version, the migration adds an encrypted password column and keeps the old Secret reference as a fallback. Edit each old resource and enter its new password to clear the legacy reference, then test every connection.

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
| `TOKEN_PEPPER` | yes | At least 32 characters; used for token/session and target-password encryption; do not rotate casually |
| `DB_SECRET_<REFERENCE>` | legacy resources | Target password for legacy `.env.gateway-secrets.prod` references; new resources use the administration console |
| `HTTP_ADDR` | no | Defaults to `:8080` |
| `WEB_DIR` | no | Defaults to `web/dist`; `/app/web` in the container |

Do not rotate `TOKEN_PEPPER` casually. The current implementation uses it to derive token/session digests, decrypt stored token configuration, and decrypt target database passwords; replacing it invalidates existing credentials and database connections.

## Security boundaries and limitations

The current defenses include default-deny authorization, Gateway-level read/write action controls, SQL AST validation, result/timeout/write limits, audit records, and field masking. See [docs/security.md](docs/security.md) for the full checklist.

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
