import type { ReactNode } from 'react'
import type { IconName } from './icons'
import { Icon } from './icons'

type EmptyStateProps = {
  icon?: IconName
  title: string
  description?: string
  action?: ReactNode
  size?: 'sm' | 'md' | 'lg'
  bordered?: boolean
}

const SIZE_CONFIG = {
  sm: { padding: '24px 20px', iconSize: 24, titleSize: 'var(--fs-md)', descSize: 'var(--fs-sm)' },
  md: { padding: '32px 24px', iconSize: 28, titleSize: 'var(--fs-lg)', descSize: 'var(--fs-base)' },
  lg: { padding: '40px 32px', iconSize: 36, titleSize: 'var(--fs-xl)', descSize: 'var(--fs-base)' },
}

export function EmptyState({
  icon,
  title,
  description,
  action,
  size = 'md',
  bordered = true,
}: EmptyStateProps) {
  const cfg = SIZE_CONFIG[size]

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        textAlign: 'center',
        padding: cfg.padding,
        ...(bordered
          ? {
              border: '1px dashed var(--border)',
              borderRadius: 'var(--r-xl)',
            }
          : {}),
      }}
    >
      {icon && (
        <div
          className="u-mb-8px" style={{ display: 'flex', color: 'var(--text-muted)', flexShrink: 0 }}
        >
          <Icon name={icon} size={cfg.iconSize} />
        </div>
      )}

      <p
        className="u-ff-font-display u-fw-display-weight" style={{ fontSize: cfg.titleSize, color: 'var(--text)', marginBottom: description ? '6px' : '0' }}
      >
        {title}
      </p>

      {description && (
        <p
          style={{
            fontSize: cfg.descSize,
            color: 'var(--text-subtle)',
            maxWidth: '48ch',
            lineHeight: 'var(--leading-body)',
            marginBottom: action ? '20px' : '0',
          }}
        >
          {description}
        </p>
      )}

      {action && <div style={{ marginTop: !description ? '16px' : '0' }}>{action}</div>}
    </div>
  )
}
