import { useState, type FormEvent } from 'react'
import { api } from '../api'
import { McpConfig } from './McpConfig'
import { TokenRow } from './TokenRow'
import type { APIToken, PrincipalAccess, User } from '../types'
import type { Tab } from '../components/AppShell'
import { EmptyState, MetricCard, Modal, ModalActions, SectionHeader, Status, Notice } from '../components/ui'
import { formatConstraint } from '../lib/format'

export function UserWorkspace({
  activeTab,
  me,
  access,
  tokens,
  refresh,
  fail,
  onTabChange,
}: {
  activeTab: Tab
  me: User
  access: PrincipalAccess[]
  tokens: APIToken[]
  refresh: () => Promise<void>
  fail: (message: string) => void
  onTabChange: (tab: Tab) => void
}) {
  if (activeTab === 'my-access') return <MyAccess access={access} />
  if (activeTab === 'my-tokens') return <MyTokens tokens={tokens} refresh={refresh} fail={fail} />
  if (activeTab === 'security') return <Security />
  return <UserOverview me={me} access={access} tokens={tokens} onTabChange={onTabChange} />
}

function UserOverview({
  me,
  access,
  tokens,
  onTabChange,
}: {
  me: User
  access: PrincipalAccess[]
  tokens: APIToken[]
  onTabChange: (tab: Tab) => void
}) {
  const activeTokens = tokens.filter((token) => !token.revoked_at && (!token.expires_at || new Date(token.expires_at).getTime() >= Date.now())).length
  return (
    <div className="pageStack">
      <section className="heroPanel userHero">
        <div className="heroCopy">
          <span className="kicker">我的工作区</span>
          <h2>你好，{me.display_name || me.username}</h2>
          <p>这里展示管理员分配给你的数据库资源，以及你自己创建的 MCP 访问凭证。</p>
        </div>
      </section>
      <section className="metricsGrid">
        <MetricCard label="可访问资源" value={access.length} detail="由管理员配置" />
        <MetricCard label="有效授权" value={access.reduce((count, item) => count + item.grants.length, 0)} detail="只读查看" />
        <MetricCard label="有效 Token" value={activeTokens} detail="仅自己可管理" />
        <MetricCard label="Token 总数" value={tokens.length} detail="含已撤销和过期" />
      </section>
      <section className="panel quickPanel">
        <SectionHeader kicker="下一步" title="管理你的访问边界" detail="查看资源权限，或者为 MCP 客户端创建一枚独立令牌。" />
        <div className="quickLinks quickLinksTwo">
          <button className="quickLink" type="button" onClick={() => onTabChange('my-access')}><span className="quickLinkIcon" aria-hidden="true">▦</span><span><strong>我的资源</strong><small>查看可用数据库、动作和约束</small></span><b aria-hidden="true">↗</b></button>
          <button className="quickLink" type="button" onClick={() => onTabChange('my-tokens')}><span className="quickLinkIcon" aria-hidden="true">◇</span><span><strong>Access Token</strong><small>创建、复制或撤销 MCP 令牌</small></span><b aria-hidden="true">↗</b></button>
        </div>
      </section>
    </div>
  )
}

function MyAccess({ access }: { access: PrincipalAccess[] }) {
  return (
    <section className="panel">
      <SectionHeader kicker="只读视图" title="我的资源" detail="资源和权限由管理员维护，你只能查看当前生效的授权。" action={<span className="countBadge">{access.length} 个资源</span>} />
      {access.length === 0 ? <EmptyState title="当前没有可访问的数据库资源" detail="请联系管理员为你的账号配置授权。" /> : <div className="accessGrid">{access.map((item) => (
        <article className="accessCard" key={item.resource_id}>
          <div className="accessCardTop"><div><Status value={item.enabled ? 'enabled' : 'disabled'} /><h3>{item.display_name || item.resource_key}</h3><code>{item.resource_key}</code></div><span className="dbBadge" aria-hidden="true">DB</span></div>
          <p className="accessDatabase">{item.database_name}</p>
          <div className="grantList">{item.grants.map((grant) => <div className="grantLine" key={grant.id}><span className={`actionTag action-${grant.action}`}>{grant.action}</span><small>{formatConstraint(grant.row_limit, grant.statement_timeout_ms, grant.require_reason)}</small></div>)}</div>
        </article>
      ))}</div>}
    </section>
  )
}

