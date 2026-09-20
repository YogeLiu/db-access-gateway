export async function api<T>(path:string, init:RequestInit = {}):Promise<T> {
  const response = await fetch(path, { ...init, credentials:'same-origin', headers: { 'Content-Type':'application/json', ...(init.headers || {}) } })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(payload?.error?.message || `HTTP ${response.status}`)
  return payload as T
}
