import { useEffect, useState, type FormEvent } from 'react'
import { api } from './api'
import Workspace from './Workspace'
import type { User } from './types'

export default function App() {
  const [me, setMe] = useState<User | null>(null)
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    void api<User>('/api/v1/auth/me')
      .then(setMe)
      .catch(() => undefined)
      .finally(() => setChecking(false))
  }, [])

  if (checking) {
    return <div className="bootScreen"><span className="bootMark">DB</span><p>正在连接控制面…</p></div>
  }
  if (!me) return <Login onLogin={setMe} />
  return <Workspace me={me} onLogout={() => setMe(null)} />
}

function Login({ onLogin }: { onLogin: (user: User) => void }) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('admin_123')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setLoading(true)
    try {
      const user = await api<User>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) })
      onLogin(user)
    } catch (requestError) {
      setError((requestError as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="loginScreen">
      <section className="loginCard" aria-labelledby="login-title">
        <h2 id="login-title">数据库访问网关</h2>
        <p className="loginIntro">使用账号密码进入你的工作区。</p>
        <form className="form" onSubmit={submit}>
          <label htmlFor="login-username">账号<input id="login-username" name="username" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" autoFocus required /></label>
          <label htmlFor="login-password">密码<input id="login-password" name="password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></label>
          {error && <div className="loginError" role="alert">{error}</div>}
          <button className="button buttonPrimary buttonFull" type="submit" disabled={loading}>{loading ? '正在登录…' : '进入控制台'}</button>
        </form>
        <div className="loginHint"><span>首次运行</span><code>admin / admin_123</code><small>登录后请立即修改管理员密码</small></div>
      </section>
    </div>
  )
}
