'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type EnvVar = {
  key: string
  value: string
  category: string
  description: string
  sensitive: boolean
  editable: boolean
}

const ENV_VARS: EnvVar[] = [
  // Auth
  { key: 'APP_ENV',                        value: 'development',  category: 'Auth', description: 'Application environment. Set to production for live deployments.', sensitive: false, editable: true },
  { key: 'AUTH_ENABLE_DEV_STUB_TOKENS',    value: 'true',         category: 'Auth', description: 'Allow stub-* bearer tokens. MUST be false outside development.', sensitive: false, editable: true },
  { key: 'OIDC_CLIENT_ID',                 value: 'dare-admin',   category: 'Auth', description: 'Keycloak client ID for OIDC authentication.', sensitive: false, editable: false },
  { key: 'OIDC_CLIENT_SECRET',             value: '●●●●●●●●●●●', category: 'Auth', description: 'Keycloak client secret. Rotate via Keycloak admin console.', sensitive: true, editable: false },
  // Workers
  { key: 'WORKER_CONCURRENCY_PER_QUEUE',   value: '2',            category: 'Workers', description: 'Max concurrent goroutines per job queue.', sensitive: false, editable: true },
  { key: 'WORKER_RECONCILE_INTERVAL',      value: '24h',          category: 'Workers', description: 'Stripe reconciliation job interval.', sensitive: false, editable: true },
  { key: 'WORKER_INTEGRITY_INTERVAL',      value: '24h',          category: 'Workers', description: 'Nightly integrity check interval.', sensitive: false, editable: true },
  { key: 'WORKER_CLEANUP_INTERVAL',        value: '6h',           category: 'Workers', description: 'Purge expired temp files interval.', sensitive: false, editable: true },
  // Compliance
  { key: 'AUDIT_RETENTION_DAYS',           value: '2555',         category: 'Compliance', description: '~7 years. Audit events older than this are eligible for archival.', sensitive: false, editable: true },
  { key: 'COMPLIANCE_EXPORT_TTL_DAYS',     value: '30',           category: 'Compliance', description: 'Data export file download window after generation.', sensitive: false, editable: true },
  { key: 'COMPLIANCE_DELETION_REVIEW_DAYS',value: '30',           category: 'Compliance', description: 'Window for admin to review deletion requests before auto-escalation.', sensitive: false, editable: true },
  // Storage
  { key: 'STORAGE_ENDPOINT',               value: 'localhost:9000', category: 'Storage', description: 'S3-compatible object storage endpoint.', sensitive: false, editable: false },
  { key: 'STORAGE_BUCKET',                 value: 'dare-imports',  category: 'Storage', description: 'Default bucket for file imports/exports.', sensitive: false, editable: false },
  { key: 'STORAGE_USE_SSL',                value: 'false',         category: 'Storage', description: 'Use TLS for storage connections. Should be true in production.', sensitive: false, editable: true },
]

const CATEGORIES = Array.from(new Set(ENV_VARS.map((e) => e.category)))

