import { mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawn } from 'node:child_process'

import globalSetup from './global-setup.mjs'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '../../..')
const serverDir = path.join(repoRoot, 'server')
const binDir = path.join(serverDir, '.e2e')
const apiBin = path.join(binDir, process.platform === 'win32' ? 'api.exe' : 'api')

function run(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd ?? repoRoot,
      env: options.env ?? process.env,
      shell: false,
      stdio: 'inherit',
    })

    child.on('error', reject)
    child.on('close', (code) => {
      if (code === 0) {
        resolve()
        return
      }
      reject(new Error(`${command} ${args.join(' ')} failed with exit code ${code}`))
    })
  })
}

async function main() {
	await globalSetup()
	mkdirSync(binDir, { recursive: true })
	await run('go', ['build', '-o', apiBin, './cmd/api'], { cwd: serverDir })

	let api
	let fallingBack = false
	const startFallback = (reason) => {
		if (fallingBack) return
		fallingBack = true
		console.warn(`Built API executable could not be started (${reason}); falling back to go run.`)
		api = spawn('go', ['run', './cmd/api'], {
			cwd: serverDir,
			env: process.env,
			stdio: 'inherit',
		})
		api.on('error', (fallbackError) => {
			console.error(fallbackError)
			process.exit(1)
		})
		api.on('exit', (code, signal) => {
			if (signal) process.exit(0)
			process.exit(code ?? 0)
		})
	}

	try {
		api = spawn(apiBin, [], {
			cwd: serverDir,
			env: process.env,
			stdio: 'inherit',
		})
	} catch (error) {
		startFallback(error.code ?? error.message)
	}

	const stop = () => {
		if (api && !api.killed) api.kill()
	}
	process.on('SIGINT', stop)
	process.on('SIGTERM', stop)

	api.on('error', (error) => {
		if (fallingBack) {
			console.error(error)
			process.exit(1)
		}
		startFallback(error.code ?? error.message)
	})
	api.on('exit', (code, signal) => {
		if (fallingBack) return
		if (signal) process.exit(0)
		process.exit(code ?? 0)
	})
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
