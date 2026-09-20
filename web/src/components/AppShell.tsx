import type { ReactNode } from 'react'
import type { User } from '../types'
import { getInitial } from './ui'

export type Tab =
  | 'overview'
  | 'users'
  | 'resources'
  | 'grants'
  | 'audits'
  | 'user-home'
  | 'my-access'
  | 'my-tokens'
  | 'security'
  | 'masking'

type NavItem = { id: Tab; label: string; icon: string }
type NavSection = { label: string; items: NavItem[] }

const adminSections: NavSection[] = [
  { label: '控制面', items: [{ id: 'overview', label: '工作台', icon: '⌂' }] },
  {
    label: '访问治理',
    items: [
      { id: 'users', label: '用户账号', icon: '◎' },
      { id: 'resources', label: '数据库资源', icon: '▦' },
      { id: 'grants', label: '授权策略', icon: '◇' },
      { id: 'masking', label: '字段脱敏', icon: '▤' },
    ],
  },
  { label: '可观测性', items: [{ id: 'audits', label: '审计记录', icon: '≋' }] },
]

const userSections: NavSection[] = [
  { label: '我的工作区', items: [{ id: 'user-home', label: '工作台', icon: '⌂' }] },
  {
    label: '访问凭证',
    items: [
      { id: 'my-access', label: '我的资源', icon: '▦' },
      { id: 'my-tokens', label: 'Access Token', icon: '◇' },
    ],
  },
  { label: '账户', items: [{ id: 'security', label: '安全设置', icon: '⚿' }] },
]

const pageDetails: Record<Tab, string> = {
  masking: '管理数据库表字段的明文、半脱敏和全脱敏规则。',
  overview: '查看账号、数据库资源和最近访问记录。',
  users: '注册账号、停用成员或重置登录密码。',
  resources: '管理网关可以连接的数据库和连接约束。',
  grants: '按账号和资源配置最小权限，并在执行前预演判定结果。',
  audits: '每个请求只展示最终状态，便于定位成功、拒绝和失败。',
  'user-home': '查看你当前的访问范围和 MCP 凭证状态。',
  'my-access': '管理员配置的资源和权限仅对你只读展示。',
  'my-tokens': '创建、复制或撤销自己的 MCP Access Token。',
  security: '修改登录密码，保护控制台会话。',
}

export function AppShell({
  me,
  activeTab,
  onTabChange,
  onRefresh,
  onLogout,
  loading,
  children,
}: {
  me: User
  activeTab: Tab
  onTabChange: (tab: Tab) => void
  onRefresh: () => void
  onLogout: () => void
  loading: boolean
  children: ReactNode
}) {
  const isAdmin = me.role === 'admin'
  const sections = isAdmin ? adminSections : userSections
  const activeItem = sections.flatMap((section) => section.items).find((item) => item.id === activeTab)
  const workspaceLabel = isAdmin ? '管理员工作区' : '用户工作区'

  return (
    <div className="appShell">
      <a className="skipLink" href="#main-content">跳到主要内容</a>
      <aside className="sidebar">
        <div className="brandLockup">
          <div className="brandMark" aria-hidden="true"><span>DB</span><i /></div>
          <div>
            <strong>Access Gateway</strong>
            <small>数据库访问管理</small>
          </div>
        </div>

        <div className="workspaceStatus">
          <span className="liveDot" aria-hidden="true" />
          <span>{workspaceLabel}</span>
          <span className="workspaceStatusMeta">在线</span>
        </div>

        <nav className="sideNav" aria-label="主导航">
          {sections.map((section) => (
            <div className="navSection" key={section.label}>
              <span className="navSectionLabel">{section.label}</span>
              {section.items.map((item) => (
                <button
                  className={`navItem ${activeTab === item.id ? 'navItemActive' : ''}`}
                  type="button"
                  key={item.id}
                  onClick={() => onTabChange(item.id)}
                  aria-current={activeTab === item.id ? 'page' : undefined}
                >
                  <span className="navIcon" aria-hidden="true">{item.icon}</span>
                  <span>{item.label}</span>
                </button>
              ))}
            </div>
          ))}
        </nav>

        <div className="sidebarFooter">
          <div className="accountMini">
            <span className="avatar" aria-hidden="true">{getInitial(me.username)}</span>
            <div>
              <strong>{me.username}</strong>
              <small>{isAdmin ? '管理员' : '成员账号'}</small>
            </div>
          </div>
          <button className="logoutButton" type="button" onClick={onLogout}>退出</button>
        </div>
      </aside>

      <main className="mainContent" id="main-content" tabIndex={-1}>
        <header className="topbar">
          <div className="pageHeading">
            <div className="breadcrumb"><span>控制台</span><b>/</b><span>{activeItem?.label ?? '工作台'}</span></div>
            <h1>{activeItem?.label ?? '工作台'}</h1>
            <p>{pageDetails[activeTab]}</p>
          </div>
          <div className="topbarActions">
            <div className="identityChip"><span className="identityDot" />{me.username}</div>
            <button className="button buttonSecondary buttonRefresh" type="button" onClick={onRefresh} disabled={loading}>
              <span aria-hidden="true">↻</span>{loading ? '同步中' : '刷新数据'}
            </button>
          </div>
        </header>
        {children}
      </main>
    </div>
  )
}

export { pageDetails }
