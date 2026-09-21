# 验证记录

验证日期：2026-09-21。

## 已执行

| 检查 | 结果 |
|---|---|
| `go test -race ./...` | 通过；除权限/SQL 语料外，覆盖连接缓存的 32 并发冷启动合并、资源更新时在途连接拒绝和单资源凭据解析 |
| `go vet ./...` | 通过 |
| `CGO_ENABLED=0 go build ... ./cmd/gateway` | 通过；生成静态、strip 后的 linux/amd64 二进制 |
| `npm run build` | 通过；TypeScript project build 与 Vite production build 成功 |
| `npm audit --omit=dev` | 0 vulnerabilities |
| `govulncheck ./...` | 升级两个间接依赖后重跑，No vulnerabilities found |
| Compose / CI YAML 解析 | 通过 |
| 派生代码归属 | `LICENSE`、`NOTICE`、`THIRD_PARTY_NOTICES.md` 与源文件修改标记齐全 |

## 当前环境未执行

当前工作容器没有 Docker，且权限策略禁止安装 MySQL/MariaDB 守护进程，所以没有在此环境启动 Compose 做真实网络集成测试。当前 Compose 只管理 Gateway 的 Control MySQL；业务数据所在的目标 MySQL 需要使用已有实例，应在有 Docker 和目标数据库访问权限的环境执行：

```bash
docker compose \
  --env-file .env.prod \
  -f docker-compose.prod.yml \
  up --build -d
```

然后在管理台注册一个已有目标 MySQL 资源，配置一套连接账号和 Secret 引用，测试连接，并在“授权判定测试”中核对 Gateway 的读写授权矩阵。这一项是发布前必须完成的验证门禁。
