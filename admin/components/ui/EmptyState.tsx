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
  sm: { padding: '32px 20px', iconSize: 28, titleSize: '14px', descSize: '13px' },
  md: { padding: '56px 32px', iconSize: 36, titleSize: '15px', descSize: '13px' },
  lg: { padding: '80px 40px', iconSize: 44, titleSize: '17px', descSize: '14px' },
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
              border: '1px dashed var(--border-strong)',
              borderRadius: 'var(--r-xl)',
              background: 'var(--bg-surface)',
            }
          : {}),
      }}
    >
      {icon && (
        <div
          style={{
            width: `${cfg.iconSize + 20}px`,
            height: `${cfg.iconSize + 20}px`,
            borderRadius: '50%',
            background: 'var(--accent-subtle)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            marginBottom: '16px',
            color: 'var(--accent)',
            flexShrink: 0,
          }}
        >
          <Icon name={icon} size={cfg.iconSize} />
        </div>
      )}

      <p
        style={{
          fontFamily: 'var(--font-display)',
          fontSize: cfg.titleSize,
          color: 'var(--text)',
          marginBottom: description ? '6px' : '0',
          fontWeight: 400,
        }}
      >
        {title}
      </p>

      {description && (
        <p
          style={{
            fontSize: cfg.descSize,
            color: 'var(--text-subtle)',
            maxWidth: '380px',
            lineHeight: 1.6,
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

// Loading state variant
export function LoadingState({ message = 'Loading…' }: { message?: string }) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: '12px',
        padding: '56px 32px',
        color: 'var(--text-subtle)',
      }}
    >
      <div className="spinner spinner-lg" />
      <p style={{ fontSize: '13px' }}>{message}</p>
    </div>
  )
}

// Error state variant
export function ErrorState({
  title = 'Something went wrong',
  description,
  onRetry,
}: {
  title?: string
  description?: string
  onRetry?: () => void
}) {
  return (
    <EmptyState
      icon="alert-circle"
      title={title}
      description={description ?? 'An error occurred. Please try again.'}
      action={
        onRetry ? (
          <button className="btn btn-secondary" onClick={onRetry}>
            <Icon name="refresh-cw" size={13} />
            Try again
          </button>
        ) : undefined
      }
    />
  )
}
