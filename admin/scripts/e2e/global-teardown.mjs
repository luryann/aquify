import { existsSync, readFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawn } from 'node:child_process'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '../../..')
const stateFile = path.join(os.tmpdir(), 'aquify-playwright-state.json')

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

export default async function globalTeardown() {
  if (!existsSync(stateFile)) {
    return
  }

  const state = JSON.parse(readFileSync(stateFile, 'utf8'))
  if (state.skipDocker || !state.startedPostgres) {
    return
  }

  await run('docker', ['compose', 'stop', 'postgres'])
}
