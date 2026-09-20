import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const server = await createServer({ server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' })
globalThis.window = { location: { origin: 'http://localhost:8080' } }
try {
  const { McpConfig } = await server.ssrLoadModule('/src/features/McpConfig.tsx')
  const { UserWorkspace } = await server.ssrLoadModule('/src/features/user.tsx')
  const { TokenRow } = await server.ssrLoadModule('/src/features/TokenRow.tsx')
  const token = 'dbag_synthetic_complete_token'
  const config = renderToStaticMarkup(createElement(McpConfig, { token }))
  assert.ok(!config.includes(token), 'config must never embed the real token')
  assert.ok(config.includes('Bearer ${DB_ACCESS_TOKEN}'))
  assert.ok(config.includes('bearer_token_env_var = &quot;DB_ACCESS_TOKEN&quot;'))
  assert.ok(config.includes('复制 Claude Code 配置'))
  assert.ok(config.includes('复制 Codex 配置'))
  assert.ok(!config.includes('role="dialog"'))
  assert.ok(!config.includes('YOUR_ACCESS_TOKEN'))
  const workspace = renderToStaticMarkup(createElement(UserWorkspace, {
    activeTab: 'my-tokens', me: {}, access: [], tokens: [], refresh: async () => {}, fail: () => {}, onTabChange: () => {},
  }))
  assert.ok(workspace.includes('id="mcp-config"'), 'configuration section must remain visible even without tokens')
  const display = renderToStaticMarkup(createElement(TokenRow, { item: { id: 'test', name: 'Test', prefix: 'dbag_prefix', config_available: true, created_at: '2026-09-20T00:00:00Z' }, deleting: false, busy: false, onDelete: () => {} }))
  assert.ok(display.includes('role="switch"'))
  assert.ok(display.includes('aria-checked="false"'))
  assert.ok(display.includes('••••••••'))
  assert.ok(display.includes('<svg'))
  assert.ok(!display.includes('切换明文'))
  const actions = display.match(/class="tokenActions">([\s\S]*?)<\/div>/)?.[1] || ''
  assert.ok(actions.includes('>有效</span>'), 'status must be in the action row')
  assert.ok(actions.includes('role="switch"'), 'eye must share the status action row')
  assert.ok(actions.includes('>删除</button>'))
  assert.ok(display.indexOf('class="tokenActions"') < display.indexOf('class="tokenValue"'), 'actions must precede the token value')
  const expired = renderToStaticMarkup(createElement(TokenRow, { item: { id: 'test', name: 'Test', prefix: 'dbag_prefix', config_available: true, created_at: '2020-01-01', expires_at: '2020-02-01' }, deleting: false, busy: false, onDelete: () => {} }))
  assert.ok(!expired.includes('role="switch"'), 'expired tokens must not expose a visibility control')
  const populated = renderToStaticMarkup(createElement(UserWorkspace, {
    activeTab: 'my-tokens', me: {}, access: [], tokens: [{ id: 't', name: 'test', prefix: 'dbag_prefix', config_available: true, created_at: '2026-09-20T00:00:00Z' }], refresh: async () => {}, fail: () => {}, onTabChange: () => {},
  }))
  assert.ok(populated.includes('>删除</button>'))
  assert.ok(!populated.includes('>撤销</button>'))
  assert.ok(populated.includes('永不过期'))
  console.log('PASS: variable-only MCP configs, copy buttons, eye icon, deletion action and permanent token display')
} finally {
  await server.close()
  delete globalThis.window
}
