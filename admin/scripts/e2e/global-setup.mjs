import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { pathToFileURL } from 'node:url'
import { spawn } from 'node:child_process'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '../../..')
const stateFile = path.join(os.tmpdir(), 'aquify-playwright-state.json')
const databaseUrl = process.env.DATABASE_URL ?? 'postgres://postgres:postgres@127.0.0.1:5432/aquify_e2e?sslmode=disable'
const skipDocker = process.env.PLAYWRIGHT_E2E_SKIP_DOCKER === 'true'

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function run(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd ?? repoRoot,
      env: options.env ?? process.env,
      shell: false,
      stdio: ['ignore', 'pipe', 'pipe'],
    })

    let stdout = ''
    let stderr = ''
    child.stdout.on('data', (chunk) => {
      stdout += chunk.toString()
    })
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString()
    })
    child.on('error', reject)
    child.on('close', (code) => {
      if (code === 0) {
        resolve({ stdout, stderr })
        return
      }
      reject(new Error(`${command} ${args.join(' ')} failed with exit code ${code}\n${stderr || stdout}`))
    })
  })
}

async function commandExists(command) {
  try {
    await run(process.platform === 'win32' ? 'where' : 'sh', process.platform === 'win32' ? [command] : ['-c', `command -v "$1"`, 'sh', command], {
      cwd: repoRoot,
    })
    return true
  } catch {
    return false
  }
}

function e2eDatabaseNames() {
  let parsed
  try {
    parsed = new URL(databaseUrl)
  } catch (error) {
    throw new Error(`DATABASE_URL is not a valid URL: ${error.message}`)
  }

  const targetName = decodeURIComponent(parsed.pathname.replace(/^\/+/, ''))
  if (!targetName) {
    throw new Error('DATABASE_URL must include a database name for Playwright E2E runs.')
  }
  if (!/^[A-Za-z0-9_-]+$/.test(targetName)) {
    throw new Error(`Refusing to create E2E database with unsafe name: ${targetName}`)
  }
  if (!/test|e2e/i.test(targetName)) {
    throw new Error(`Refusing to create/reset non-test E2E database: ${targetName}`)
  }

  const maintenanceUrl = new URL(parsed.toString())
  maintenanceUrl.pathname = '/postgres'
  return { targetName, maintenanceUrl: maintenanceUrl.toString() }
}

async function isPostgresRunning() {
  if (skipDocker) return true
  if (!(await commandExists('docker'))) return false
  try {
    const { stdout } = await run('docker', ['compose', 'ps', '--services', '--filter', 'status=running'])
    return stdout.split(/\r?\n/).map((line) => line.trim()).includes('postgres')
  } catch {
    return false
  }
}

async function ensurePostgres() {
  if (skipDocker) return false
  if (!(await commandExists('docker'))) return false

  const alreadyRunning = await isPostgresRunning()
  if (alreadyRunning) {
    return false
  }

  await run('docker', ['compose', 'up', '-d', 'postgres'])
  for (let attempt = 0; attempt < 60; attempt += 1) {
    try {
      await run('docker', ['compose', 'exec', '-T', 'postgres', 'pg_isready', '-U', 'postgres', '-d', 'aquify'])
      return true
    } catch {
      await delay(1_000)
    }
  }

  throw new Error('Timed out waiting for local Postgres to become ready for Playwright E2E runs.')
}

async function ensureE2EDatabase() {
  const { targetName, maintenanceUrl } = e2eDatabaseNames()

  if (process.env.DATABASE_URL || skipDocker || !(await commandExists('docker'))) {
    if (!(await commandExists('psql'))) {
      throw new Error('Docker is unavailable and psql was not found; install one of them or create the E2E database manually.')
    }

    const { stdout } = await run('psql', [
      '-t',
      '-A',
      '-c',
      `SELECT 1 FROM pg_database WHERE datname = '${targetName.replaceAll("'", "''")}'`,
      maintenanceUrl,
    ])
    if (stdout.trim() === '1') return

    await run('psql', [
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      `CREATE DATABASE "${targetName.replaceAll('"', '""')}"`,
      maintenanceUrl,
    ])
    return
  }

  await run('docker', [
    'compose',
    'exec',
    '-T',
    'postgres',
    'psql',
    '-U',
    'postgres',
    '-d',
    'postgres',
    '-t',
    '-c',
    `SELECT 1 FROM pg_database WHERE datname = '${targetName.replaceAll("'", "''")}'`,
  ]).then(async ({ stdout }) => {
    if (stdout.trim() === '1') return
    await run('docker', [
      'compose',
      'exec',
      '-T',
      'postgres',
      'createdb',
      '-U',
      'postgres',
      targetName,
    ])
  })
}

async function migrateDatabase() {
  await run(
    'go',
    ['run', './cmd/e2e-reset-db'],
    {
      cwd: path.join(repoRoot, 'server'),
      env: {
        ...process.env,
        DATABASE_URL: databaseUrl,
        E2E_ALLOW_DB_RESET: 'true',
      },
    },
  )

  await applyMigrationsWithPsql()
}

function extractGooseUp(sql, filename) {
  const up = sql.match(/^\s*--\s*\+goose\s+Up\s*$/im)
  if (!up) {
    throw new Error(`${filename} is missing a -- +goose Up marker.`)
  }

  const afterUp = sql.slice(up.index + up[0].length)
  const down = afterUp.match(/^\s*--\s*\+goose\s+Down\s*$/im)
  return down ? afterUp.slice(0, down.index) : afterUp
}

async function applyMigrationsWithPsql() {
  if (!(await commandExists('psql'))) {
    throw new Error('psql is required to apply E2E database migrations.')
  }

  const migrationsDir = path.join(repoRoot, 'db', 'migrations')
  const migrationFiles = readdirSync(migrationsDir)
    .filter((file) => file.endsWith('.sql'))
    .sort()

  const tempDir = mkdtempSync(path.join(os.tmpdir(), 'aquify-e2e-migrations-'))
  try {
    for (const filename of migrationFiles) {
      const sourcePath = path.join(migrationsDir, filename)
      const upSql = extractGooseUp(readFileSync(sourcePath, 'utf8'), filename)
      const tempPath = path.join(tempDir, filename)
      writeFileSync(tempPath, upSql, 'utf8')
      await run('psql', [
        '-v',
        'ON_ERROR_STOP=1',
        '-f',
        tempPath,
        databaseUrl,
      ])
    }
  } finally {
    rmSync(tempDir, { recursive: true, force: true })
  }
}

export default async function globalSetup() {
  const startedPostgres = await ensurePostgres()
  await ensureE2EDatabase()
  await migrateDatabase()
  writeFileSync(stateFile, JSON.stringify({ startedPostgres, skipDocker }), 'utf8')
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  globalSetup().catch((error) => {
    console.error(error)
    process.exit(1)
  })
}
