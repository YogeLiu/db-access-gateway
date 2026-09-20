export function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatConstraint(rowLimit?: number, timeout?: number, requireReason = false) {
  const parts = [
    rowLimit ? `${rowLimit} 行` : '',
    timeout ? `${timeout} ms` : '',
    requireReason ? '需要 reason' : '',
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : '未设置额外约束'
}
