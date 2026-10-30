import { Skeleton, SkeletonTable } from './Skeleton'
import './PageSkeleton.css'

export type PageSkeletonVariant = 'table' | 'list' | 'cards' | 'detail' | 'form' | 'dashboard'

type PageSkeletonProps = {
  variant?: PageSkeletonVariant
  /** Hide the title/actions header block (for panels nested in an existing page). */
  noHeader?: boolean
  rows?: number
  columns?: number
  label?: string
}

function Header() {
  return (
    <div className="page-header">
      <div className="ps-stack">
        <Skeleton width="min(280px, 60%)" height="28px" />
        <Skeleton width="min(420px, 80%)" height="14px" />
      </div>
      <Skeleton width="120px" height="36px" radius="var(--r-md, 8px)" />
    </div>
  )
}

function CardBlock({ lines = 3 }: { lines?: number }) {
  return (
    <div className="card ps-card">
      <Skeleton width="45%" height="16px" />
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} height="12px" width={i === lines - 1 ? '60%' : '100%'} />
      ))}
    </div>
  )
}

function Body({ variant, rows, columns }: { variant: PageSkeletonVariant; rows: number; columns: number }) {
  switch (variant) {
    case 'table':
      return (
        <div className="card" style={{ overflow: 'hidden' }}>
          <SkeletonTable rows={rows} columns={columns} />
        </div>
      )
    case 'list':
      return (
        <div className="card" style={{ overflow: 'hidden' }}>
          {Array.from({ length: rows }).map((_, i) => (
            <div className="ps-row" key={i}>
              <Skeleton width="36px" height="36px" radius="50%" />
              <div className="ps-stack" style={{ flex: 1, gap: 'var(--space-2)' }}>
                <Skeleton width="40%" height="14px" />
                <Skeleton width="65%" height="12px" />
              </div>
              <Skeleton width="64px" height="22px" radius="999px" />
            </div>
          ))}
        </div>
      )
    case 'cards':
      return (
        <div className="ps-grid">
          {Array.from({ length: 6 }).map((_, i) => (
            <CardBlock key={i} lines={3} />
          ))}
        </div>
      )
    case 'dashboard':
      return (
        <>
          <div className="ps-stats">
            {Array.from({ length: 4 }).map((_, i) => (
              <div className="card ps-card" key={i}>
                <Skeleton width="50%" height="12px" />
                <Skeleton width="40%" height="30px" />
              </div>
            ))}
          </div>
          <div className="ps-split" style={{ marginTop: 'var(--space-4)' }}>
            <CardBlock lines={6} />
            <CardBlock lines={4} />
          </div>
        </>
      )
    case 'form':
      return (
        <div className="card ps-card">
          <div className="ps-fields">
            {Array.from({ length: 6 }).map((_, i) => (
              <div className="ps-stack" key={i} style={{ gap: 'var(--space-2)' }}>
                <Skeleton width="35%" height="12px" />
                <Skeleton height="38px" radius="var(--r-md, 8px)" />
              </div>
            ))}
          </div>
          <Skeleton width="140px" height="38px" radius="var(--r-md, 8px)" />
        </div>
      )
    case 'detail':
    default:
      return (
        <div className="ps-split">
          <div className="ps-stack">
            <CardBlock lines={5} />
            <div className="card" style={{ overflow: 'hidden' }}>
              <SkeletonTable rows={Math.min(rows, 4)} columns={Math.min(columns, 4)} />
            </div>
          </div>
          <div className="ps-stack">
            <CardBlock lines={4} />
            <CardBlock lines={3} />
          </div>
        </div>
      )
  }
}

export function PageSkeleton({ variant = 'table', noHeader = false, rows = 6, columns = 5, label = 'Loading' }: PageSkeletonProps) {
  return (
    <div className={noHeader ? undefined : 'page'} aria-busy="true" aria-live="polite" role="status">
      <span className="sr-only">{label}</span>
      <div aria-hidden="true">
        {!noHeader && <Header />}
        <Body variant={variant} rows={rows} columns={columns} />
      </div>
    </div>
  )
}

export default PageSkeleton
