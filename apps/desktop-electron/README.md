# Electron desktop wrapper

This app is only a desktop shell for the H5 output from `apps/user-uni`.

It must not contain user-facing business pages, API clients, stores, or duplicated domain logic. Add desktop-only capabilities through `packages/platform-adapter` and expose them to the H5 app through a bridge.

Development:

```powershell
npm.cmd --prefix apps/user-uni run dev
$env:XIANZHI_DESKTOP_DEV_URL="http://127.0.0.1:5173"
npm.cmd --prefix apps/desktop-electron run dev
```

Packaging:

```powershell
npm.cmd --prefix apps/desktop-electron install --ignore-scripts
npm.cmd --prefix apps/desktop-electron run build
```
