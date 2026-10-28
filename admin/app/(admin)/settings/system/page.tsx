'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchRuntimeSettings, type RuntimeSettingsRecord } from '@/lib/adminData'

function sectionEntries(section?: Record<string, unknown>) {
  return Object.entries(section ?? {}).sort(([a], [b]) => a.localeCompare(b))
}

function formatValue(value: unknown) {
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  if (typeof value === 'number') return String(value)
  if (typeof value === 'string') return value || '—'
  if (value == null) return '—'
  return JSON.stringify(value)
}

export default function SystemDiagnosticsPage() {
  return (
    <AuthGuard requiredRoles={['admin']}>
      <SystemDiagnosticsPageContent />
    </AuthGuard>
  )
}

function SystemDiagnosticsPageContent() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [settings, setSettings] = useState<RuntimeSettingsRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activeCategory, setActiveCategory] = useState('auth')

  useEffect(() => {
    if (!session) return
    let cancelled = false
    setLoading(true)
    setError(null)

    fetchRuntimeSettings(api, session.user.orgId)
      .then((nextSettings) => {
        if (!cancelled) setSettings(nextSettings)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        if (err instanceof ApiError) {
          setError(err.message)
          return
        }
        setError('Couldn’t load runtime settings. Try again.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [api, session])

  const categories = useMemo(() => {
    if (!settings) return []
    return ['auth', 'workers', 'compliance', 'storage', 'stripe', 'postmark', 'telemetry'].filter((category) => settings[category as keyof RuntimeSettingsRecord])
  }, [settings])

  const activeSection = settings?.[activeCategory as keyof RuntimeSettingsRecord] as Record<string, unknown> | undefined

  useEffect(() => {
    if (!categories.length) return
    if (!categories.includes(activeCategory)) {
      setActiveCategory(categories[0])
    }
  }, [activeCategory, categories])

  if (!session) {
    return <div className="u-p-32px" ><EmptyState icon="shield-check" title="Sign in required" description="Authenticate to inspect live runtime settings." /></div>
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">
          System Diagnostics
        </h1>
      </div>

      {error ? (
        <div className="u-p-12px-16px u-mb-20px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      {settings?.app_env && settings.app_env !== 'production' ? (
        <div className="u-p-12px-16px u-mb-20px u-gap-12px" style={{ background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '8px', display: 'flex', alignItems: 'center' }}>
          <Icon name="alert-triangle" size={14} style={{ color: 'var(--warning)' }} />
          <span className="u-fs-fs-sm u-fw-500" style={{ color: 'var(--warning)' }}>
            Backend reports `APP_ENV={settings.app_env}`. Verify production-only hardening before cutover.
          </span>
        </div>
      ) : null}

      <div className="split-nav u-gap-20px" style={{ alignItems: 'start' }}>
        <div className="card u-p-6px" style={{ overflow: 'hidden' }}>
          {categories.map((category) => (
            <button
              key={category}
              onClick={() => setActiveCategory(category)}
              className="u-p-8px-12px u-fs-fs-base u-ff-font-body" style={{ width: '100%', textAlign: 'left', border: 'none', borderRadius: '6px', cursor: 'pointer', background: activeCategory === category ? 'var(--accent-subtle)' : 'transparent', color: activeCategory === category ? 'var(--accent)' : 'var(--text-subtle)', fontWeight: activeCategory === category ? 600 : 400, textTransform: 'capitalize' }}
            >
              {category}
            </button>
          ))}
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
            <h2 className="card-title" style={{ textTransform: 'capitalize' }}>
              {activeCategory} Configuration
            </h2>
          </div>
          {loading ? (
            <div className="u-p-24px" >
              <EmptyState icon="settings" title="Loading settings" description="Fetching the server configuration." />
            </div>
          ) : !activeSection ? (
            <div className="u-p-24px" >
              <EmptyState icon="settings" title="No settings available" description="The server doesn’t expose settings in this category." />
            </div>
          ) : (
            <div>
              {sectionEntries(activeSection).map(([key, value], index, all) => (
                <div key={key} className="u-p-16px-20px u-gap-16px" style={{ borderBottom: index < all.length - 1 ? '1px solid var(--border)' : 'none', display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                  <div>
                    <div className="u-mb-4px" >
                      <code className="u-fs-fs-sm u-ff-font-body u-p-2px-6px" style={{ color: 'var(--text)', background: 'var(--bg-elevated)', borderRadius: '4px', border: '1px solid var(--border)' }}>
                        {key}
                      </code>
                    </div>
                  </div>
                  <code className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)', textAlign: 'right', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                    {formatValue(value)}
                  </code>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
