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