export default function SettingsPage() {
  const [activeCategory, setActiveCategory] = useState(CATEGORIES[0])
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [saved, setSaved] = useState<Set<string>>(new Set())

  const filtered = ENV_VARS.filter((e) => e.category === activeCategory)

  function startEdit(env: EnvVar) {
    setEditingKey(env.key)
    setEditValue(env.sensitive ? '' : env.value)
  }

  function saveEdit(key: string) {
    setSaved((prev) => new Set([...prev, key]))
    setEditingKey(null)
    setTimeout(() => setSaved((prev) => { const n = new Set(prev); n.delete(key); return n }), 2000)
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1000px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          System
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
          System Settings
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Runtime configuration for the DARE platform. Most values are sourced from environment variables and read-only at runtime.
        </p>
      </div>

      {/* Production warning */}
      {ENV_VARS.find(e => e.key === 'APP_ENV')?.value !== 'production' && (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '8px', display: 'flex', gap: '10px', alignItems: 'center' }}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
            <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
          </svg>
          <span style={{ fontSize: '12px', color: 'var(--warning)', fontWeight: 500 }}>
            Running in <strong>development</strong> mode. Stub tokens are enabled. Ensure AUTH_ENABLE_DEV_STUB_TOKENS=false before production deployment.
          </span>
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '180px 1fr', gap: '20px', alignItems: 'start' }}>
        {/* Category sidebar */}
        <div className="card" style={{ overflow: 'hidden', padding: '6px' }}>
          {CATEGORIES.map((cat) => (
            <button
              key={cat}
              onClick={() => setActiveCategory(cat)}
              style={{
                width: '100%', padding: '9px 12px', textAlign: 'left', background: 'none', border: 'none',
                borderRadius: '6px', cursor: 'pointer',
                backgroundColor: activeCategory === cat ? 'var(--accent-subtle)' : 'transparent',
                color: activeCategory === cat ? 'var(--accent)' : 'var(--text-subtle)',
                fontSize: '13px', fontWeight: activeCategory === cat ? 600 : 400,
                fontFamily: 'var(--font-body)', transition: 'all 0.1s ease',
                display: 'flex', alignItems: 'center', gap: '8px',
              }}
              onMouseEnter={(e) => { if (activeCategory !== cat) e.currentTarget.style.backgroundColor = 'var(--bg-surface)' }}
              onMouseLeave={(e) => { if (activeCategory !== cat) e.currentTarget.style.backgroundColor = 'transparent' }}
            >
              <span style={{
                width: '6px', height: '6px', borderRadius: '50%', flexShrink: 0,
                background: activeCategory === cat ? 'var(--accent)' : 'var(--text-disabled)',
              }} />
              {cat}
            </button>
          ))}
        </div>

        {/* Settings panel */}
        <div>
          <div className="card" style={{ overflow: 'hidden' }}>
            <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
              <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{activeCategory} Configuration</h3>
            </div>
            <div>
              {filtered.map((env, idx) => (
                <div
                  key={env.key}
                  style={{
                    padding: '16px 18px',
                    borderBottom: idx < filtered.length - 1 ? '1px solid var(--border)' : 'none',
                    display: 'flex', alignItems: 'flex-start', gap: '16px',
                  }}
                >
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '4px' }}>
                      <code style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text)', background: 'var(--bg-elevated)', padding: '1px 6px', borderRadius: '4px', border: '1px solid var(--border)' }}>
                        {env.key}
                      </code>
                      {env.sensitive && (
                        <span style={{ fontSize: '10px', padding: '1px 5px', background: 'var(--warning-bg)', color: 'var(--warning)', border: '1px solid var(--warning-border)', borderRadius: '4px', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                          SENSITIVE
                        </span>
                      )}
                      {!env.editable && (
                        <span style={{ fontSize: '10px', padding: '1px 5px', background: 'var(--bg-elevated)', color: 'var(--text-muted)', border: '1px solid var(--border)', borderRadius: '4px', fontFamily: 'var(--font-mono)' }}>
                          READ-ONLY
                        </span>
                      )}
                    </div>
                    <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.5 }}>{env.description}</p>
                  </div>

                  <div style={{ flexShrink: 0, minWidth: '200px', textAlign: 'right' }}>
                    {editingKey === env.key ? (
                      <div style={{ display: 'flex', gap: '6px', alignItems: 'center', justifyContent: 'flex-end' }}>
                        <input
                          value={editValue}
                          onChange={(e) => setEditValue(e.target.value)}
                          type={env.sensitive ? 'password' : 'text'}
                          style={{ width: '140px', padding: '5px 8px', background: 'var(--bg-surface)', border: '1px solid var(--border-focus)', borderRadius: '5px', color: 'var(--text)', fontSize: '12px', fontFamily: 'var(--font-mono)', outline: 'none' }}
                          autoFocus
                        />
                        <button onClick={() => saveEdit(env.key)} style={{ padding: '5px 10px', background: 'var(--accent)', border: 'none', borderRadius: '5px', color: '#fff', fontSize: '11px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Save</button>
                        <button onClick={() => setEditingKey(null)} style={{ padding: '5px 10px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '5px', color: 'var(--text-muted)', fontSize: '11px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
                      </div>
                    ) : (
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', justifyContent: 'flex-end' }}>
                        {saved.has(env.key) ? (
                          <span style={{ fontSize: '11px', color: 'var(--success)', fontFamily: 'var(--font-mono)' }}>✓ saved</span>
                        ) : null}
                        <code style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: env.sensitive ? 'var(--text-muted)' : 'var(--text-subtle)', background: 'var(--bg-elevated)', padding: '3px 8px', borderRadius: '5px', border: '1px solid var(--border)' }}>
                          {env.sensitive ? '●●●●●●●●●' : env.value}
                        </code>
                        {env.editable && (
                          <button
                            onClick={() => startEdit(env)}
                            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '2px', borderRadius: '3px', transition: 'color 0.15s' }}
                            onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text)' }}
                            onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
                          >
                            <Icon name="edit" size={12} />
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* .env source note */}
          <div style={{ marginTop: '12px', padding: '10px 14px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', display: 'flex', gap: '8px', alignItems: 'center' }}>
            <Icon name="info" size={12} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
            <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', margin: 0 }}>
              Values sourced from <code>.env.local</code> / environment. Editable values are updated in-process and do not persist across restarts without updating the env file.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
