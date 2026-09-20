import { useCallback, useEffect, useState } from 'react'
import { api } from './api'
import { AppShell, type Tab } from './components/AppShell'
import { Notice } from './components/ui'
import { AdminWorkspace } from './features/admin'
import { UserWorkspace } from './features/user'
import type { AccessResponse, APIToken, Audit, AuditPage, Grant, PrincipalAccess, Resource, TokenResponse, User } from './types'

const auditPageSize = 20
const emptyAuditPage: AuditPage = { items: [], page: 1, page_size: auditPageSize, total: 0, total_pages: 0 }

type NoticeState = { kind: 'error' | 'success'; message: string } | null

const adminTabs: Tab[] = ['overview', 'users', 'resources', 'grants', 'audits', 'masking']
const userTabs: Tab[] = ['user-home', 'my-access', 'my-tokens', 'security']

function readTab(isAdmin: boolean): Tab {
  const candidate = window.location.hash.slice(1) as Tab
  const allowed = isAdmin ? adminTabs : userTabs
  return allowed.includes(candidate) ? candidate : allowed[0]
}

function useWorkspaceData(isAdmin: boolean) {
  const [loading, setLoading] = useState(false)
  const [notice, setNotice] = useState<NoticeState>(null)
  const [users, setUsers] = useState<User[]>([])
  const [resources, setResources] = useState<Resource[]>([])
  const [grants, setGrants] = useState<Grant[]>([])
  const [audits, setAudits] = useState<Audit[]>([])
  const [auditPage, setAuditPage] = useState(1)
  const [auditMeta, setAuditMeta] = useState<AuditPage>(emptyAuditPage)
  const [access, setAccess] = useState<PrincipalAccess[]>([])
  const [tokens, setTokens] = useState<APIToken[]>([])

  const refresh = useCallback(async () => {
    setLoading(true)
    setNotice(null)
    try {
      if (isAdmin) {
        const [userResponse, resourceResponse, grantResponse, auditResponse] = await Promise.all([
          api<User[]>('/api/v1/admin/users'),
          api<Resource[]>('/api/v1/admin/resources'),
          api<Grant[]>('/api/v1/admin/grants'),
          api<AuditPage>(`/api/v1/admin/audits?page=${auditPage}&page_size=${auditPageSize}`),
        ])
        setUsers(userResponse)
        setResources(resourceResponse)
        setGrants(grantResponse)
        setAudits(auditResponse.items)
        setAuditMeta(auditResponse)
        if (auditResponse.page !== auditPage) setAuditPage(auditResponse.page)
      } else {
        const [accessResponse, tokenResponse] = await Promise.all([
          api<AccessResponse>('/api/v1/me/access'),
          api<TokenResponse>('/api/v1/me/tokens'),
        ])
        setAccess(accessResponse.items)
        setTokens(tokenResponse.items)
      }
    } catch (error) {
      setNotice({ kind: 'error', message: (error as Error).message })
    } finally {
      setLoading(false)
    }
  }, [auditPage, isAdmin])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return {
    loading,
    notice,
    setNotice,
    refresh,
    users,
    resources,
    grants,
    audits,
    auditMeta,
    setAuditPage,
    access,
    tokens,
  }
}

export default function Workspace({ me, onLogout }: { me: User; onLogout: () => void }) {
  const isAdmin = me.role === 'admin'
  const [activeTab, setActiveTab] = useState<Tab>(() => readTab(isAdmin))
  const data = useWorkspaceData(isAdmin)

  useEffect(() => {
    const handleHashChange = () => setActiveTab(readTab(isAdmin))
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [isAdmin])

  function navigate(tab: Tab) {
    setActiveTab(tab)
    window.history.replaceState(null, '', `#${tab}`)
  }

  async function logout() {
    await api('/api/v1/auth/logout', { method: 'POST' }).catch(() => undefined)
    onLogout()
  }

  return (
    <AppShell
      me={me}
      activeTab={activeTab}
      onTabChange={navigate}
      onRefresh={() => void data.refresh()}
      onLogout={() => void logout()}
      loading={data.loading}
    >
      {data.notice && <Notice kind={data.notice.kind} onDismiss={() => data.setNotice(null)}>{data.notice.message}</Notice>}
      {isAdmin ? (
        <AdminWorkspace
          users={data.users}
          resources={data.resources}
          grants={data.grants}
          audits={data.audits}
          auditMeta={data.auditMeta}
          refresh={data.refresh}
          fail={(message) => data.setNotice({ kind: 'error', message })}
          notify={(message) => data.setNotice({ kind: 'success', message })}
          activeTab={activeTab}
          onTabChange={navigate}
          onAuditPageChange={data.setAuditPage}
        />
      ) : (
        <UserWorkspace
          activeTab={activeTab}
          me={me}
          access={data.access}
          tokens={data.tokens}
          refresh={data.refresh}
          fail={(message) => data.setNotice({ kind: 'error', message })}
          onTabChange={navigate}
        />
      )}
    </AppShell>
  )
}
