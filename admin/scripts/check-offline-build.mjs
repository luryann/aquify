import { readdir, readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const sourceDirs = ['app', 'components', 'lib']
const bannedPatterns = [
  { label: 'next/font/google', regex: /from\s+['"]next\/font\/google['"]/ },
  { label: 'fonts.googleapis.com', regex: /fonts\.googleapis\.com/i },
  { label: 'fonts.gstatic.com', regex: /fonts\.gstatic\.com/i },
]
const textExtensions = new Set(['.ts', '.tsx', '.js', '.jsx', '.mjs', '.css'])

async function collectFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...await collectFiles(fullPath))
      continue
    }
    if (textExtensions.has(path.extname(entry.name))) {
      files.push(fullPath)
    }
  }
  return files
}

const violations = []
for (const sourceDir of sourceDirs) {
  const dir = path.join(root, sourceDir)
  const files = await collectFiles(dir)
  for (const file of files) {
    const contents = await readFile(file, 'utf8')
    for (const banned of bannedPatterns) {
      if (banned.regex.test(contents)) {
        violations.push(`${path.relative(root, file)}: found ${banned.label}`)
      }
    }
  }
}

if (violations.length > 0) {
  console.error('Offline build guard failed. Remove remote font dependencies:')
  for (const violation of violations) {
    console.error(`- ${violation}`)
  }
  process.exit(1)
}

console.log('Offline build guard passed.')
