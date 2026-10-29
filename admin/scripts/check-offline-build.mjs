import { readdir, readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const sourceDirs = ['app', 'components', 'lib', 'public']
// Config files live at the package root; `.next/static` is the build output
// (checked only when a build exists, so the guard also covers what ships).
const rootFilePattern = /^(next\.config\..+|postcss\.config\..+|tailwind\.config\..+)$/
const buildOutputDirs = [path.join('.next', 'static')]
const remoteFontHosts = [
  'fonts.googleapis.com', 'fonts.gstatic.com', 'use.typekit.net', 'fonts.bunny.net',
  'fast.fonts.net', 'use.fontawesome.com', 'kit.fontawesome.com',
]
const bannedPatterns = [
  { label: 'next/font/google', regex: /from\s+['"]next\/font\/google['"]/ },
  ...remoteFontHosts.map((host) => ({ label: host, regex: new RegExp(host.replaceAll('.', '\\.'), 'i') })),
  // CSS @import of any remote stylesheet (could pull in fonts from an unlisted host).
  { label: 'remote CSS @import', regex: /@import\s+(?:url\(\s*)?['"]?(?:https?:)?\/\//i },
  // Remote font files referenced from @font-face / preload links.
  { label: 'remote font file', regex: /(?:https?:)?\/\/[^\s'")]+\.(?:woff2?|ttf|otf|eot)\b/i },
]
const textExtensions = new Set(['.ts', '.tsx', '.js', '.jsx', '.mjs', '.cjs', '.css', '.html', '.json', '.webmanifest', '.svg'])

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

async function existingFiles(dir) {
  try {
    return await collectFiles(dir)
  } catch (error) {
    if (error?.code === 'ENOENT') return []
    throw error
  }
}

const targets = []
for (const sourceDir of [...sourceDirs, ...buildOutputDirs]) {
  targets.push(...await existingFiles(path.join(root, sourceDir)))
}
for (const entry of await readdir(root, { withFileTypes: true })) {
  if (entry.isFile() && rootFilePattern.test(entry.name)) targets.push(path.join(root, entry.name))
}

const violations = []
{
  for (const file of targets) {
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