function MyTokens({ tokens, refresh, fail }: { tokens: APIToken[]; refresh: () => Promise<void>; fail: (message: string) => void }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [plain, setPlain] = useState('')
  const [copied, setCopied] = useState(false)
  const [deletedIds, setDeletedIds] = useState<string[]>([])
  const [deletingId, setDeletingId] = useState('')
  const displayedTokens = tokens.filter(t => !deletedIds.includes(t.id))
  function showConfig() {
    document.getElementById('mcp-config')?.scrollIntoView({ behavior: 'instant', block: 'start' })
  }

  async function create(event: FormEvent) {
    event.preventDefault()
    try {
      const result = await api<{ id: string; token: string }>('/api/v1/me/tokens', { method: 'POST', body: JSON.stringify({ name }) })
      setPlain(result.token)
      setCopied(false)
      setOpen(false)
      setName('')
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  async function deleteToken(id: string) {
    if (!window.confirm('确定永久删除这枚 Token 吗？删除后将从列表移除，使用它的 MCP 客户端将无法继续访问，且无法恢复。')) return
    if (deletingId) return
    setDeletingId(id)
    try {
      await api(`/api/v1/me/tokens/${id}`, { method: 'DELETE' })
      setDeletedIds(ids => [...ids, id])
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    } finally { setDeletingId('') }
  }

  async function copyToken() {
    try {
      await navigator.clipboard.writeText(plain)
      setCopied(true)
    } catch {
      fail('浏览器不允许自动复制，请手动复制完整 Token。')
    }
  }

  return (
    <div className="pageStack">
      <section className="panel">
        <SectionHeader kicker="MCP 凭证" title="Access Token" detail="新建默认永久有效。点击眼睛图标查看凭证。" action={<button className="button buttonPrimary" type="button" onClick={() => setOpen(true)}>＋ 创建 Token</button>} />
        {displayedTokens.length === 0 ? <EmptyState title="还没有创建 Access Token" detail="为本地 MCP 客户端创建一枚独立令牌。" action={<button className="button buttonSecondary" type="button" onClick={() => setOpen(true)}>创建第一枚 Token</button>} /> : <div className="tokenList">{displayedTokens.map(token => <TokenRow key={token.id} item={token} deleting={deletingId === token.id} busy={Boolean(deletingId)} onDelete={() => void deleteToken(token.id)} />)}</div>}

      </section>

      {open && <Modal title="创建 Access Token" onClose={() => setOpen(false)}><form className="form" onSubmit={create}><label htmlFor="token-name">名称<input id="token-name" name="token_name" value={name} onChange={(event) => setName(event.target.value)} autoComplete="off" autoFocus placeholder="本地 MCP 客户端" /></label><p className="formHint">新建 Token 默认永久有效，删除后失效。</p><ModalActions onCancel={() => setOpen(false)} submit="生成 Token" /></form></Modal>}
      {plain && <Modal title="Token 已生成" onClose={() => setPlain('')}><div className="secretBox"><strong>完整 Access Token</strong><code>{plain}</code><button className="button buttonSecondary" type="button" onClick={() => void copyToken()}>{copied ? '已复制' : '复制 Token'}</button><button className="button buttonPrimary" type="button" onClick={() => { setPlain(''); showConfig() }}>查看下方 MCP 配置</button></div></Modal>}
      <section className="panel" id="mcp-config">
        <SectionHeader kicker="" title="Claude Code / Codex MCP 配置" detail="配置仅引用环境变量，不显示或复制真实 Token。" />
        <McpConfig />
      </section>
    </div>
  )
}

function Security() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setMessage('')
    if (next !== confirm) {
      setError('两次输入的新密码不一致')
      return
    }
    try {
      await api('/api/v1/auth/password', { method: 'POST', body: JSON.stringify({ current_password: current, new_password: next }) })
      setCurrent('')
      setNext('')
      setConfirm('')
      setMessage('密码已更新，其他网页登录会话已退出。')
    } catch (requestError) {
      setError((requestError as Error).message)
    }
  }

  return (
    <section className="panel securityPanel">
      <SectionHeader kicker="账户安全" title="修改密码" detail="新密码至少 8 个字符且不超过 72 个字节。修改后其他网页登录会话会失效。" />
      <form className="form narrowForm" onSubmit={submit}>
        <label htmlFor="current-password">当前密码<input id="current-password" name="current_password" type="password" value={current} onChange={(event) => setCurrent(event.target.value)} autoComplete="current-password" required /></label>
        <label htmlFor="new-password">新密码<input id="new-password" name="new_password" type="password" value={next} onChange={(event) => setNext(event.target.value)} autoComplete="new-password" minLength={8} required /></label>
        <label htmlFor="confirm-password">确认新密码<input id="confirm-password" name="confirm_password" type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} autoComplete="new-password" minLength={8} required /></label>
        {error && <Notice kind="error">{error}</Notice>}
        {message && <Notice kind="success">{message}</Notice>}
        <button className="button buttonPrimary" type="submit">更新密码</button>
      </form>
    </section>
  )
}
