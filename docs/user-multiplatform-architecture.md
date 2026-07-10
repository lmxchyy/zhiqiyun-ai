# User multiplatform architecture

This repository keeps the PC admin and Go backend stable while serving the user client from one reusable multiplatform core.

## Stable surfaces

- `admin-vue`: PC admin, Vue 3 + Element Plus + Pinia + Axios.
- `backend-go`: Go + Gin + Gorm-compatible data layer + Redis + PostgreSQL/MySQL deployment path.
- `apps/user-uni`: the only user uni-app entry for H5, WeChat mini program, App Plus, and Harmony build targets. Docker serves its H5 output through the Go service.

Deployment stays Docker-first. The optimized user-client core is compiled from `apps/user-uni` into H5 static artifacts and served by the existing Go container; desktop wrappers consume the same H5 output instead of introducing another business-code deployment path.

## Shared user-client core

```text
packages/
  shared-types/       API/domain contracts shared by user clients
  platform-adapter/   web, WeChat mini program, App, Harmony, desktop runtime differences
  api-client/         auth header, base URL, timeout, error/envelope handling
  shared-auth/        password login, WeChat mini program login, refresh token, session restore
  business-sdk/       domain SDKs for dashboard, generation, assets, billing, membership, agents
  design-token/       TS tokens and SCSS/CSS variables
```

Each shared package has its own `package.json` and `tsconfig.json`. The root workspace builds them with:

```powershell
npm.cmd run build:packages
```

The intended dependency direction is:

```text
pages / Pinia stores
  -> business-sdk / shared-auth
  -> api-client
  -> platform-adapter
  -> uni.request / fetch / desktop bridge
```

Pages should not directly encode platform-specific login, storage, request, or download behavior. Add those differences in `platform-adapter` first, then expose business methods through `business-sdk`.

For the H5 surface, the rule is:

```text
auth / token / session      -> shared-auth via apps/user-uni/src/api/client.ts
list tasks/assets/models    -> business-sdk
points and channel reads    -> business-sdk
temporary mutations         -> keep api(path, init) wrapper until a domain SDK method exists
```

## Current integration state

- `apps/user-uni/src/api/client.ts` keeps the old `api(path, init)` signature while using shared `api-client` and `platform-adapter` underneath.
- `apps/user-uni/src/api/client.ts` also exports `authStorage`, `authService`, and `businessSdk`; H5 pages should use those for auth and common user-domain reads before adding new page-local API calls.
- `apps/user-uni/src/types.ts` re-exports `shared-types` so page-level imports stay stable.

## Target app layout

The current structural target is:

```text
apps/
  admin-web/          current admin-vue, can remain as-is or be moved later
  user-uni/           single uni-app / progressive uni-app x user client
  desktop-tauri/      wraps user-uni H5 output
  desktop-electron/   optional wrapper for the same H5 output
```

Do not rewrite desktop business logic. Tauri/Electron should only package the H5 build and expose optional desktop bridge APIs through `platform-adapter`.

`apps/user-uni` is the canonical user-client app directory. Mini program, App, Harmony, and H5 build commands all converge on this directory.

## Platform policy

- Web/H5: same-origin `/api` in production, dev proxy in local H5.
- WeChat mini program: absolute API base URL via `VITE_API_BASE_URL`/`VITE_SERVER_BASEURL`; `uni.login({ provider: 'weixin' })` lives behind `shared-auth`.
- Android/iOS/Harmony App: use the same `api-client` and add native-only behavior in `platform-adapter`.
- Desktop: package H5 output and add file-system/window features only through the adapter. Tauri/Electron bridge globals are detected before plain H5 so the client reports `desktop` instead of `web` inside a desktop shell.

## Runtime capability and request tracing

`platform-adapter` is now the only place that should know how to call platform APIs such as upload, download, payment, share, file selection, clipboard, QR scan, URL opening, login, storage, and request. If a capability is not available on a runtime, the adapter rejects with an explicit unsupported error instead of silently pretending it worked.

`api-client` adds these headers to every shared request:

```text
X-Request-Id
X-Client-Platform
X-Client-Name
X-Client-Version
X-Client-Language
```

If H5 connects to the backend cross-origin instead of same-origin/proxy, set the backend environment variable:

```text
XIANZHI_CORS_ALLOWED_ORIGINS=https://app.example.com,https://desktop.example.com
```

Keep it empty for same-origin deployments. The Go middleware allows only exact origins from this comma-separated list, plus the shared request headers and `Authorization`/`Content-Type`.

Both current user entries clear their local access token/session on `401` responses through `api-client.onUnauthorized`, while preserving a stored refresh token for `shared-auth.restore()` recovery. If refresh fails, `shared-auth` clears the whole local auth state.

The Go API now exposes:

```text
POST /api/v1/auth/refresh
```

In Redis-backed Docker deployments, login returns both `accessToken` and `refreshToken`. Refresh tokens are stored with a separate Redis session key and are rotated when used.

The Go API server mirrors `X-Request-Id` back on every response. If the client does not provide one, the server generates one and exposes it through `Access-Control-Expose-Headers: X-Request-Id` so H5 clients can read it when CORS is configured.

`api-client` throws `ApiClientError` for network, HTTP, and API-envelope failures. It still behaves as a normal `Error`, but carries `path`, `statusCode`, `requestId`, optional `apiCode`, and raw `payload` for user support and backend log correlation.

`business-sdk` is intentionally split by domain:

```text
packages/business-sdk/src/
  auth.ts
  dashboard.ts
  generation.ts
  assets.ts
  billing.ts
  membership.ts
  agents.ts
  role-workbench.ts
  models.ts
  points.ts
  mappers.ts
  types.ts
  index.ts
```

The public entry stays `createBusinessSdk(api)`; pages should keep using the composed SDK instead of importing internal domain modules directly.

`billing.ts` owns wallet, points account, plan list, recharge order, subscription order, regular commerce order, agent join order, and operation-center join order calls.

`role-workbench.ts` owns the H5 mini-program-style role workbench reads: member profile, wallet/points account, recent assets, channel center, and operation-center overview lists. Page-local direct API calls should now be limited to mutations that still need a domain SDK method, such as child-agent creation and withdrawal requests.

## Verification

Run these before promoting more pages to the shared core:

```powershell
npm.cmd run build:packages
npm.cmd run generate:api-types
npm.cmd run typecheck:user-core
npm.cmd --prefix admin-vue run build
npm.cmd run build:user-h5
npm.cmd run test:user-h5:smoke
npm.cmd run build:user-mp-weixin
npm.cmd run build:user-app-plus
npm.cmd run build:user-app-harmony
npm.cmd run build:user-mp-harmony
docker compose -f compose.yml config --quiet
docker compose --env-file .env.production.example -f compose.prod.yml config --quiet
```

## API type generation

The shared domain types are still maintained in `packages/shared-types/src/index.ts`, but backend-generated API contracts now have a dedicated path:

```powershell
npm.cmd run generate:api-types
```

The script reads `OPENAPI_SPEC` first, then falls back to `backend-go/docs/openapi.json`, `backend-go/docs/swagger.json`, `docs/openapi.json`, or `docs/swagger.json`. The repository now includes an initial user-core contract at `backend-go/docs/openapi.json`. The generator writes contracts to `packages/shared-types/src/generated/openapi.ts`. Do not hand-edit generated files; keep UI-facing convenience types in `shared-types/src/index.ts`.
