import { useEffect, useId, type ReactNode } from 'react'

type NoticeKind = 'error' | 'success' | 'info'

export function Notice({
  kind,
  children,
  onDismiss,
}: {
  kind: NoticeKind
  children: ReactNode
  onDismiss?: () => void
}) {
  return (
    <div className={`notice notice-${kind}`} role={kind === 'error' ? 'alert' : 'status'} aria-live="polite">
      <span className="noticeMark" aria-hidden="true">
        {kind === 'error' ? '!' : kind === 'success' ? '✓' : 'i'}
      </span>
      <span className="noticeText">{children}</span>
      {onDismiss && (
        <button className="iconButton" type="button" onClick={onDismiss} aria-label="关闭提示">
          ×
        </button>
      )}
    </div>
  )
}

export function MetricCard({
  label,
  value,
  detail,
  tone = 'default',
}: {
  label: string
  value: number | string
  detail: string
  tone?: 'default' | 'warning' | 'danger'
}) {
  return (
    <article className={`metricCard metricCard-${tone}`}>
      <div className="metricCardTop">
        <span>{label}</span>
        <span className="metricDot" aria-hidden="true" />
      </div>
      <strong>{value}</strong>
      <small>{detail}</small>
    </article>
  )
}

export function Status({ value }: { value: string }) {
  const labels: Record<string, string> = {
    active: '正常',
    disabled: '已停用',
    enabled: '已启用',
    SUCCEEDED: '成功',
    COMMITTED: '成功',
    DENIED: '拒绝',
    REJECTED: '拦截',
    FAILED: '失败',
  }
  const good = ['active', 'enabled', 'SUCCEEDED', 'COMMITTED'].includes(value)
  const bad = ['disabled', 'DENIED', 'REJECTED', 'FAILED'].includes(value)
  return (
    <span className={`status ${good ? 'statusGood' : ''} ${bad ? 'statusBad' : ''}`} title={value}>
      <i aria-hidden="true" />
      {labels[value] ?? value}
    </span>
  )
}

export function EmptyState({
  title,
  detail,
  action,
}: {
  title: string
  detail?: string
  action?: ReactNode
}) {
  return (
    <div className="emptyState">
      <span className="emptyGlyph" aria-hidden="true">⌁</span>
      <strong>{title}</strong>
      {detail && <p>{detail}</p>}
      {action}
    </div>
  )
}

export function Pagination({
  page,
  totalPages,
  total,
  onChange,
}: {
  page: number
  totalPages: number
  total: number
  onChange: (page: number) => void
}) {
  const safeTotalPages = Math.max(totalPages, 1)
  return (
    <div className="pagination" aria-label="分页">
      <span>共 {total} 项</span>
      <div className="paginationControls">
        <button type="button" disabled={page <= 1} onClick={() => onChange(page - 1)}>
          上一页
        </button>
        <strong>{page} / {safeTotalPages}</strong>
        <button type="button" disabled={page >= safeTotalPages} onClick={() => onChange(page + 1)}>
          下一页
        </button>
      </div>
    </div>
  )
}

export function SearchField({
  value,
  onChange,
  placeholder,
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
}) {
  return (
    <label className="searchField">
      <span aria-hidden="true">⌕</span>
      <input
        name="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        autoComplete="off"
      />
      {value && (
        <button type="button" onClick={() => onChange('')} aria-label="清除搜索">
          ×
        </button>
      )}
    </label>
  )
}

export function Modal({
  title,
  onClose,
  children,
  wide = false,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  wide?: boolean
}) {
  const titleId = useId()

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKeyDown)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.body.style.overflow = previousOverflow
    }
  }, [onClose])

  return (
    <div className="modalBackdrop" onMouseDown={onClose}>
      <div
        className={`modal ${wide ? 'modalWide' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="modalHead">
          <h2 id={titleId}>{title}</h2>
          <button className="iconButton modalClose" type="button" onClick={onClose} aria-label="关闭弹窗">
            ×
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}

export function ModalActions({ onCancel, submit }: { onCancel: () => void; submit: string }) {
  return (
    <div className="modalActions">
      <button type="button" className="button buttonSecondary" onClick={onCancel}>取消</button>
      <button type="submit" className="button buttonPrimary">{submit}</button>
    </div>
  )
}

export function SectionHeader({
  kicker,
  title,
  detail,
  action,
}: {
  kicker: string
  title: string
  detail: string
  action?: ReactNode
}) {
  return (
    <div className="sectionHeader">
      <div>
        <span className="kicker">{kicker}</span>
        <h2>{title}</h2>
        <p>{detail}</p>
      </div>
      {action}
    </div>
  )
}

export function TableEmpty({ colSpan, title }: { colSpan: number; title: string }) {
  return (
    <tr>
      <td colSpan={colSpan}>
        <EmptyState title={title} />
      </td>
    </tr>
  )
}

export function getInitial(value: string) {
  return value.trim().slice(0, 1).toUpperCase() || '?'
}
