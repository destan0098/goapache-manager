import { useCallback, useEffect, useState, type FormEvent } from 'react'
import {
  Activity,
  Archive,
  CheckCircle2,
  CircleAlert,
  Clock,
  Database,
  Globe,
  LayoutDashboard,
  LogOut,
  Plus,
  RefreshCw,
  RotateCcw,
  Server,
  ShieldCheck,
  Trash2,
  Power,
  PowerOff,
  Pencil,
} from 'lucide-react'
import type { Backup, User, VHost, VHostDetails } from './api/client'
import { api } from './api/client'


type Page = 'dashboard' | 'vhosts' | 'backups'

function App() {
  const [user, setUser] = useState<User | null>(null)
  const [checking, setChecking] = useState(true)
  const [page, setPage] = useState<Page>('dashboard')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [loginLoading, setLoginLoading] = useState(false)

  const [status, setStatus] = useState('Unknown')
  const [statusLoading, setStatusLoading] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)

  const [vhosts, setVhosts] = useState<VHost[]>([])
  const [vhostsLoading, setVhostsLoading] = useState(false)
  const [newDomain, setNewDomain] = useState('')

  const [selectedDomain, setSelectedDomain] = useState('')
  const [backups, setBackups] = useState<Backup[]>([])
  const [backupsLoading, setBackupsLoading] = useState(false)

  const [editingVHost, setEditingVHost] = useState<VHostDetails | null>(null)
  const [editDocumentRoot, setEditDocumentRoot] = useState('')
  const [editServerAliases, setEditServerAliases] = useState('')
  const [editLoading, setEditLoading] = useState(false)
  const [editSaving, setEditSaving] = useState(false)
  const [editModalOpen, setEditModalOpen] = useState(false)
  const [editError, setEditError] = useState('')

  const loadStatus = useCallback(async () => {
    setStatusLoading(true)
    try {
      const result = await api.status()
      setStatus(result.output?.trim() || (result.success ? 'Active' : 'Unknown'))
    } catch {
      setStatus('Unavailable')
    } finally {
      setStatusLoading(false)
    }
  }, [])

  const loadVHosts = useCallback(async () => {
    setVhostsLoading(true)
    try {
      const result = await api.vhosts()
      setVhosts(result.vhosts || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not load VirtualHosts')
    } finally {
      setVhostsLoading(false)
    }
  }, [])

  const loadBackups = useCallback(async (domain: string) => {
    if (!domain) {
      setBackups([])
      return
    }

    setBackupsLoading(true)
    try {
      const result = await api.backups(domain)
      setBackups(result.backups || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not load backups')
    } finally {
      setBackupsLoading(false)
    }
  }, [])

  useEffect(() => {
    api.me()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setChecking(false))
  }, [])

  useEffect(() => {
    if (user) void loadStatus()
  }, [user, loadStatus])

  useEffect(() => {
    if (user) void loadVHosts()
  }, [user, loadVHosts])

  useEffect(() => {
    if (user && page === 'backups' && selectedDomain) {
      void loadBackups(selectedDomain)
    }
  }, [user, page, selectedDomain, loadBackups])

  function showNotice(message: string) {
    setError('')
    setNotice(message)
    window.setTimeout(() => setNotice(''), 4000)
  }

  async function handleLogin(event: FormEvent) {
    event.preventDefault()
    setError('')
    setLoginLoading(true)

    try {
      await api.login(username, password)
      const currentUser = await api.me()
      setUser(currentUser)
      setPassword('')
      showNotice('Successfully signed in')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoginLoading(false)
    }
  }
  const handleOpenEditVHost = async (domain: string) => {
    setError('')
    setEditError('')
    setEditingVHost(null)
    setEditDocumentRoot('')
    setEditServerAliases('')
    setEditModalOpen(true)
    setEditLoading(true)

    try {
      const response = await api.getVHost(domain)

      setEditingVHost(response.vhost)
      setEditDocumentRoot(response.vhost.document_root)
      setEditServerAliases(response.vhost.server_aliases.join('\n'))
    } catch (err) {
      setEditError(
          err instanceof Error ? err.message : 'Failed to load VirtualHost details'
      )
    } finally {
      setEditLoading(false)
    }
  }

  async function handleSaveEditVHost(event: FormEvent) {
    event.preventDefault()
    setEditError('')

    if (!editingVHost) return

    setError('')
    setEditSaving(true)

    try {
      const aliases = editServerAliases
          .split(/[\s,]+/)
          .map(alias => alias.trim())
          .filter(Boolean)

      await api.editVHost(
          editingVHost.domain,
          editDocumentRoot.trim(),
          aliases
      )

      showNotice(`VirtualHost ${editingVHost.domain} updated successfully`)
      setEditModalOpen(false)
      setEditingVHost(null)

      await loadVHosts()
    } catch (err) {
      setEditError(
          err instanceof Error ? err.message : 'Failed to save VirtualHost'
      )
    }finally {
      setEditSaving(false)
    }
  }
  async function handleLogout() {
    try {
      await api.logout()
    } catch {
      // Clear local UI even if the server session has expired.
    }
    setUser(null)
    setPage('dashboard')
  }

  async function handleApacheAction(action: 'config' | 'reload' | 'restart') {
    const labels = {
      config: 'Configuration test',
      reload: 'Apache reload',
      restart: 'Apache restart',
    }

    if (action !== 'config' && !window.confirm(`Are you sure you want to ${action} Apache?`)) {
      return
    }

    setActionLoading(true)
    setError('')

    try {
      const result =
        action === 'config'
          ? await api.configTest()
          : action === 'reload'
            ? await api.reload()
            : await api.restart()

      if (!result.success) {
        throw new Error(result.error || result.output || `${labels[action]} failed`)
      }

      showNotice(result.output || `${labels[action]} completed successfully`)
      await loadStatus()
    } catch (err) {
      setError(err instanceof Error ? err.message : `${labels[action]} failed`)
    } finally {
      setActionLoading(false)
    }
  }

  async function handleCreateVHost(event: FormEvent) {
    event.preventDefault()
    const domain = newDomain.trim()
    if (!domain) return

    setError('')
    try {
      await api.createVHost(domain)
      setNewDomain('')
      showNotice(`VirtualHost ${domain} created`)
      await loadVHosts()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create VirtualHost')
    }
  }

  async function handleVHostAction(domain: string, action: 'enable' | 'disable' | 'delete') {
    if (!window.confirm(`Are you sure you want to ${action} ${domain}?`)) return

    setError('')
    try {
      if (action === 'enable') await api.enableVHost(domain)
      if (action === 'disable') await api.disableVHost(domain)
      if (action === 'delete') await api.deleteVHost(domain)

      showNotice(`${domain}: ${action} completed`)
      await loadVHosts()
    } catch (err) {
      setError(err instanceof Error ? err.message : `Could not ${action} VirtualHost`)
    }
  }

  async function handleBackupAction(backup: Backup, action: 'restore' | 'delete') {
    if (!selectedDomain) return

    const confirmed = window.confirm(
      `${action === 'restore' ? 'Restore' : 'Permanently delete'} backup "${backup.id}"?`,
    )
    if (!confirmed) return

    setError('')
    try {
      if (action === 'restore') {
        await api.restoreBackup(selectedDomain, backup.id)
      } else {
        await api.deleteBackup(selectedDomain, backup.id)
      }

      showNotice(`Backup ${action} completed successfully`)
      await loadBackups(selectedDomain)
    } catch (err) {
      setError(err instanceof Error ? err.message : `Backup ${action} failed`)
    }
  }

  if (checking) {
    return <div className="screen-center">Loading GoApache Manager...</div>
  }

  if (!user) {
    return (
      <div className="login-screen">
        <form className="login-card" onSubmit={handleLogin}>
          <div className="login-logo"><Server size={30} /></div>
          <h1>GoApache Manager</h1>
          <p className="muted">Sign in to manage your Apache server</p>

          {error && <div className="alert error"><CircleAlert size={17} />{error}</div>}

          <label>Username</label>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
          />

          <label>Password</label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />

          <button className="primary full" disabled={loginLoading}>
            {loginLoading ? 'Signing in...' : 'Sign in'}
          </button>
          <div className="login-security"><ShieldCheck size={15} /> Secure session authentication</div>
        </form>
      </div>
    )
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-icon"><Server size={22} /></span>
          <span>GoApache</span>
        </div>

        <div className="nav-label">MAIN MENU</div>
        <button className={`nav-item ${page === 'dashboard' ? 'selected' : ''}`} onClick={() => setPage('dashboard')}>
          <LayoutDashboard size={18} /> Dashboard
        </button>
        <button className={`nav-item ${page === 'vhosts' ? 'selected' : ''}`} onClick={() => setPage('vhosts')}>
          <Globe size={18} /> Virtual Hosts
        </button>
        <button className={`nav-item ${page === 'backups' ? 'selected' : ''}`} onClick={() => setPage('backups')}>
          <Archive size={18} /> Backup Manager
        </button>

        <div className="sidebar-bottom">
          <div className="user-box">
            <div className="avatar">{user.username.slice(0, 1).toUpperCase()}</div>
            <div className="user-info">
              <strong>{user.username}</strong>
              <small>{user.role}</small>
            </div>
            <button className="icon-button" title="Logout" onClick={handleLogout}><LogOut size={17} /></button>
          </div>
          <div className="version">GoApache Manager · v1.0.0</div>
        </div>
      </aside>

      <main className="main-content">
        <header className="topbar">
          <div>
            <div className="breadcrumb">GoApache / <strong>{page === 'vhosts' ? 'Virtual Hosts' : page === 'backups' ? 'Backup Manager' : 'Dashboard'}</strong></div>
            <h1>{page === 'vhosts' ? 'Virtual Hosts' : page === 'backups' ? 'Backup Manager' : 'Dashboard'}</h1>
          </div>
          <div className="topbar-right">
            <span className="connection"><span className="live-dot" /> API Connected</span>
            <div className="avatar">{user.username.slice(0, 1).toUpperCase()}</div>
          </div>
        </header>

        {(error || notice) && (
          <div className={`alert ${error ? 'error' : 'success'}`}>
            {error ? <CircleAlert size={17} /> : <CheckCircle2 size={17} />}
            <span>{error || notice}</span>
            <button className="alert-close" onClick={() => { setError(''); setNotice('') }}>×</button>
          </div>
        )}

        {page === 'dashboard' && (
          <section className="page-content">
            <div className="welcome-panel">
              <div>
                <span className="eyebrow">SERVER OVERVIEW</span>
                <h2>Welcome back, {user.username}</h2>
                <p>Monitor and manage your Apache web server.</p>
              </div>
              <Server size={54} />
            </div>

            <div className="section-heading">
              <div><h2>System status</h2><p>Live information from your server</p></div>
              <button className="secondary" onClick={() => void loadStatus()} disabled={statusLoading}>
                <RefreshCw size={15} className={statusLoading ? 'spin' : ''} /> Refresh
              </button>
            </div>

            <div className="stats-grid">
              <div className="stat-card">
                <div className="stat-top"><span>Apache service</span><Activity size={19} /></div>
                <strong className={status.toLowerCase() === 'active' ? 'green-text' : ''}>{status}</strong>
                <small>systemd service status</small>
              </div>
              <div className="stat-card">
                <div className="stat-top"><span>Virtual Hosts</span><Globe size={19} /></div>
                <strong>{vhosts.length || '—'}</strong>
                <small>Load the Virtual Hosts page for details</small>
              </div>
              <div className="stat-card">
                <div className="stat-top"><span>Authentication</span><ShieldCheck size={19} /></div>
                <strong>Protected</strong>
                <small>Authenticated session</small>
              </div>
            </div>

            <div className="section-heading"><div><h2>Apache controls</h2><p>Run server maintenance operations</p></div></div>
            <div className="action-grid">
              <button className="action-card" disabled={actionLoading} onClick={() => void handleApacheAction('config')}>
                <CheckCircle2 /><span><strong>Configuration test</strong><small>Run apache configtest</small></span>
              </button>
              <button className="action-card" disabled={actionLoading} onClick={() => void handleApacheAction('reload')}>
                <RefreshCw /><span><strong>Reload Apache</strong><small>Apply configuration changes</small></span>
              </button>
              <button className="action-card" disabled={actionLoading} onClick={() => void handleApacheAction('restart')}>
                <Power /><span><strong>Restart Apache</strong><small>Restart the web server</small></span>
              </button>
            </div>
          </section>
        )}

        {page === 'vhosts' && (
          <section className="page-content">
            <div className="section-heading">
              <div><h2>Manage Virtual Hosts</h2><p>Create and control Apache virtual hosts</p></div>
              <button className="secondary" onClick={() => void loadVHosts()} disabled={vhostsLoading}>
                <RefreshCw size={15} className={vhostsLoading ? 'spin' : ''} /> Refresh
              </button>
            </div>

            <form className="create-form" onSubmit={handleCreateVHost}>
              <div><Plus size={19} /><strong>Create Virtual Host</strong></div>
              <input
                placeholder="example.com"
                value={newDomain}
                onChange={(e) => setNewDomain(e.target.value)}
                required
              />
              <button className="primary" disabled={!newDomain.trim()}>Create</button>
            </form>

            <div className="table-card">
              <div className="table-title"><Database size={18} /><strong>Virtual Hosts</strong><span>{vhosts.length} total</span></div>
              {vhostsLoading ? <div className="empty-state">Loading Virtual Hosts...</div> : vhosts.length === 0 ? (
                <div className="empty-state">No Virtual Hosts found.</div>
              ) : (
                <div className="table-scroll">
                  <table>
                    <thead><tr><th>Domain</th><th>Port</th><th>Config file</th><th>Default</th><th>Actions</th></tr></thead>
                    <tbody>
                    {vhosts.map((host) => {
                      const managed =
                          host.file.endsWith(`/sites-enabled/${host.domain}.conf`)

                      return (
                          <tr key={`${host.domain}-${host.port}-${host.file}`}>
                            <td>
                              <strong>{host.domain}</strong>
                              {!managed && (
                                  <div>
                                    <span className="pill">External</span>
                                  </div>
                              )}
                            </td>

                            <td>{host.port}</td>

                            <td className="file-cell">
                              {host.file}:{host.line}
                            </td>

                            <td>
                              {host.default
                                  ? <span className="pill green">Default</span>
                                  : <span className="pill">Standard</span>}
                            </td>

                            <td>
                              <div className="row-actions">
                                {managed ? (
                                    <>
                                      <button
                                          className="icon-button"
                                          title="Edit VirtualHost"
                                          onClick={() => void handleOpenEditVHost(host.domain)}
                                      >
                                        <Pencil size={16} />
                                      </button>
                                      <button
                                          title="Enable"
                                          className="icon-button green-icon"
                                          onClick={() => void handleVHostAction(host.domain, 'enable')}
                                      >
                                        <Power size={16} />
                                      </button>

                                      <button
                                          title="Disable"
                                          className="icon-button"
                                          onClick={() => void handleVHostAction(host.domain, 'disable')}
                                      >
                                        <PowerOff size={16} />
                                      </button>

                                      <button
                                          title="Delete"
                                          className="icon-button red-icon"
                                          onClick={() => void handleVHostAction(host.domain, 'delete')}
                                      >
                                        <Trash2 size={16} />
                                      </button>

                                      <button
                                          title="View backups"
                                          className="icon-button"
                                          onClick={() => {
                                            setSelectedDomain(host.domain)
                                            setPage('backups')
                                          }}
                                      >
                                        <Archive size={16} />
                                      </button>
                                    </>
                                ) : (
                                    <span className="pill">Read only</span>
                                )}
                              </div>
                            </td>
                          </tr>
                      )
                    })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </section>
        )}

        {page === 'backups' && (
          <section className="page-content">
            <div className="section-heading">
              <div><h2>Backup Manager</h2><p>Restore or remove saved VirtualHost configurations</p></div>
              <button className="secondary" onClick={() => void loadBackups(selectedDomain)} disabled={!selectedDomain || backupsLoading}>
                <RefreshCw size={15} className={backupsLoading ? 'spin' : ''} /> Refresh
              </button>
            </div>

            <div className="backup-selector">
              <label htmlFor="domain-select">Select Virtual Host</label>
              <select
                id="domain-select"
                value={selectedDomain}
                onChange={(e) => setSelectedDomain(e.target.value)}
              >
                <option value="">Choose a domain...</option>
                {vhosts.map((host) => <option key={host.domain} value={host.domain}>{host.domain}</option>)}
              </select>
              <button className="secondary" onClick={() => { setPage('vhosts'); void loadVHosts() }}>Manage hosts</button>
            </div>

            {!selectedDomain ? (
              <div className="empty-panel"><Archive size={34} /><strong>Select a Virtual Host</strong><p>Choose a domain above to view its backups.</p></div>
            ) : (
              <div className="table-card">
                <div className="table-title"><Archive size={18} /><strong>Backups for {selectedDomain}</strong><span>{backups.length} found</span></div>
                {backupsLoading ? <div className="empty-state">Loading backups...</div> : backups.length === 0 ? (
                  <div className="empty-state">No backups found for this Virtual Host.</div>
                ) : (
                  <div className="table-scroll">
                    <table>
                      <thead><tr><th>Backup ID</th><th>Type</th><th>Size</th><th>Modified</th><th>Actions</th></tr></thead>
                      <tbody>
                        {backups.map((backup) => (
                          <tr key={backup.id}>
                            <td className="backup-id">{backup.id}</td>
                            <td><span className={`pill ${backup.type === 'deleted' ? 'orange' : 'blue'}`}>{backup.type}</span></td>
                            <td>{formatBytes(backup.size)}</td>
                            <td><span className="date-cell"><Clock size={14} />{backup.modified}</span></td>
                            <td>
                              <div className="row-actions">
                                <button className="icon-button green-icon" title="Restore backup" onClick={() => void handleBackupAction(backup, 'restore')}><RotateCcw size={16} /></button>
                                <button className="icon-button red-icon" title="Delete backup" onClick={() => void handleBackupAction(backup, 'delete')}><Trash2 size={16} /></button>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}
          </section>
        )}
      </main>
      {editModalOpen && (
          <div
              className="modal-overlay"
              onClick={() => {
                if (!editSaving) {
                  setEditModalOpen(false)
                  setEditingVHost(null)
                  setEditError('')
                }
              }}
          >
            <div
                className="modal-card"
                onClick={event => event.stopPropagation()}
            >
              <div className="modal-header">
                <div>
                  <h2>Edit VirtualHost</h2>
                  <p>
                    {editingVHost?.domain || 'Loading VirtualHost...'}
                  </p>
                </div>

                <button
                    className="icon-button"
                    type="button"
                    disabled={editSaving}
                    onClick={() => {
                      setEditModalOpen(false)
                      setEditingVHost(null)
                      setEditError('')
                    }}
                >
                  ×
                </button>
              </div>

              {editLoading ? (
                  <div className="edit-loading">
                    <RefreshCw className="spin" size={24} />
                    <p>Loading VirtualHost details...</p>
                  </div>
              ) : editError ? (
                  <div className="alert error">
                    <CircleAlert size={18} />
                    <span>{editError}</span>
                  </div>
              ) : editingVHost ? (
                  <>
                    <form onSubmit={handleSaveEditVHost}>
                      <label>Document Root</label>
                      <input
                          value={editDocumentRoot}
                          onChange={event =>
                              setEditDocumentRoot(event.target.value)
                          }
                          placeholder="/var/www/example.com"
                          required
                          disabled={editSaving}
                      />

                      <label>Server Aliases</label>
                      <textarea
                          value={editServerAliases}
                          onChange={event =>
                              setEditServerAliases(event.target.value)
                          }
                          placeholder={'www.example.com\nblog.example.com'}
                          rows={4}
                          disabled={editSaving}
                      />

                      <p className="modal-hint">
                        Enter one alias per line or separate aliases with spaces or commas.
                      </p>

                      <div className="modal-actions">
                        <button
                            type="button"
                            className="secondary"
                            disabled={editSaving}
                            onClick={() => {
                              setEditModalOpen(false)
                              setEditingVHost(null)
                              setEditError('')
                            }}
                        >
                          Cancel
                        </button>

                        <button
                            type="submit"
                            className="primary"
                            disabled={editSaving}
                        >
                          {editSaving ? 'Saving...' : 'Save Changes'}
                        </button>
                      </div>
                    </form>
                  </>
              ) : null}
            </div>
          </div>
      )}
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export default App