# DB Access Gateway

一个面向 AI/MCP 客户端的 MySQL 访问网关：管理员登记账号和逻辑数据库资源，再按 `账号 × 资源 × 动作` 授权。服务端持有数据库凭据，客户端只拿到个人访问令牌，不会接触 DSN 或密码。

本实现先对 [GoNavi](https://github.com/YogeLiu/GoNavi) 与 [db-mcp-gateway](https://github.com/developerz-ai/db-mcp-gateway) 做了代码级调研，再组合两者的长处。具体采用与取舍见 [docs/research.md](docs/research.md)。

数据库连接层不是重新造轮子：`internal/target/database.go`、`registry.go` 直接移植并裁剪自 GoNavi 的 MySQL adapter、SQL pool 和 App 连接缓存。所有派生文件均标记来源与修改，详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## 已实现

- Go 1.25 服务，使用官方 `modelcontextprotocol/go-sdk` 暴露 Streamable HTTP MCP `/mcp`
- React + TypeScript 控制台：管理员/用户账号密码登录、角色工作区、用户自助 Access Token、数据库资源、授权矩阵、授权预演和审计查询
- 管理员首次账号为 `admin / admin_123`，密码使用 bcrypt 哈希保存，管理员和用户都可以修改自己的密码（至少 8 个字符，最多 72 个字节）
- 管理员注册和管理用户；用户只读查看自己的资源/授权，并自行创建、查看和撤销 MCP Access Token
- 三层动作：`query_write ⊇ query_read ⊇ schema_read`，没有匹配授权时默认拒绝
- MySQL 单语句 AST 守卫：只允许 `SELECT` 和获得写权限后的 `INSERT/UPDATE/DELETE`
- 永久拒绝 DDL、多语句、跨库、系统库、锁定读取、危险函数、无 `WHERE` 的更新/删除
- 读取与写入使用不同数据库账号；目标账号本身构成第二道权限边界
- 行数、返回体、超时、写入影响行数限制；多条匹配授权按最严格值合并
- 写入在事务提交前重新读取账号、资源版本和授权，授权已撤销则回滚
- 审计记录保存 SQL 文本和 SHA-256 指纹，不保存参数值、结果或数据库密码；同一请求只展示最终状态
- GoNavi 派生的 MySQL adapter、连接池、30 秒健康检查、坏连接剔除和 `singleflight` 并发建连合并
- 网关扩展的资源版本、读写池隔离、更新期间在途旧连接失效和请求取消传递

## 权限语义

| 授权 | schema_read | query_read | query_write |
|---|---:|---:|---:|
| schema_read | 允许 | 拒绝 | 拒绝 |
| query_read | 允许 | 允许 | 拒绝 |
| query_write | 允许 | 允许 | 允许 |

因此题目中的矩阵会得到：

| 请求 | 结果 |
|---|---|
| `user_a → database_a → SELECT` | 允许 |
| `user_a → database_c → SELECT` | 拒绝（没有该资源授权） |
| `user_c → database_c → INSERT/UPDATE/DELETE` | 允许 |
| 只有 `query_read` 的用户执行 `INSERT` | 拒绝 |

## 一键启动演示

需要 Docker Compose。以下命令会启动控制库、含三个演示数据库的目标 MySQL，以及网关：

```bash
docker compose up --build -d
```

打开 `http://localhost:8080`，使用管理员账号登录：

```text
账号：admin
密码：admin_123
```

首次登录后请在“安全设置”修改管理员密码。管理员可以在“用户管理”中注册用户账号、设置初始密码、停用账号或重置密码；普通用户登录后只能查看自己的资源和授权，并在“Access Token”中创建 MCP 令牌。

`ADMIN_TOKEN` 仍保留为脚本和旧版 API 客户端的兼容认证入口，不用于控制台页面登录。

也可以安装 `jq` 后生成题目中的完整授权矩阵和两个个人令牌：

```bash
./scripts/bootstrap-demo.sh
```

脚本会输出 `USER_A_TOKEN` 和 `USER_C_TOKEN`，每个令牌仅在创建时出现一次。脚本用于全新数据卷；重复执行前请在管理台删除/更名资源，或用 `docker compose down -v` 清空演示数据。

## 生产部署（单实例、内网、Docker Compose）

当前仓库的 `docker-compose.yml` 是演示配置：包含演示目标库、演示初始化 SQL 和开发密码。生产环境不要直接使用它，也不要挂载 `deploy/target-init.sql`。

在“单个网关实例、控制库和目标库都运行在 Compose、数据需要持久化、网关不对公网开放”的场景下，不需要修改 Go 或 React 业务代码。建议在仓库根目录另建 `docker-compose.prod.yml`，使用独立的生产环境变量文件：

```yaml
services:
  control-mysql:
    image: mysql:8.4
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${CONTROL_MYSQL_ROOT_PASSWORD:?missing}
      MYSQL_DATABASE: gateway
      MYSQL_USER: gateway
      MYSQL_PASSWORD: ${CONTROL_MYSQL_PASSWORD:?missing}
    volumes:
      - control-data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -uroot -p$$MYSQL_ROOT_PASSWORD --silent"]
      interval: 3s
      timeout: 3s
      retries: 30

  target-mysql:
    image: mysql:8.4
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${TARGET_MYSQL_ROOT_PASSWORD:?missing}
    volumes:
      - target-data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -uroot -p$$MYSQL_ROOT_PASSWORD --silent"]
      interval: 3s
      timeout: 3s
      retries: 30

  gateway:
    build: .
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      HTTP_ADDR: ":8080"
      WEB_DIR: /app/web
      CONTROL_DSN: "gateway:${CONTROL_MYSQL_PASSWORD}@tcp(control-mysql:3306)/gateway?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"
      ADMIN_TOKEN: ${ADMIN_TOKEN:?missing}
      TOKEN_PEPPER: ${TOKEN_PEPPER:?missing}
      DB_SECRET_PROD_READ: ${TARGET_READ_PASSWORD:?missing}
      DB_SECRET_PROD_WRITE: ${TARGET_WRITE_PASSWORD:?missing}
    depends_on:
      control-mysql:
        condition: service_healthy
      target-mysql:
        condition: service_healthy

volumes:
  control-data:
  target-data:
```

创建不纳入 Git 的 `.env.prod`，并限制文件权限：

```dotenv
CONTROL_MYSQL_ROOT_PASSWORD=<随机密码>
CONTROL_MYSQL_PASSWORD=<随机密码>
TARGET_MYSQL_ROOT_PASSWORD=<随机密码>
TARGET_READ_PASSWORD=<目标库只读账号密码>
TARGET_WRITE_PASSWORD=<目标库读写账号密码>
ADMIN_TOKEN=<至少 20 个字符的随机字符串>
TOKEN_PEPPER=<至少 32 个字符的随机字符串>
```

推荐使用 `openssl rand -hex 32` 生成随机值，并执行：

```bash
chmod 600 .env.prod
docker compose --env-file .env.prod -f docker-compose.prod.yml config
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build
```

验证网关：

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

`control-data` 和 `target-data` 是持久化命名卷。普通的 `docker compose down` 不会删除数据，但生产环境不要执行 `docker compose down -v`。持久化不等于备份，仍应定期执行 MySQL 备份并测试恢复。

网关访问目标库时使用 Docker 服务名 `target-mysql`。在管理台创建资源时填写：

- Host：`target-mysql`
- Port：`3306`
- `read_secret_ref`：`prod_read`，对应 `DB_SECRET_PROD_READ`
- `write_secret_ref`：`prod_write`，对应 `DB_SECRET_PROD_WRITE`
- 目标库已配置 TLS 时使用 `tls_mode=required`

`target-mysql` 只负责启动 MySQL，不会自动创建业务数据库和最小权限账号。启用资源前，请按实际业务库名称创建数据库及账号，例如：

```sql
CREATE DATABASE appdb CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE USER 'gateway_read'@'%' IDENTIFIED BY '<只读密码>';
GRANT SELECT, SHOW VIEW ON appdb.* TO 'gateway_read'@'%';

CREATE USER 'gateway_write'@'%' IDENTIFIED BY '<读写密码>';
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON appdb.* TO 'gateway_write'@'%';
```

实际部署时应将 `'%'` 收紧为网关所在的受控网络范围，并将账号密码分别放入 `TARGET_READ_PASSWORD`、`TARGET_WRITE_PASSWORD`。如果有多个资源或多组账号，在 Compose 环境中增加对应的 `DB_SECRET_<引用名>` 变量。

控制库会在网关首次启动时自动迁移。首次登录仍使用 `admin / admin_123`，必须在开放内网访问前修改管理员密码。管理台和 MCP 只绑定到 `127.0.0.1`；如需从内网其他机器访问，应绑定服务器私网 IP，并在防火墙中只允许内网网段。

## MCP 客户端配置

将脚本生成的个人令牌放进 `Authorization`，不要使用管理员令牌连接 MCP：

```json
{
  "mcpServers": {
    "database-gateway": {
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer dbag_REPLACE_WITH_USER_TOKEN"
      }
    }
  }
}
```

工具：

- `list_databases`：仅返回当前账号有授权的逻辑资源，不返回主机或凭据
- `list_tables`：需要 `schema_read` 或更高权限
- `describe_table`：需要 `schema_read` 或更高权限
- `query_sql`：SELECT 需要 `query_read`；INSERT/UPDATE/DELETE 需要 `query_write`

参数使用显式类型，避免客户端把任意 JSON 对象直接下传给驱动：

```json
{
  "resource_key": "database_a",
  "sql": "SELECT id, customer, amount FROM orders WHERE id = ?",
  "params": [{"type": "int", "value": "1"}],
  "max_rows": 50,
  "reason": "investigate order 1"
}
```

## 本地开发

```bash
# 后端
go test ./...
go vet ./...
go run ./cmd/gateway

# 前端（另一个终端）
cd web
npm ci
npm run dev
```

后端至少需要：

```text
CONTROL_DSN
ADMIN_TOKEN          # 至少 20 字符
TOKEN_PEPPER         # 至少 32 字符
DB_SECRET_<引用名>   # 例如 demo_read -> DB_SECRET_DEMO_READ
```

完整样例见 [.env.example](.env.example)。前端生产构建由 Go 服务同源提供，因此没有宽松 CORS 配置。

## 目录

```text
cmd/gateway/        服务入口、健康检查、静态站点
internal/api/       管理 REST API 与 MCP 适配层
internal/authz/     动作层级、默认拒绝、约束合并
internal/sqlguard/  MySQL AST 安全检查
internal/query/     授权、执行、提交前复核、审计
internal/target/    目标连接池和 Secret 引用解析
internal/control/   控制面模型、迁移和存储
web/                React 管理台
deploy/             演示目标库初始化
docs/               调研和安全说明
```

## 上线前必须调整

- 替换 Compose 内所有密码、`ADMIN_TOKEN` 与 `TOKEN_PEPPER`；通过 Secret Manager 注入，不写入镜像或配置仓库。
- MCP 和管理台放在 HTTPS 反向代理后；目标 MySQL 使用 `required` TLS，并配置最小权限读写账号。
- 管理接口增加企业 SSO/MFA、CSRF/来源控制和网络访问限制；当前管理员 Bearer Token 适用于内网首版。
- 审计表导出到不可变存储并配置保留策略；当前 MySQL 表本身不是防篡改账本。
- 当前首版聚焦资源/动作 RBAC，不包含列级脱敏或行级策略；若数据包含敏感字段，先通过受控视图或目标库权限隔离。

更完整的威胁边界见 [docs/security.md](docs/security.md)。

## License 与代码来源

项目按 Apache License 2.0 分发。GoNavi 派生代码的原始提交、文件映射和修改内容见 [NOTICE](NOTICE) 与 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
