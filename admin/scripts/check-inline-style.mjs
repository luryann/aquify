// Non-blocking report: counts inline style={{ ... }} props that set spacing or typography.
// Usage: npm run check:inline-style [-- --strict]  (strict exits 1 when any are found)
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const ROOTS = ['app', 'components']
const PROP = /\b(padding(?:Top|Right|Bottom|Left)?|margin(?:Top|Right|Bottom|Left)?|gap|fontSize|fontWeight|fontFamily|lineHeight|letterSpacing)\s*:/
const STYLE = /style=\{\{([^}]*(?:\{[^}]*\}[^}]*)*)\}\}/g

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (p.endsWith('.tsx')) out.push(p)
  }
  return out
}

const perFile = []
let total = 0
for (const root of ROOTS) {
  for (const file of walk(root)) {
    const src = readFileSync(file, 'utf8')
    let count = 0
    for (const m of src.matchAll(STYLE)) if (PROP.test(m[1])) count++
    if (count) { perFile.push([count, relative('.', file)]); total += count }
  }
}
perFile.sort((a, b) => b[0] - a[0])
console.log(`Inline style props setting spacing/typography: ${total} across ${perFile.length} files`)
for (const [c, f] of perFile.slice(0, 15)) console.log(`  ${String(c).padStart(4)}  ${f}`)
console.log('Prefer .stack/.row/.card utilities and --space-* / --fs-* tokens from globals.css.')
if (process.argv.includes('--strict') && total > 0) process.exit(1)
