import './ShimmerBlock.inline.css'

type ShimmerBlockProps = { w?: string | number; h?: number; r?: number }

/** Animated placeholder block for portal loading states. */
export function ShimmerBlock({ w = '100%', h = 16, r = 8 }: ShimmerBlockProps) {
  return <div className="shimmer-block" style={{ width: w, height: h, borderRadius: r }} />
}
