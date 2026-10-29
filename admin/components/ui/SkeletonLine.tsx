import './SkeletonLine.inline.css'

/** Pulsing placeholder line for swimmer-portal loading states. */
export function SkeletonLine({ width = '100%', height = '14px' }: { width?: string; height?: string }) {
  return <div className="skeleton-line" style={{ width, height }} />
}
