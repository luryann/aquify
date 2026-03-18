'use client'

type Tab = {
  id: string
  label: string
  count?: number
}

type TabsProps = {
  tabs: Tab[]
  active: string
  onChange: (id: string) => void
}

export function Tabs({ tabs, active, onChange }: TabsProps) {
  return (
    <div
      style={{
        display: 'flex',
        gap: '2px',
        borderBottom: '1px solid var(--border)',
        marginBottom: '24px',
      }}
    >
      {tabs.map((tab) => {
        const isActive = tab.id === active
        return (
          <button
            key={tab.id}
            onClick={() => onChange(tab.id)}
            style={{
              padding: '8px 14px',
              background: 'none',
              border: 'none',
              borderBottom: isActive ? '2px solid var(--accent)' : '2px solid transparent',
              marginBottom: '-1px',
              cursor: 'pointer',
              fontSize: '13px',
              fontWeight: isActive ? 600 : 400,
              color: isActive ? 'var(--text)' : 'var(--text-subtle)',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              transition: 'color var(--dur-fast)',
              whiteSpace: 'nowrap',
            }}
            onMouseEnter={(e) => {
              if (!isActive) e.currentTarget.style.color = 'var(--text)'
            }}
            onMouseLeave={(e) => {
              if (!isActive) e.currentTarget.style.color = 'var(--text-subtle)'
            }}
          >
            {tab.label}
            {tab.count !== undefined && (
              <span
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  padding: '1px 6px',
                  borderRadius: '10px',
                  background: isActive ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                  color: isActive ? 'var(--accent)' : 'var(--text-muted)',
                  fontWeight: 500,
                  minWidth: '20px',
                  textAlign: 'center',
                }}
              >
                {tab.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
