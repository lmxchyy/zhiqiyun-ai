import { existsSync, mkdirSync } from 'node:fs'
import { dirname, isAbsolute, relative, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const defaultCandidates = [
  'backend-go/docs/openapi.json',
  'backend-go/docs/swagger.json',
  'docs/openapi.json',
  'docs/swagger.json',
]

const candidates = [
  process.env.OPENAPI_SPEC,
  ...defaultCandidates,
].filter(Boolean)

const specRecord = candidates
  .map(path => ({ raw: path, absolute: resolve(path) }))
  .find(candidate => existsSync(candidate.absolute))
const outputRaw = process.env.OPENAPI_TYPES_OUT || 'packages/shared-types/src/generated/openapi.ts'
const outputPath = resolve(outputRaw)
const cliPath = resolve('node_modules/openapi-typescript/bin/cli.js')

if (!specRecord) {
  console.error('No OpenAPI spec found.')
  console.error('Set OPENAPI_SPEC or create one of:')
  for (const candidate of defaultCandidates) {
    console.error(`  - ${candidate}`)
  }
  process.exit(1)
}

mkdirSync(dirname(outputPath), { recursive: true })

function asCliPath(raw, absolute) {
  if (!isAbsolute(raw)) return raw
  const relativePath = relative(process.cwd(), absolute)
  return relativePath && !relativePath.startsWith('..') ? relativePath : absolute
}

const specArg = asCliPath(specRecord.raw, specRecord.absolute)
const outputArg = asCliPath(outputRaw, outputPath)
const result = spawnSync(process.execPath, [cliPath, specArg, '-o', outputArg], {
  stdio: 'inherit',
  shell: false,
})

if (result.error) {
  console.error(result.error.message)
}

process.exit(result.status ?? 1)
