import type { CSSProperties } from 'react'

type SkeletonProps = {
  width?: string | number
  height?: string | number
  radius?: string | number
  style?: CSSProperties
  className?: string
}

export function Skeleton({ width, height = '16px', radius, style, className }: SkeletonProps) {
  return (
    <div
      className={`skeleton${className ? ` ${className}` : ''}`}
      style={{
        width: width ?? '100%',
        height,
        borderRadius: radius ?? 'var(--r-sm)',
        flexShrink: 0,
        ...style,
      }}
    />
  )
}

// Preset skeleton shapes

export function SkeletonText({ lines = 1, gap = 6 }: { lines?: number; gap?: number }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: `${gap}px` }}>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton
          key={i}
          height="14px"
          width={i === lines - 1 && lines > 1 ? '70%' : '100%'}
        />
      ))}
    </div>
  )
}

export function SkeletonAvatar({ size = 32 }: { size?: number }) {
  return <Skeleton width={size} height={size} radius="50%" style={{ flexShrink: 0 }} />
}

export function SkeletonBadge() {
  return <Skeleton width="64px" height="22px" radius="999px" />
}

type SkeletonTableProps = {
  rows?: number
  columns?: number
  hasHeader?: boolean
}

export function SkeletonTable({ rows = 5, columns = 4, hasHeader = true }: SkeletonTableProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      {hasHeader && (
        <div
          className="u-gap-12px u-p-12px-16px" style={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, borderBottom: '1px solid var(--border)' }}
        >
          {Array.from({ length: columns }).map((_, i) => (
            <Skeleton key={i} height="12px" width="60%" />
          ))}
        </div>
      )}
      {/* Rows */}
      {Array.from({ length: rows }).map((_, rowIdx) => (
        <div
          key={rowIdx}
          className="u-gap-12px u-p-12px-16px" style={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, borderBottom: '1px solid var(--border)', animationDelay: `${rowIdx * 60}ms` }}
        >
          {Array.from({ length: columns }).map((_, colIdx) => (
            <Skeleton
              key={colIdx}
              height="14px"
              width={colIdx === 0 ? '80%' : colIdx === columns - 1 ? '40%' : '65%'}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

export function SkeletonCard({ height = '120px' }: { height?: string }) {
  return (
    <div
      style={{
        background: 'var(--bg-surface)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--r-lg)',
        padding: '20px',
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
        height,
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Skeleton width="40%" height="13px" />
        <Skeleton width="20%" height="22px" radius="999px" />
      </div>
      <Skeleton height="28px" width="55%" />
      <Skeleton height="12px" width="75%" />
    </div>
  )
}

export function SkeletonStatGrid({ count = 4 }: { count?: number }) {
  return (
    <div
      className="u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))' }}
    >
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} />
      ))}
    </div>
  )
}
