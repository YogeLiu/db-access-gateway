import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { api } from '../api'
import { Masking } from './Masking'
import type { Audit, AuditPage, Grant, Resource, User } from '../types'
import type { Tab } from '../components/AppShell'
import {
  EmptyState,
  MetricCard,
  Modal,
  ModalActions,
  Pagination,
  SearchField,
  SectionHeader,
  Status,
  TableEmpty,
} from '../components/ui'
import { formatConstraint, formatDate } from '../lib/format'

const localPageSize = 8

type ResourceDraft = {
  resource_key: string
  display_name: string
  host: string
  port: number
  database_name: string
  username: string
  secret_ref: string
  tls_mode: string
  max_rows: number
  max_write_rows: number
  statement_timeout_ms: number
  enabled: boolean
  version: number
}

type GrantDraft = {
  principal_id: string
  resource_id: string
  action: Grant['action']
  require_reason: boolean
  row_limit: number
  statement_timeout_ms: number
}

const newResourceDraft = (): ResourceDraft => ({
  resource_key: '',
  display_name: '',
  host: '',
  port: 3306,
  database_name: '',
  username: '',
  secret_ref: '',
  tls_mode: 'required',
  max_rows: 1000,
  max_write_rows: 100,
  statement_timeout_ms: 10000,
  enabled: false,
  version: 0,
})

const newGrantDraft = (): GrantDraft => ({
  principal_id: '',
  resource_id: '',
  action: 'query_read',
  require_reason: false,
  row_limit: 200,
  statement_timeout_ms: 5000,
})

function useClientPagination<T>(
  items: T[],
  query: string,
  matches: (item: T, normalizedQuery: string) => boolean,
) {
  const [page, setPage] = useState(1)
  const filtered = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    return normalizedQuery ? items.filter((item) => matches(item, normalizedQuery)) : items
  }, [items, matches, query])
  const totalPages = Math.max(Math.ceil(filtered.length / localPageSize), 1)
  const rows = filtered.slice((page - 1) * localPageSize, page * localPageSize)

  useEffect(() => setPage(1), [query])
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  return { page, setPage, rows, total: filtered.length, totalPages }
}

function ListToolbar({
  query,
  onQueryChange,
  placeholder,
  count,
}: {
  query: string
  onQueryChange: (value: string) => void
  placeholder: string
  count: number
}) {
  return (
    <div className="listToolbar">
      <SearchField value={query} onChange={onQueryChange} placeholder={placeholder} />
      <span className="resultCount">{count} 项</span>
    </div>
  )
}

export function AdminWorkspace({
  users,
  resources,
  grants,
  audits,
  auditMeta,
  refresh,
  fail,
  notify,
  activeTab,
  onTabChange,
  onAuditPageChange,
}: {
  users: User[]
  resources: Resource[]
  grants: Grant[]
  audits: Audit[]
  auditMeta: AuditPage
  refresh: () => Promise<void>
  fail: (message: string) => void
  notify: (message: string) => void
  activeTab: Tab
  onTabChange: (tab: Tab) => void
  onAuditPageChange: (page: number) => void
}) {
  if (activeTab === 'masking') return <Masking resources={resources} />
  if (activeTab === 'users') return <Users users={users} refresh={refresh} fail={fail} />
  if (activeTab === 'resources') return <Resources resources={resources} refresh={refresh} fail={fail} notify={notify} />
  if (activeTab === 'grants') return <Grants users={users} resources={resources} grants={grants} refresh={refresh} fail={fail} />
  if (activeTab === 'audits') return <Audits audits={audits} meta={auditMeta} onPageChange={onAuditPageChange} />
  return <Overview users={users} resources={resources} grants={grants} audits={audits} onTabChange={onTabChange} />
}

