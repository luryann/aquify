'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
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

export default function SettingsPage() {
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
        setError('Unable to load runtime settings.')
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
    return <div style={{ padding: '32px' }}><EmptyState icon="shield-check" title="Sign in required" description="Authenticate to inspect live runtime settings." /></div>
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1000px' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          System
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
          Runtime Settings
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Sanitized configuration values returned by the live backend for this environment.
        </p>
      </div>

      {error ? (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {settings?.app_env && settings.app_env !== 'production' ? (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '8px', display: 'flex', gap: '10px', alignItems: 'center' }}>
          <Icon name="alert-triangle" size={14} style={{ color: 'var(--warning)' }} />
          <span style={{ fontSize: '12px', color: 'var(--warning)', fontWeight: 500 }}>
            Backend reports `APP_ENV={settings.app_env}`. Verify production-only hardening before cutover.
          </span>
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: '220px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ overflow: 'hidden', padding: '6px' }}>
          {categories.map((category) => (
            <button
              key={category}
              onClick={() => setActiveCategory(category)}
              style={{
                width: '100%',
                padding: '9px 12px',
                textAlign: 'left',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                background: activeCategory === category ? 'var(--accent-subtle)' : 'transparent',
                color: activeCategory === category ? 'var(--accent)' : 'var(--text-subtle)',
                fontSize: '13px',
                fontWeight: activeCategory === category ? 600 : 400,
                fontFamily: 'var(--font-body)',
                textTransform: 'capitalize',
              }}
            >
              {category}
            </button>
          ))}
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
            <h2 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textTransform: 'capitalize' }}>
              {activeCategory} Configuration
            </h2>
          </div>
          {loading ? (
            <div style={{ padding: '24px' }}>
              <EmptyState icon="settings" title="Loading settings" description="Fetching sanitized runtime configuration from the backend." />
            </div>
          ) : !activeSection ? (
            <div style={{ padding: '24px' }}>
              <EmptyState icon="settings" title="No settings available" description="This runtime category is not exposed by the backend." />
            </div>
          ) : (
            <div>
              {sectionEntries(activeSection).map(([key, value], index, all) => (
                <div key={key} style={{ padding: '16px 18px', borderBottom: index < all.length - 1 ? '1px solid var(--border)' : 'none', display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px' }}>
                  <div>
                    <div style={{ marginBottom: '4px' }}>
                      <code style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text)', background: 'var(--bg-elevated)', padding: '2px 6px', borderRadius: '4px', border: '1px solid var(--border)' }}>
                        {key}
                      </code>
                    </div>
                  </div>
                  <code style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', textAlign: 'right', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
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
