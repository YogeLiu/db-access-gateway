import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import type { Resource } from '../types'
import { Modal, Notice, Pagination, SearchField, SectionHeader } from '../components/ui'

type Field = { table_name: string; column_name: string; data_type: string; table_type: string }
type Rule = { table_name: string; column_name: string; mode: string; keep_prefix?: number; keep_suffix?: number }
const labels: Record<string, string> = { plain: '明文', partial: '半脱敏', full: '全脱敏' }

export function Masking({ resources }: { resources: Resource[] }) {
  const [resourceId, setResourceId] = useState('')
  const [tables, setTables] = useState<string[]>([])
  const [table, setTable] = useState('')
  const [tableQuery, setTableQuery] = useState('')
  const [tablesLoading, setTablesLoading] = useState(false)
  const [prefix, setPrefix] = useState(3)
  const [suffix, setSuffix] = useState(4)
  const [sample, setSample] = useState('13812345678')
  const [fields, setFields] = useState<Field[]>([])
  const [rules, setRules] = useState<Rule[]>([])
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Field | null>(null)
  const [mode, setMode] = useState('plain')
  const [saving, setSaving] = useState(false)
  const id = resourceId
  useEffect(() => {
    const controller = new AbortController()
    setTables([]); setTable(''); setTablesLoading(false)
    if (!id) return
    setTablesLoading(true); setError('')
    api<string[]>(`/api/v1/admin/resources/${id}/tables`, { signal: controller.signal })
      .then(t => { if (!controller.signal.aborted) setTables(t) })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setTablesLoading(false) })
    return () => controller.abort()
  }, [id])
  useEffect(() => {
    const controller = new AbortController()
    setFields([]); setRules([]); setError(''); setPage(1)
    setLoading(false)
    if (!id || !table) return
    setLoading(true)
    Promise.all([
      api<Field[]>(`/api/v1/admin/resources/${id}/fields?table=${encodeURIComponent(table)}`, { signal: controller.signal }),
      api<Rule[]>(`/api/v1/admin/resources/${id}/masking`, { signal: controller.signal }),
    ]).then(([f, r]) => { if (!controller.signal.aborted) { setFields(f.filter(x => x.table_type === 'BASE TABLE')); setRules(r) } })
      .catch(e => { if (!controller.signal.aborted) setError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [id, table, revision])
  const getMode = (f: Field) => rules.find(r => r.table_name === f.table_name && r.column_name === f.column_name)?.mode || 'plain'
  const rows = fields.filter(f => (f.table_name + '.' + f.column_name).toLowerCase().includes(query.toLowerCase()))
  async function save(e: FormEvent) {
    e.preventDefault()
    if (!editing || saving) return
    setSaving(true); setError('')
    try {
      await api(`/api/v1/admin/resources/${id}/masking`, { method: 'PUT', body: JSON.stringify({ table_name: editing.table_name, column_name: editing.column_name, mode, keep_prefix: prefix, keep_suffix: suffix }) })
      setEditing(null); setRevision(v => v + 1)
    } catch (e) { setError((e as Error).message) } finally { setSaving(false) }
  }
  return <section className="panel">
    <SectionHeader kicker="" title="敏感字段脱敏" detail="按数据库资源、表和字段配置，对该资源的所有 MCP 用户生效。未配置的字段默认明文。" />
    <div className="auditToolbar maskingToolbar">
      <label>数据库资源<select value={id} disabled={saving} onChange={e => { setResourceId(e.target.value); setTable(''); setFields([]); setTableQuery(''); setEditing(null) }}><option value="">请选择数据库</option>{resources.map(r => <option key={r.id} value={r.id}>{r.resource_key} / {r.database_name}</option>)}</select></label>
      {id && <><SearchField value={tableQuery} onChange={setTableQuery} placeholder="搜索数据表" /><label>数据表<select value={table} disabled={tablesLoading || saving} onChange={e => { setTable(e.target.value); setFields([]); setQuery(''); setPage(1); setEditing(null) }}><option value="">{tablesLoading ? '加载数据表中…' : '请选择数据表'}</option>{tables.filter(t => t === table || t.toLowerCase().includes(tableQuery.toLowerCase())).map(t => <option key={t} value={t}>{t}</option>)}</select></label></>}
    </div>
    <p className="formHint">半脱敏可设置保留前后位数：手机号可保留前 3 后 4 位（138****5678）。长度不足时全隐藏，NULL 保持 NULL。</p>
    <p className="formHint">启用后，筛选、排序和表达式均基于脱敏值执行；该资源只允许读取基础表，不支持写入、视图和自定义函数。</p>
    {error && <Notice kind="error">{error}</Notice>}
    {!table ? <p className="formHint">请选择数据库和数据表，再查看字段。</p> : <>
      <SearchField value={query} onChange={v => { setQuery(v); setPage(1) }} placeholder="搜索当前表字段" />
      {loading ? <p role="status">加载字段中…</p> : <div className="tableScroll"><table><thead><tr><th>字段</th><th>类型</th><th>显示方式</th><th>操作</th></tr></thead><tbody>{rows.slice((page - 1) * 20, page * 20).map(f => {
        const rule = rules.find(r => r.table_name === f.table_name && r.column_name === f.column_name)
        return <tr key={f.column_name}><td>{f.column_name}</td><td>{f.data_type}</td><td>{labels[getMode(f)]}{rule?.mode === 'partial' && <small>保留前 {rule.keep_prefix ?? 1} / 后 {rule.keep_suffix ?? 1} 位</small>}</td><td><button className="tableButton" onClick={() => { setEditing(f); setMode(getMode(f)); setPrefix(rule?.keep_prefix ?? 3); setSuffix(rule?.keep_suffix ?? 4); setSample('13812345678'); setError('') }}>设置脱敏</button></td></tr>
      })}{!rows.length && <tr><td colSpan={4}>没有匹配的字段。</td></tr>}</tbody></table></div>}
      <Pagination page={page} totalPages={Math.ceil(rows.length / 20)} total={rows.length} onChange={setPage} />
    </>}
    {editing && <Modal title={editing.table_name + '.' + editing.column_name} onClose={() => { if (!saving) setEditing(null) }}><form className="form" onSubmit={save}>
      <label>显示方式<select value={mode} onChange={e => setMode(e.target.value)}>{Object.entries(labels).map(([v, label]) => <option value={v} key={v}>{label}</option>)}</select></label>
      {mode === 'partial' && <>
        <label>常用格式<select value={prefix === 3 && suffix === 4 ? 'phone' : prefix === 1 && suffix === 1 ? 'name' : prefix === 6 && suffix === 4 ? 'id' : 'custom'} onChange={e => { const preset = e.target.value; if (preset === 'phone') { setPrefix(3); setSuffix(4) } else if (preset === 'name') { setPrefix(1); setSuffix(1) } else if (preset === 'id') { setPrefix(6); setSuffix(4) } }}><option value="phone">手机号：前 3 后 4</option><option value="name">姓名：前 1 后 1</option><option value="id">证件号：前 6 后 4</option><option value="custom">自定义（修改下方位数）</option></select></label>
        <div className="fieldGrid"><label>保留前几位<input type="number" min={0} max={64} step={1} required value={prefix} onChange={e => setPrefix(Number(e.target.value))} /></label><label>保留后几位<input type="number" min={0} max={64} step={1} required value={suffix} onChange={e => setSuffix(Number(e.target.value))} /></label></div>
      </>}
      <label>预览文本（不会保存）<input value={sample} onChange={e => setSample(e.target.value)} /></label>
      <p className="formHint">脱敏结果：{mode === 'plain' ? sample : mode === 'full' || Array.from(sample).length <= prefix + suffix ? '******' : Array.from(sample).slice(0, prefix).join('') + '****' + (suffix ? Array.from(sample).slice(-suffix).join('') : '')}</p>
      {error && <Notice kind="error">{error}</Notice>}<button className="button buttonPrimary" disabled={saving}>{saving ? '保存中…' : '保存规则'}</button>
    </form></Modal>}
  </section>
}