function Overview({
  users,
  resources,
  grants,
  audits,
  onTabChange,
}: {
  users: User[]
  resources: Resource[]
  grants: Grant[]
  audits: Audit[]
  onTabChange: (tab: Tab) => void
}) {
  const denied = audits.filter((audit) => ['DENIED', 'REJECTED', 'FAILED'].includes(audit.outcome)).length
  const quickLinks: { tab: Tab; title: string; detail: string; icon: string }[] = [
    { tab: 'users', title: '管理用户账号', detail: '注册成员、停用账号或重置密码', icon: '◎' },
    { tab: 'resources', title: '登记数据库资源', detail: '配置目标、TLS 和查询限制', icon: '▦' },
    { tab: 'grants', title: '配置授权策略', detail: '按账号和资源授予最小权限', icon: '◇' },
  ]

  return (
    <div className="pageStack">
      <section className="metricsGrid">
        <MetricCard label="活跃账号" value={users.filter((user) => user.status === 'active').length} detail={`${users.length} 个已登记账号`} />
        <MetricCard label="可用资源" value={resources.filter((resource) => resource.enabled).length} detail={`${resources.length} 个数据库资源`} />
        <MetricCard label="生效授权" value={grants.length} detail="没有匹配时默认拒绝" />
        <MetricCard label="最近拒绝" value={denied} detail="当前审计页中的拒绝 / 失败" tone={denied ? 'danger' : 'default'} />
      </section>

      <section className="panel quickPanel">
        <SectionHeader kicker="快速入口" title="常用操作" detail="管理账号、数据库资源和访问权限。" />
        <div className="quickLinks">
          {quickLinks.map((item) => (
            <button className="quickLink" type="button" key={item.tab} onClick={() => onTabChange(item.tab)}>
              <span className="quickLinkIcon" aria-hidden="true">{item.icon}</span>
              <span><strong>{item.title}</strong><small>{item.detail}</small></span>
              <b aria-hidden="true">↗</b>
            </button>
          ))}
        </div>
      </section>

      <section className="panel">
        <SectionHeader
          kicker="最近请求"
          title="审计活动"
          detail="查看最近请求的 SQL 和执行结果。"
          action={<button className="button buttonSecondary" type="button" onClick={() => onTabChange('audits')}>查看全部</button>}
        />
        {audits.length ? <AuditTable audits={audits.slice(0, 6)} /> : <EmptyState title="暂无审计记录" detail="当 MCP 请求产生后，最终状态会显示在这里。" />}
      </section>
    </div>
  )
}

