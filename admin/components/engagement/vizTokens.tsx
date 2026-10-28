// Readiness colour scale (diverging: low = red, steady = neutral grey,
// high = blue) for wellness charts. Default theme is dark; light values apply
// under [data-theme="light"]. Text never takes these colours.
export const READINESS_VIZ_CSS = `
.readiness-viz {
  --rd-1: #e66767; --rd-2: #8c3f3e; --rd-3: #383835; --rd-4: #1c5cab; --rd-5: #5598e7;
  --rd-empty: rgba(255,255,255,0.04); --rd-line: #5598e7; --rd-grid: rgba(255,255,255,0.08);
}
[data-theme="light"] .readiness-viz {
  --rd-1: #e34948; --rd-2: #f2a6a5; --rd-3: #e4e2de; --rd-4: #9ec5f4; --rd-5: #2a78d6;
  --rd-empty: rgba(0,0,0,0.035); --rd-line: #2a78d6; --rd-grid: rgba(0,0,0,0.07);
}
`

export function readinessFill(step: number | null): string {
  return step ? `var(--rd-${step})` : 'var(--rd-empty)'
}
