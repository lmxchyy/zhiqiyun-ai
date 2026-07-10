# WeChat Mini Program Login Test

This project has two WeChat mini-program login paths:

- Local WeChat DevTools simulation: `mock-devtools-code` -> local Go API -> demo user token.
- Real mini-program login: `wx.login` code -> WeChat `jscode2session` -> internal user token.

## Local DevTools Test

1. Start or rebuild the local Docker service:

```powershell
docker compose up -d --build xianzhi-ai
```

2. Confirm the Go API is healthy:

```powershell
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:3100/healthz
```

3. Confirm the mock login API returns an access token:

```powershell
Invoke-WebRequest -UseBasicParsing `
  -Method Post `
  -Uri http://127.0.0.1:3100/api/v1/auth/wechat-mini-program/login `
  -ContentType "application/json" `
  -Body '{"code":"mock-devtools-code"}'
```

4. Build the WeChat mini-program:

```powershell
npm.cmd run build:user-mp-weixin
```

5. Open the build output in WeChat DevTools:

```powershell
& "C:\Program Files (x86)\Tencent\微信web开发者工具\cli.bat" open --project "$PWD\apps\user-uni\dist\build\mp-weixin"
```

6. In WeChat DevTools, open `pages/WechatLoginPage`, then click `微信小程序登录`.

Expected result:

- The page shows login success.
- The backend response contains `accessToken`.
- The logged-in user is `demo@xianzhi.ai`.
- DevTools console logs `mock-devtools-code`.

## Real WeChat Login

1. Create or edit the server `.env` file and set:

```dotenv
WECHAT_MINI_PROGRAM_APPID=your-mini-program-appid
WECHAT_MINI_PROGRAM_SECRET=your-mini-program-secret
```

2. Recreate the backend container:

```powershell
docker compose up -d --build xianzhi-ai
```

3. Build the mini-program with a reachable HTTPS API domain:

```powershell
$env:VITE_API_BASE_URL="https://api.example.com"
npm.cmd run build:user-mp-weixin
```

4. In WeChat Mini Program Admin and WeChat DevTools, make sure the request domain allows that HTTPS API domain.

5. Test on a real device or in DevTools with a real `wx.login` code.

## Common Errors

- `wechat mini program login is not configured`: the backend container does not have `WECHAT_MINI_PROGRAM_APPID` or `WECHAT_MINI_PROGRAM_SECRET`.
- Request timeout in DevTools: `VITE_API_BASE_URL` points to an unreachable API, or DevTools request domain checks are blocking the request.
- Mock login works but real login fails: check the mini-program appid, secret, and WeChat platform domain configuration.
