'use client'

import { useLayoutEffect, useRef, useState } from 'react'

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
  const stripRef = useRef<HTMLDivElement>(null)
  // null until measured: the active tab keeps its own underline until then,
  // and the indicator never slides in from nowhere on first paint.
  const [bar, setBar] = useState<{ left: number; width: number } | null>(null)

  useLayoutEffect(() => {
    const strip = stripRef.current
    if (!strip) return
    const measure = () => {
      const el = strip.querySelector<HTMLElement>('[data-active="true"]')
      setBar(el ? { left: el.offsetLeft, width: el.offsetWidth } : null)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(strip)
    return () => ro.disconnect()
  }, [active, tabs])

  return (
    <div
      ref={stripRef}
      className="tab-strip"
      style={{
        position: 'relative',
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
          <button type="button"
            key={tab.id}
            data-active={isActive}
            onClick={() => onChange(tab.id)}
            className="u-p-8px-16px u-fs-fs-base u-gap-6px" style={{ background: 'none', border: 'none', borderBottom: isActive && !bar ? '2px solid var(--accent)' : '2px solid transparent', flexShrink: 0, cursor: 'pointer', fontWeight: isActive ? 600 : 400, color: isActive ? 'var(--text)' : 'var(--text-subtle)', display: 'flex', alignItems: 'center', transition: 'color var(--dur-fast)', whiteSpace: 'nowrap' }}
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
                className="u-fs-fs-xs u-ff-font-body u-p-1px-6px u-fw-500" style={{ borderRadius: '12px', background: isActive ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: isActive ? 'var(--accent)' : 'var(--text-muted)', minWidth: '20px', textAlign: 'center' }}
              >
                {tab.count}
              </span>
            )}
          </button>
        )
      })}
      {bar && (
        <span
          aria-hidden
          className="tab-indicator"
          style={{ transform: `translateX(${bar.left}px) scaleX(${bar.width})` }}
        />
      )}
    </div>
  )
}