function Users({ users, refresh, fail }: { users: User[]; refresh: () => Promise<void>; fail: (message: string) => void }) {
  const [query, setQuery] = useState('')
  const [modal, setModal] = useState<'create' | 'reset' | null>(null)
  const [selected, setSelected] = useState<User | null>(null)
  const [form, setForm] = useState({ username: '', display_name: '', password: '' })
  const [resetPassword, setResetPassword] = useState('')
  const paged = useClientPagination(users, query, (user, normalizedQuery) =>
    [user.username, user.display_name, user.role, user.status].some((value) => value.toLowerCase().includes(normalizedQuery)),
  )

  async function create(event: FormEvent) {
    event.preventDefault()
    try {
      await api('/api/v1/admin/users', { method: 'POST', body: JSON.stringify(form) })
      setModal(null)
      setForm({ username: '', display_name: '', password: '' })
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  async function reset(event: FormEvent) {
    event.preventDefault()
    if (!selected) return
    try {
      await api(`/api/v1/admin/users/${selected.id}`, { method: 'PATCH', body: JSON.stringify({ password: resetPassword }) })
      setModal(null)
      setResetPassword('')
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  async function toggle(user: User) {
    if (user.status === 'active' && !window.confirm(`确定停用账号 ${user.username} 吗？该账号将无法继续建立访问会话。`)) return
    try {
      await api(`/api/v1/admin/users/${user.id}`, { method: 'PATCH', body: JSON.stringify({ status: user.status === 'active' ? 'disabled' : 'active' }) })
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  return (
    <div className="pageStack">
      <section className="panel">
        <SectionHeader
          kicker="身份目录"
          title="用户账号"
          detail="管理员负责账号生命周期；用户自行管理自己的 Access Token。"
          action={<button className="button buttonPrimary" type="button" onClick={() => setModal('create')}>＋ 注册用户</button>}
        />
        <ListToolbar query={query} onQueryChange={setQuery} placeholder="搜索账号、显示名称或状态" count={paged.total} />
        <div className="tableScroll">
          <table>
            <caption className="visuallyHidden">用户账号列表</caption>
            <thead><tr><th>账号</th><th>角色</th><th>密码</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead>
            <tbody>
              {paged.rows.length ? paged.rows.map((user) => (
                <tr key={user.id}>
                  <td><strong>{user.username}</strong><small>{user.display_name || '未设置显示名称'}</small></td>
                  <td><span className="tag">{user.role === 'admin' ? '管理员' : '用户'}</span></td>
                  <td>{user.password_set ? <span className="status statusGood"><i aria-hidden="true" />已设置</span> : <span className="status statusBad"><i aria-hidden="true" />待设置</span>}</td>
                  <td><Status value={user.status} /></td>
                  <td>{formatDate(user.created_at)}</td>
                  <td><div className="rowActions"><button className="tableButton" type="button" onClick={() => { setSelected(user); setResetPassword(''); setModal('reset') }}>重置密码</button><button className="tableButton" type="button" onClick={() => void toggle(user)}>{user.status === 'active' ? '停用' : '启用'}</button></div></td>
                </tr>
              )) : <TableEmpty colSpan={6} title={query ? '没有匹配的账号' : '还没有用户账号'} />}
            </tbody>
          </table>
        </div>
        <Pagination page={paged.page} totalPages={paged.totalPages} total={paged.total} onChange={paged.setPage} />
      </section>

      {modal === 'create' && (
        <Modal title="注册用户" onClose={() => setModal(null)}>
          <form className="form" onSubmit={create}>
            <label htmlFor="create-username">账号<input id="create-username" name="username" value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} autoComplete="off" autoFocus required placeholder="yuanchuangliu" /></label>
            <label htmlFor="create-display-name">显示名称<input id="create-display-name" name="display_name" value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} autoComplete="off" placeholder="业务开发" /></label>
            <label htmlFor="create-password">初始密码<input id="create-password" name="new_password" type="password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} autoComplete="new-password" minLength={8} required placeholder="至少 8 个字符" /></label>
            <ModalActions onCancel={() => setModal(null)} submit="创建账号" />
          </form>
        </Modal>
      )}
      {modal === 'reset' && selected && (
        <Modal title={`重置 ${selected.username} 的密码`} onClose={() => setModal(null)}>
          <form className="form" onSubmit={reset}>
            <p className="formHint">重置后该用户的现有网页登录会话将失效。</p>
            <label htmlFor="reset-password">新密码<input id="reset-password" name="new_password" type="password" value={resetPassword} onChange={(event) => setResetPassword(event.target.value)} autoComplete="new-password" autoFocus minLength={8} required /></label>
            <ModalActions onCancel={() => setModal(null)} submit="确认重置" />
          </form>
        </Modal>
      )}
    </div>
  )
}

function Resources({
  resources,
  refresh,
  fail,
  notify,
}: {
  resources: Resource[]
  refresh: () => Promise<void>
  fail: (message: string) => void
  notify: (message: string) => void
}) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [draft, setDraft] = useState<ResourceDraft>(newResourceDraft())
  const paged = useClientPagination(resources, query, (resource, normalizedQuery) =>
    [resource.resource_key, resource.display_name, resource.host, resource.database_name, resource.username].some((value) => value.toLowerCase().includes(normalizedQuery)),
  )

  async function save(event: FormEvent) {
    event.preventDefault()
    try {
      await api(editingId ? `/api/v1/admin/resources/${editingId}` : '/api/v1/admin/resources', { method: editingId ? 'PATCH' : 'POST', body: JSON.stringify(draft) })
      closeEditor()
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  async function test(resource: Resource) {
    try {
      await api(`/api/v1/admin/resources/${resource.id}/test`, { method: 'POST', body: '{}' })
      notify(`${resource.resource_key} 连接测试通过`)
    } catch (error) {
      fail((error as Error).message)
    }
  }

  function closeEditor() {
    setOpen(false)
    setEditingId(null)
    setDraft(newResourceDraft())
  }

  function edit(resource: Resource) {
    setEditingId(resource.id)
    setDraft({
      resource_key: resource.resource_key,
      display_name: resource.display_name,
      host: resource.host,
      port: resource.port,
      database_name: resource.database_name,
      username: resource.username,
      secret_ref: resource.secret_ref,
      tls_mode: resource.tls_mode,
      max_rows: resource.max_rows,
      max_write_rows: resource.max_write_rows,
      statement_timeout_ms: resource.statement_timeout_ms,
      enabled: resource.enabled,
      version: resource.version,
    })
    setOpen(true)
  }

  return (
    <div className="pageStack">
      <section className="panel">
        <SectionHeader
          kicker="数据源"
          title="数据库资源"
          detail="资源凭据只由网关持有；用户只能看到自己被授予的资源和动作。"
          action={<button className="button buttonPrimary" type="button" onClick={() => { setDraft(newResourceDraft()); setEditingId(null); setOpen(true) }}>＋ 添加资源</button>}
        />
        <ListToolbar query={query} onQueryChange={setQuery} placeholder="搜索资源键、主机或数据库名" count={paged.total} />
        <div className="tableScroll">
          <table>
            <caption className="visuallyHidden">数据库资源列表</caption>
		    <thead><tr><th>资源</th><th>目标</th><th>连接账号</th><th>限制</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {paged.rows.length ? paged.rows.map((resource) => (
                <tr key={resource.id}>
                  <td><strong>{resource.display_name || resource.resource_key}</strong><small><code>{resource.resource_key}</code></small></td>
                  <td><code>{resource.host}:{resource.port}</code><small>{resource.database_name}</small></td>
				  <td><strong>{resource.username}</strong><small>TLS：{resource.tls_mode}</small></td>
                  <td>{resource.max_rows} 行<small>{resource.statement_timeout_ms} ms</small></td>
                  <td><Status value={resource.enabled ? 'enabled' : 'disabled'} /></td>
                  <td><div className="rowActions"><button className="tableButton" type="button" onClick={() => edit(resource)}>编辑</button><button className="tableButton" type="button" onClick={() => void test(resource)}>测试连接</button></div></td>
                </tr>
              )) : <TableEmpty colSpan={6} title={query ? '没有匹配的资源' : '还没有登记数据库资源'} />}
            </tbody>
          </table>
        </div>
        <Pagination page={paged.page} totalPages={paged.totalPages} total={paged.total} onChange={paged.setPage} />
      </section>

      {open && (
        <Modal title={editingId ? '编辑数据库资源' : '登记数据库资源'} onClose={closeEditor} wide>
          <form className="form compactForm" onSubmit={save}>
            <div className="fieldGrid">
              <label htmlFor="resource-key">资源键<input id="resource-key" name="resource_key" value={draft.resource_key} disabled={Boolean(editingId)} onChange={(event) => setDraft({ ...draft, resource_key: event.target.value })} autoComplete="off" required placeholder="doc_ai" /></label>
              <label htmlFor="resource-display-name">显示名称<input id="resource-display-name" name="display_name" value={draft.display_name} onChange={(event) => setDraft({ ...draft, display_name: event.target.value })} autoComplete="off" placeholder="Document AI" /></label>
            </div>
            <div className="fieldGrid">
              <label htmlFor="resource-host">Host<input id="resource-host" name="host" value={draft.host} onChange={(event) => setDraft({ ...draft, host: event.target.value })} autoComplete="off" required placeholder="127.0.0.1" /></label>
              <label htmlFor="resource-port">Port<input id="resource-port" name="port" type="number" value={draft.port} onChange={(event) => setDraft({ ...draft, port: Number(event.target.value) })} required /></label>
            </div>
            <label htmlFor="resource-database">Database<input id="resource-database" name="database_name" value={draft.database_name} onChange={(event) => setDraft({ ...draft, database_name: event.target.value })} autoComplete="off" required placeholder="doc_ai" /></label>
            <div className="fieldGrid">
              <label htmlFor="resource-username">连接账号<input id="resource-username" name="username" value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} autoComplete="off" required /></label>
              <label htmlFor="resource-secret">Secret ref<input id="resource-secret" name="secret_ref" value={draft.secret_ref} onChange={(event) => setDraft({ ...draft, secret_ref: event.target.value })} autoComplete="off" required /></label>
            </div>
            <div className="fieldGrid fieldGridThree">
              <label htmlFor="resource-tls">TLS<select id="resource-tls" name="tls_mode" value={draft.tls_mode} onChange={(event) => setDraft({ ...draft, tls_mode: event.target.value })}><option value="required">required</option><option value="preferred">preferred</option><option value="skip_verify">skip_verify</option><option value="disabled">disabled（不启用）</option></select></label>
              <label htmlFor="resource-timeout">超时 ms<input id="resource-timeout" name="statement_timeout_ms" type="number" value={draft.statement_timeout_ms} onChange={(event) => setDraft({ ...draft, statement_timeout_ms: Number(event.target.value) })} /></label>
              <label htmlFor="resource-max-rows">读取上限<input id="resource-max-rows" name="max_rows" type="number" value={draft.max_rows} onChange={(event) => setDraft({ ...draft, max_rows: Number(event.target.value) })} /></label>
            </div>
            <label htmlFor="resource-max-write-rows">写入行上限<input id="resource-max-write-rows" name="max_write_rows" type="number" value={draft.max_write_rows} onChange={(event) => setDraft({ ...draft, max_write_rows: Number(event.target.value) })} /></label>
            <label className="checkField" htmlFor="resource-enabled"><input id="resource-enabled" type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} />立即启用资源</label>
            <ModalActions onCancel={closeEditor} submit={editingId ? '保存修改' : '创建资源'} />
          </form>
        </Modal>
      )}
    </div>
  )
}

function Grants({
  users,
  resources,
  grants,
  refresh,
  fail,
}: {
  users: User[]
  resources: Resource[]
  grants: Grant[]
  refresh: () => Promise<void>
  fail: (message: string) => void
}) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<GrantDraft>(newGrantDraft())
  const [test, setTest] = useState<{ principal_id: string; resource_id: string; action: Grant['action'] }>({ principal_id: '', resource_id: '', action: 'query_read' })
  const [decision, setDecision] = useState<boolean | null>(null)
  const paged = useClientPagination(grants, query, (grant, normalizedQuery) =>
    [grant.username, grant.resource_key, grant.action].some((value) => value.toLowerCase().includes(normalizedQuery)),
  )

  async function save(event: FormEvent) {
    event.preventDefault()
    try {
      await api('/api/v1/admin/grants', { method: 'POST', body: JSON.stringify(form) })
      setOpen(false)
      setForm(newGrantDraft())
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  async function revoke(id: string) {
    if (!window.confirm('确定撤销这条授权吗？撤销后该账号将立即失去对应访问权限。')) return
    try {
      await api(`/api/v1/admin/grants/${id}`, { method: 'DELETE' })
      await refresh()
    } catch (error) {
      fail((error as Error).message)
    }
  }

  async function authorize() {
    try {
      const result = await api<{ allowed: boolean }>('/api/v1/admin/authorize', { method: 'POST', body: JSON.stringify(test) })
      setDecision(result.allowed)
    } catch (error) {
      fail((error as Error).message)
    }
  }

  return (
    <div className="pageStack">
      <section className="panel">
        <SectionHeader
          kicker="访问策略"
          title="授权矩阵"
          detail="动作按 schema_read、query_read、query_write 逐级收敛，默认拒绝。"
          action={<button className="button buttonPrimary" type="button" onClick={() => setOpen(true)}>＋ 新增授权</button>}
        />
        <ListToolbar query={query} onQueryChange={setQuery} placeholder="搜索账号、资源或动作" count={paged.total} />
        <div className="tableScroll">
          <table>
            <caption className="visuallyHidden">授权策略列表</caption>
            <thead><tr><th>账号</th><th>资源</th><th>动作</th><th>约束</th><th>创建时间</th><th>操作</th></tr></thead>
            <tbody>
              {paged.rows.length ? paged.rows.map((grant) => (
                <tr key={grant.id}>
                  <td><strong>{grant.username}</strong></td>
                  <td><code>{grant.resource_key}</code></td>
                  <td><span className={`actionTag action-${grant.action}`}>{grant.action}</span></td>
                  <td>{formatConstraint(grant.row_limit, grant.statement_timeout_ms, grant.require_reason)}</td>
                  <td>{formatDate(grant.created_at)}</td>
                  <td><button className="tableButton tableButtonDanger" type="button" onClick={() => void revoke(grant.id)}>撤销</button></td>
                </tr>
              )) : <TableEmpty colSpan={6} title={query ? '没有匹配的授权' : '还没有授权策略'} />}
            </tbody>
          </table>
        </div>
        <Pagination page={paged.page} totalPages={paged.totalPages} total={paged.total} onChange={paged.setPage} />
      </section>

      <section className="panel decisionPanel">
        <div>
          <span className="kicker">策略预演</span>
          <h2>判定一次访问</h2>
          <p>只计算授权，不连接业务数据库。适合在修改策略前确认 ALLOW / DENY。</p>
        </div>
        <div className="decisionForm">
          <select value={test.principal_id} onChange={(event) => setTest({ ...test, principal_id: event.target.value })} aria-label="选择账号"><option value="">选择账号</option>{users.map((user) => <option key={user.id} value={user.id}>{user.username}</option>)}</select>
          <select value={test.resource_id} onChange={(event) => setTest({ ...test, resource_id: event.target.value })} aria-label="选择资源"><option value="">选择资源</option>{resources.map((resource) => <option key={resource.id} value={resource.id}>{resource.resource_key}</option>)}</select>
          <select value={test.action} onChange={(event) => setTest({ ...test, action: event.target.value as Grant['action'] })} aria-label="选择动作"><option value="schema_read">schema_read</option><option value="query_read">query_read</option><option value="query_write">query_write</option></select>
          <button className="button buttonSecondary" type="button" onClick={() => void authorize()}>执行判定</button>
          {decision !== null && <strong className={`decisionResult ${decision ? 'decisionAllow' : 'decisionDeny'}`}>{decision ? 'ALLOW' : 'DENY'}</strong>}
        </div>
      </section>

      {open && (
        <Modal title="新增 / 收紧授权" onClose={() => setOpen(false)}>
          <form className="form" onSubmit={save}>
            <label htmlFor="grant-principal">账号<select id="grant-principal" name="principal_id" value={form.principal_id} onChange={(event) => setForm({ ...form, principal_id: event.target.value })} required><option value="">选择账号</option>{users.filter((user) => user.role === 'user').map((user) => <option key={user.id} value={user.id}>{user.username}</option>)}</select></label>
            <label htmlFor="grant-resource">数据库资源<select id="grant-resource" name="resource_id" value={form.resource_id} onChange={(event) => setForm({ ...form, resource_id: event.target.value })} required><option value="">选择资源</option>{resources.map((resource) => <option key={resource.id} value={resource.id}>{resource.resource_key}</option>)}</select></label>
            <label htmlFor="grant-action">动作<select id="grant-action" name="action" value={form.action} onChange={(event) => setForm({ ...form, action: event.target.value as Grant['action'] })}><option value="schema_read">schema_read</option><option value="query_read">query_read</option><option value="query_write">query_write</option></select></label>
            <div className="fieldGrid"><label htmlFor="grant-row-limit">行数上限<input id="grant-row-limit" name="row_limit" type="number" value={form.row_limit} onChange={(event) => setForm({ ...form, row_limit: Number(event.target.value) })} /></label><label htmlFor="grant-timeout">超时 ms<input id="grant-timeout" name="statement_timeout_ms" type="number" value={form.statement_timeout_ms} onChange={(event) => setForm({ ...form, statement_timeout_ms: Number(event.target.value) })} /></label></div>
            <label className="checkField" htmlFor="grant-reason"><input id="grant-reason" type="checkbox" checked={form.require_reason} onChange={(event) => setForm({ ...form, require_reason: event.target.checked })} />必须提供 reason</label>
            <ModalActions onCancel={() => setOpen(false)} submit="保存授权" />
          </form>
        </Modal>
      )}
    </div>
  )
}

function Audits({ audits, meta, onPageChange }: { audits: Audit[]; meta: AuditPage; onPageChange: (page: number) => void }) {
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState('')
  const outcomes = useMemo(() => Array.from(new Set(audits.map((audit) => audit.outcome))), [audits])
  const rows = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    return audits.filter((audit) => {
      const matchesStatus = !filter || audit.outcome === filter
      const matchesQuery = !normalizedQuery || [audit.username, audit.resource_key, audit.sql_text, audit.request_id, audit.error_code].filter(Boolean).some((value) => value!.toLowerCase().includes(normalizedQuery))
      return matchesStatus && matchesQuery
    })
  }, [audits, filter, query])

  return (
    <div className="pageStack">
      <section className="panel">
        <SectionHeader kicker="请求追踪" title="审计记录" detail="每条记录对应一次请求，展示 SQL 和最终执行结果。搜索和状态筛选仅作用于当前页。" />
        <div className="auditToolbar">
          <SearchField value={query} onChange={setQuery} placeholder="搜索账号、资源、SQL 或 Request ID" />
          <select className="compactSelect" value={filter} onChange={(event) => setFilter(event.target.value)} aria-label="按状态筛选"><option value="">全部状态</option>{outcomes.map((outcome) => <option key={outcome} value={outcome}>{outcome}</option>)}</select>
        </div>
        {rows.length ? <AuditTable audits={rows} /> : <EmptyState title="没有匹配的审计记录" detail="可以尝试清空搜索或切换状态筛选。" />}
        <Pagination page={meta.page} totalPages={meta.total_pages} total={meta.total} onChange={onPageChange} />
      </section>
    </div>
  )
}

export function AuditTable({ audits }: { audits: Audit[] }) {
  return (
    <div className="tableScroll">
      <table className="auditTable">
        <caption className="visuallyHidden">审计记录列表</caption>
        <thead><tr><th>时间</th><th>账号 / 资源</th><th>SQL</th><th>动作</th><th>结果</th><th>耗时</th><th>Request ID</th></tr></thead>
        <tbody>{audits.map((audit) => (
          <tr key={audit.id}>
            <td>{formatDate(audit.created_at)}</td>
            <td><strong>{audit.username || '—'}</strong><small>{audit.resource_key || '—'}</small></td>
            <td><code className="auditSql">{audit.sql_text || '—'}</code></td>
            <td><code>{audit.action || audit.tool}</code></td>
            <td><Status value={audit.outcome} />{audit.error_code && <small>{audit.error_code}</small>}</td>
            <td>{audit.duration_ms ?? '—'} ms</td>
            <td><code className="dim">{audit.request_id.slice(0, 8)}</code></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  )
}
