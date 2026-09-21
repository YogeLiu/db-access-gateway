# DB Access Gateway

[![CI](https://github.com/YogeLiu/db-access-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/YogeLiu/db-access-gateway/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**中文** · [English](README.en.md)

一个面向 AI/MCP 客户端的 MySQL 访问网关。

管理员登记数据库资源，再按“用户 × 资源 × 动作”分配权限。网关持有真实数据库凭据，MCP 客户端只获得个人 Access Token，不会接触 DSN、主机或数据库密码。

> 项目当前是一个可运行的首版，适合内网、单实例部署。公网或企业级部署前，请先阅读[安全边界与生产检查单](docs/security.md)。

## 解决什么问题

直接把数据库连接交给 AI/MCP 客户端，会带来凭据泄露、权限过大、审计缺失和危险 SQL 等问题。DB Access Gateway 将访问拆成三层：

- 控制面：管理用户、资源、授权、Token、脱敏规则和审计记录；
- 数据面：使用受控的只读/读写数据库账号执行查询；
- 协议面：通过 Streamable HTTP MCP 暴露有限的数据库工具。

客户端只提交逻辑资源名 `resource_key`，不能提交主机、DSN、用户名或密码。

## 主要特性

- Go 1.25 服务，使用官方 `modelcontextprotocol/go-sdk` 暴露 `/mcp`；
- React + TypeScript 管理台，支持管理员和普通用户工作区；
- 个人 MCP Access Token，可创建、查看、撤销和设置有效期；
- 默认拒绝的资源级 RBAC：`query_write ⊇ query_read ⊇ schema_read`；
- MySQL 单语句 AST 守卫，只允许受控的 `SELECT`、`INSERT`、`UPDATE`、`DELETE`；
- 拒绝 DDL、多语句、跨库、系统库、锁定读取、危险函数和无 `WHERE` 的更新/删除；
- 读写数据库账号和连接池隔离；
- 行数、结果大小、语句超时和写入影响行数限制；
- 授权、资源版本和用户状态在执行前后复核，写事务提交前再次校验；
- 保存 SQL 文本和 SHA-256 指纹的审计记录，不保存参数值、结果或数据库密码；
- 支持按资源、表和字段配置明文、部分脱敏和全脱敏规则；
- 内置连接池健康检查、坏连接剔除、并发建连合并和请求取消传递。

## 架构

这是一个“模块化单体 + 控制面/数据面分离”的服务。管理台、REST API 和 MCP Handler 由同一个 Go 进程提供，控制库保存策略和状态，目标 MySQL 保存业务数据。

```mermaid
flowchart LR
    Admin[管理员浏览器] -->|Session Cookie| Gateway[Go Gateway]
    MCP[MCP 客户端] -->|个人 Access Token| Gateway
    Gateway --> Control[(Control MySQL)]
    Gateway --> Policy[授权与 SQL Guard]
    Policy --> Registry[Target Registry<br/>读写连接池]
    Registry -->|只读账号| Target[(Target MySQL)]
    Registry -->|读写账号| Target
    Gateway --> Audit[审计记录]
    Control --- Audit
```

一次 `query_sql` 请求的大致流程：

1. 用个人 Token 查找用户并确认账号仍处于启用状态；
2. 查询资源和有效授权，合并最严格的行数、超时和原因约束；
3. 解析 SQL，拒绝不符合白名单的语句；
4. 根据动作选择只读或读写账号；
5. 执行查询，并在返回结果或提交写事务前重新检查授权；
6. 更新最终审计状态。

## 权限模型

| 授权 | `schema_read` | `query_read` | `query_write` |
| --- | ---: | ---: | ---: |
| `schema_read` | 允许 | 拒绝 | 拒绝 |
| `query_read` | 允许 | 允许 | 拒绝 |
| `query_write` | 允许 | 允许 | 允许 |

没有匹配授权时默认拒绝。多个授权同时适用时，`require_reason` 取并集，行数和超时取更严格的限制。

## 快速开始：演示环境

需要 Docker Compose：

```bash
docker compose up --build -d
```

打开 <http://localhost:8080>，使用默认管理员登录：

```text
账号：admin
密码：admin_123
```

首次登录后立即修改管理员密码。也可以安装 `jq` 后执行演示引导脚本，创建用户、资源、授权和个人 Token：

```bash
./scripts/bootstrap-demo.sh
```

脚本会输出 `USER_A_TOKEN` 和 `USER_C_TOKEN`；Token 只在创建时显示一次。演示数据使用全新的 Docker 卷，清空演示环境时才执行：

```bash
docker compose down -v
```

## 生产部署：单实例、内网、持久化

生产环境不要直接使用演示用的 `docker-compose.yml`。仓库提供了独立的 [docker-compose.prod.yml](docker-compose.prod.yml) 和 [.env.prod.example](.env.prod.example)，满足以下拓扑：

- 一个 Gateway 实例；
- Control MySQL 和 Target MySQL 都运行在 Compose 中；
- 数据保存到 `control-data` 和 `target-data` 命名卷；
- MySQL 不发布宿主机端口；
- Gateway 默认只绑定 `127.0.0.1:8080`，不直接对公网开放。

准备环境变量：

```bash
cp .env.prod.example .env.prod
chmod 600 .env.prod
openssl rand -hex 32
```

将 `.env.prod` 中的占位值全部替换，然后启动：

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

验证服务：

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

`readyz` 只检查 Control MySQL。目标库需要在管理台中单独执行连接测试。

### 配置目标数据库

`target-mysql` 不会自动创建业务库和最小权限账号。启用资源前，根据实际业务库执行类似 SQL：

```sql
CREATE DATABASE appdb CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE USER 'gateway_read'@'%' IDENTIFIED BY '<只读密码>';
GRANT SELECT, SHOW VIEW ON appdb.* TO 'gateway_read'@'%';

CREATE USER 'gateway_write'@'%' IDENTIFIED BY '<读写密码>';
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON appdb.* TO 'gateway_write'@'%';
```

生产环境应将 `'%'` 收紧为网关所在的受控网络范围。管理台资源配置示例：

```text
Host:             target-mysql
Port:             3306
Database:         appdb
Read username:    gateway_read
Read secret ref:  prod_read
Write username:   gateway_write
Write secret ref: prod_write
TLS mode:         required（目标 MySQL 已配置 TLS 时）
```

`prod_read` 对应 `DB_SECRET_PROD_READ`，`prod_write` 对应 `DB_SECRET_PROD_WRITE`。多资源或多组账号时，按同样规则增加 `DB_SECRET_<引用名>` 环境变量。

控制库会在 Gateway 首次启动时自动迁移。首次登录仍是 `admin / admin_123`，必须在允许内网用户访问前修改。普通的 `docker compose down` 不会删除命名卷；生产环境不要执行 `docker compose down -v`。持久化不等于备份，应定期备份两个 MySQL 并测试恢复。

如果需要让其他内网机器访问，把 Gateway 端口从 `127.0.0.1` 改为服务器私网 IP，并在防火墙中只允许内网网段。

## MCP 客户端配置

MCP 客户端必须使用普通用户自己的 Token，不要使用 `ADMIN_TOKEN`：

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

生产环境如果通过内网 HTTPS 代理访问，将 URL 改为代理后的 `/mcp` 地址。

可用工具：

- `list_databases`：列出当前用户获授权的逻辑资源；
- `list_tables`：需要 `schema_read` 或更高权限；
- `describe_table`：需要 `schema_read` 或更高权限；
- `query_sql`：`SELECT` 需要 `query_read`，DML 需要 `query_write`。

示例参数：

```json
{
  "resource_key": "orders",
  "sql": "SELECT id, customer, amount FROM orders WHERE id = ?",
  "params": [{"type": "int", "value": "1"}],
  "max_rows": 50,
  "reason": "investigate order 1"
}
```

## 配置参考

| 变量 | 必需 | 说明 |
| --- | --- | --- |
| `CONTROL_DSN` | 是 | Control MySQL DSN；服务启动时自动迁移 |
| `ADMIN_TOKEN` | 是 | 至少 20 个字符；兼容旧版管理 API，必须作为高敏感 Secret 保存 |
| `TOKEN_PEPPER` | 是 | 至少 32 个字符；用于 Token 摘要、Session 和 Token 加密 |
| `DB_SECRET_<引用名>` | 按资源 | 目标库密码，引用名会转换为大写环境变量名 |
| `HTTP_ADDR` | 否 | 默认 `:8080` |
| `WEB_DIR` | 否 | 默认 `web/dist`；容器中为 `/app/web` |

不要随意更换 `TOKEN_PEPPER`：当前实现用它计算已有 Token/Session 摘要并解密已保存的 Token 配置，直接更换会使已有凭据失效。

## 安全边界与当前限制

已有防线包括：默认拒绝授权、读写账号隔离、SQL AST 守卫、结果/超时/写入限制、审计和字段脱敏。完整说明见 [docs/security.md](docs/security.md)。

当前版本仍有明确边界：

- 没有企业 SSO、MFA、细粒度管理员角色和完整 CSRF 防护；
- 环境变量 Secret 解析器不是完整 Secret Manager；
- 控制库审计表本身不是不可篡改账本；
- 不提供通用的列级 DLP 或行级安全策略；
- 单实例部署最简单，多副本需要额外处理迁移锁、MCP Session 和连接池预算。

因此，首版建议部署在内网或 VPN 中，并限制管理台的网络访问。

## 本地开发与验证

后端：

```bash
go test ./...
go vet ./...
go run ./cmd/gateway
```

前端：

```bash
cd web
npm ci
npm run dev
```

完整检查：

```bash
make check
```

验证记录见 [docs/verification.md](docs/verification.md)。

## 项目结构

```text
cmd/gateway/        服务入口、健康检查、静态站点
internal/api/       管理 REST API、Session 和 MCP 适配层
internal/authz/     动作层级、默认拒绝、约束合并
internal/sqlguard/  MySQL AST 安全检查
internal/query/     授权、执行、提交前复核、审计
internal/target/    目标连接池和 Secret 引用解析
internal/control/   控制面模型、迁移和存储
internal/masking/   字段脱敏规则和 SQL 重写
web/                React 管理台
deploy/             演示目标库初始化
docs/               安全、调研和验证记录
```

## 贡献与许可证

提交变更前请运行 `make check`。涉及授权、SQL 守卫、Token 或 Secret 的改动，应同时补充边界测试和安全说明。

项目使用 Apache License 2.0，第三方派生代码说明见 [NOTICE](NOTICE) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
