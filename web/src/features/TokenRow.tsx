import { useEffect, useState } from 'react'
import { api } from '../api'
import type { APIToken } from '../types'
import { formatDate } from '../lib/format'

function useTokenSecret(id: string) {
  const [result, setResult] = useState({ id: '', token: '', error: '' })
  useEffect(() => {
    if (!id) return
    const controller = new AbortController()
    setResult({ id, token: '', error: '' })
    api<{ token: string }>(`/api/v1/me/tokens/${id}/secret`, { cache: 'no-store', signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setResult({ id, token: value.token, error: '' }) })
      .catch(error => { if (!controller.signal.aborted) setResult({ id, token: '', error: error.message }) })
    return () => controller.abort()
  }, [id])
  return id && result.id === id ? result : { id, token: '', error: '' }
}

export function TokenRow({ item, deleting, busy, onDelete }: { item: APIToken; deleting: boolean; busy: boolean; onDelete: () => void }) {
  const expired = Boolean(item.expires_at && new Date(item.expires_at).getTime() <= Date.now())
  const revoked = Boolean(item.revoked_at)
  const available = item.config_available && !expired && !revoked
  const [visible, setVisible] = useState(false)
  const { token, error } = useTokenSecret(available && visible ? item.id : '')
  return <article className="tokenRow">
    <div className="tokenRowHeader">
      <strong className="tokenName">{item.name || '未命名 Token'}</strong>
      <div className="tokenActions">
        <span className={`tokenState ${revoked || expired ? 'tokenStateBad' : 'tokenStateGood'}`}>{revoked ? '已撤销' : expired ? '已过期' : '有效'}</span>
        {available && <button className="tokenVisibility" type="button" role="switch" aria-checked={visible} aria-label={`显示 ${item.name || '未命名 Token'} 的明文`} title={visible ? '隐藏 Token' : '显示 Token'} onClick={() => setVisible(v => !v)}><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" />{visible && <path d="m3 3 18 18" />}</svg></button>}
        <button className="tableButton tableButtonDanger" type="button" disabled={busy} onClick={onDelete}>{deleting ? '删除中…' : '删除'}</button>
      </div>
    </div>
    <code className="tokenValue">{available && visible ? token || (error ? '无法加载 Token' : '加载中…') : item.prefix + '••••••••'}</code>
    <div className="tokenMeta"><span>创建于 {formatDate(item.created_at)}</span><span>{item.expires_at ? `到期 ${formatDate(item.expires_at)}` : '永不过期'}</span></div>
    {!item.config_available && !revoked && !expired && <p className="tokenHint">旧 Token 无法恢复原文，遗失后请新建。</p>}
    {available && visible && error && <p className="tokenHint" role="alert">{error}</p>}
  </article>
}
