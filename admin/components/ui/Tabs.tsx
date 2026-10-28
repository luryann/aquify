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
      className="tab-strip"
      style={{
        display: 'flex',
        gap: '2px',
        // Inset rule instead of a border so the strip can scroll sideways on
        // narrow screens without clipping the active tab's underline.
        boxShadow: 'inset 0 -1px 0 var(--border)',
        marginBottom: '24px',
        overflowX: 'auto',
      }}
    >
      {tabs.map((tab) => {
        const isActive = tab.id === active
        return (
          <button
            key={tab.id}
            onClick={() => onChange(tab.id)}
            style={{
              padding: '8px 16px',
              background: 'none',
              border: 'none',
              borderBottom: isActive ? '2px solid var(--accent)' : '2px solid transparent',
              flexShrink: 0,
              cursor: 'pointer',
              fontSize: 'var(--fs-base)',
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
                  fontSize: 'var(--fs-xs)',
                  fontFamily: 'var(--font-body)',
                  padding: '1px 6px',
                  borderRadius: '12px',
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
