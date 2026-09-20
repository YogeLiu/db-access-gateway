import { useState } from 'react'

export function McpConfig() {
  const [endpoint, setEndpoint] = useState(window.location.origin + '/mcp')
  return <div className="form">
    <label>网关地址<input type="url" value={endpoint} onChange={e => setEndpoint(e.target.value)} /></label>
    <details className="configHelp"><summary>如何设置 DB_ACCESS_TOKEN？</summary><p>将环境变量设为一枚有效 Token，不带 Bearer 前缀。macOS / Linux：<code>export DB_ACCESS_TOKEN='&lt;你的 Access Token&gt;'</code>，随后在同一终端启动 claude 或 codex。重启终端后需重新设置，或通过环境管理工具注入。配置文件只引用变量，不包含真实 Token。</p></details>
    <div className="mcpConfigGrid">
      <ConfigBlock title="Claude Code" hint="合并到项目根目录的 .mcp.json" config={JSON.stringify({ mcpServers: { database_gateway: { type: 'http', url: endpoint, headers: { Authorization: 'Bearer ${DB_ACCESS_TOKEN}' } } } }, null, 2)} />
      <ConfigBlock title="Codex" hint="合并到 ~/.codex/config.toml" config={'[mcp_servers.database_gateway]\nurl = ' + JSON.stringify(endpoint) + '\nbearer_token_env_var = "DB_ACCESS_TOKEN"\n'} />
    </div>
  </div>
}

function ConfigBlock({ title, hint, config }: { title: string; hint: string; config: string }) {
  const [message, setMessage] = useState('')
  async function copy() {
    try { await navigator.clipboard.writeText(config); setMessage('完整配置已复制') }
    catch { setMessage('自动复制不可用，请选中配置手动复制。') }
  }
  return <div className="form">
    <div className="configBlockHeader"><h3>{title}</h3><button className="button buttonSecondary" type="button" onClick={() => void copy()} aria-label={`复制 ${title} 配置`}>复制配置</button></div>
    <p className="formHint">{hint}，保留已有配置。网关地址需能从客户端访问。</p>
    <textarea className="configCode" aria-label={`${title} MCP 配置`} readOnly value={config} rows={13} spellCheck={false} />
    {message && <p role="status">{message}</p>}
  </div>
}
