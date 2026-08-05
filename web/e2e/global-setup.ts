import { execSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PROJECT_ROOT = resolve(__dirname, '../..')
const IMAGE_NAME = 'openposterdb-test'
const CONTAINER_NAME = 'openposterdb-test'
const BACKEND_URL = 'http://127.0.0.1:3333/api/auth/status'

/** Parse a .env file and return key-value pairs (ignores comments and blank lines). */
function parseEnvFile(path: string): Record<string, string> {
  const vars: Record<string, string> = {}
  try {
    const content = readFileSync(path, 'utf-8')
    for (const line of content.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed || trimmed.startsWith('#')) continue
      const eqIdx = trimmed.indexOf('=')
      if (eqIdx === -1) continue
      const key = trimmed.slice(0, eqIdx).trim()
      let value = trimmed.slice(eqIdx + 1).trim()
      // Strip surrounding quotes (single or double)
      if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
        value = value.slice(1, -1)
      }
      vars[key] = value
    }
  } catch {
    // .env file not found — use defaults
  }
  return vars
}

function containerCmd(): string {
  try {
    execSync('podman --version', { stdio: 'ignore' })
    return 'podman'
  } catch {
    return 'docker'
  }
}

export default async function globalSetup() {
  const cmd = containerCmd()

  // Read real API keys from api/.env (falls back to dummy values if not present)
  const envFile = parseEnvFile(resolve(PROJECT_ROOT, 'api/.env'))

  const tmdbKey = envFile.TMDB_API_KEY || 'test'
  const omdbKey = envFile.OMDB_API_KEY || ''
  const mdblistKey = envFile.MDBLIST_API_KEY || 'test'
  const fanartKey = envFile.FANART_API_KEY || 'test'
  const jwtSecret = envFile.JWT_SECRET || 'abababababababababababababababababababababababababababababababab'

  const hasRealKeys = tmdbKey !== 'test'
  if (hasRealKeys) {
    console.log('[e2e] Using real API keys from api/.env')
  } else {
    console.log('[e2e] No api/.env found or TMDB_API_KEY not set — using dummy keys')
  }

  console.log(`[e2e] Building container image via "${cmd}"...`)
  execSync(
    `${cmd} build -t ${IMAGE_NAME} --build-arg CARGO_FEATURES=test-support -f Dockerfile .`,
    { cwd: PROJECT_ROOT, stdio: 'inherit' },
  )

  console.log('[e2e] Starting container...')
  try { execSync(`${cmd} rm -f ${CONTAINER_NAME}`, { stdio: 'ignore' }) } catch { /* ignore */ }

  const envFlags = [
    `-e TMDB_API_KEY=${tmdbKey}`,
    `-e MDBLIST_API_KEY=${mdblistKey || 'test'}`,
    `-e JWT_SECRET=${jwtSecret}`,
    '-e LISTEN_ADDR=0.0.0.0:3000',
    '-e COOKIE_SECURE=false',
    `-e FANART_API_KEY=${fanartKey}`,
    '-e CACHE_DIR=/tmp/openposterdb-e2e',
    ...(omdbKey ? [`-e OMDB_API_KEY=${omdbKey}`] : []),
  ]

  execSync(
    [
      `${cmd} run -d --name ${CONTAINER_NAME}`,
      '-p 3333:3000',
      '--tmpfs /tmp/openposterdb-e2e',
      ...envFlags,
      IMAGE_NAME,
    ].join(' '),
    { cwd: PROJECT_ROOT, stdio: 'inherit' },
  )

  console.log('[e2e] Waiting for backend to be ready...')
  const deadline = Date.now() + 60_000
  while (Date.now() < deadline) {
    try {
      const res = await fetch(BACKEND_URL)
      if (res.ok) {
        console.log('[e2e] Backend is ready')
        return
      }
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error('Backend did not start within 60 seconds')
}
