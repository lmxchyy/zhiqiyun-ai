# Tauri desktop wrapper

This app wraps the H5 output from `apps/user-uni`. Keep business code in `apps/user-uni` and shared packages; use `packages/platform-adapter` for desktop-only bridge behavior.

Development:

```powershell
npm.cmd --prefix apps/desktop-tauri install --ignore-scripts
npm.cmd --prefix apps/desktop-tauri run dev
```

Packaging:

```powershell
npm.cmd --prefix apps/desktop-tauri run build
```

Tauri requires the Rust toolchain and platform-specific build prerequisites on the packaging machine.
