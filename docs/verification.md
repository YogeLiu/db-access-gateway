# 验证记录

验证日期：2026-09-19。

## 已执行

| 检查 | 结果 |
|---|---|
| `go test -race ./...` | 通过；除权限/SQL 语料外，覆盖 GoNavi 派生连接缓存的 32 并发冷启动合并、资源更新时在途连接拒绝、读写缓存键隔离 |
| `go vet ./...` | 通过 |
| `CGO_ENABLED=0 go build ... ./cmd/gateway` | 通过；生成静态、strip 后的 linux/amd64 二进制 |
| `npm run build` | 通过；TypeScript project build 与 Vite production build 成功 |
| `npm audit --omit=dev` | 0 vulnerabilities |
| `govulncheck ./...` | 升级两个间接依赖后重跑，No vulnerabilities found |
| Compose / CI YAML 解析 | 通过 |
| `bash -n scripts/bootstrap-demo.sh` | 通过 |
| 派生代码归属 | `LICENSE`、`NOTICE`、`THIRD_PARTY_NOTICES.md` 与源文件修改标记齐全 |

## 当前环境未执行

当前工作容器没有 Docker，且权限策略禁止安装 MySQL/MariaDB 守护进程，所以没有在此环境启动 Compose 做真实网络集成测试。交付包含 MySQL 8.4 Compose、初始化 SQL、健康检查和演示引导脚本；应在有 Docker 的环境执行：

```bash
docker compose up --build -d
./scripts/bootstrap-demo.sh
```

然后在管理台“授权判定测试”中核对题目矩阵，并分别使用 `USER_A_TOKEN` / `USER_C_TOKEN` 调用 MCP `query_sql`。这一项是发布前必须完成的验证门禁。
