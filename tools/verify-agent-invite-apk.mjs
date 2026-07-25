import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const read = (path) => readFileSync(resolve(root, path), "utf8");
const page = read("apps/user-uni/src/pages/invite/InviteRegisterPage.vue");
const routes = read("apps/user-uni/src/pages.json");
const server = read("backend-go/internal/httpserver/server.go");
const inviteAPI = read("backend-go/internal/httpserver/agent_invite_apk_api.go");
const app = read("apps/user-uni/src/App.vue");
const loginPage = read("apps/user-uni/src/pages/WechatLoginPage.vue");
const migration = read("database/migrations/077-agent-invite-apk-distribution.sql");
const postgresTest = read("backend-go/internal/httpserver/agent_invite_apk_postgres_test.go");

const checks = [
  ["H5 invite page route", routes.includes('"path": "pages/invite/InviteRegisterPage"')],
  ["WeChat open-in-browser guidance", page.includes("在浏览器中打开") && page.includes("isWeChat")],
  ["iOS blocks APK button", page.includes('v-else-if="isIOS"') && page.includes("iOS 版本敬请期待")],
  ["Android/browser download action", page.includes("@click=\"downloadAPK\"")],
  ["legal acceptance UI", page.includes("《用户协议》") && page.includes("《隐私政策》")],
  ["public H5 short link", server.includes('router.GET("/d/:inviteCode"')],
  ["fixed download aliases", server.includes('router.GET("/android/latest"') && server.includes('"/public/app/releases/android/latest/download"')],
  ["APP activation funnel hook", server.includes('"/app/activation"') && app.includes("recordAgentInviteAppActivation")],
  ["APP activation after login", loginPage.includes("void recordAgentInviteAppActivation()")],
  ["public agent alias is explicit", inviteAPI.includes("agent.raw->>'inviteDisplayName'") && !inviteAPI.includes("nullif(users.name") && !inviteAPI.includes("nullif(inviter.name")],
  ["registration idempotency is request scoped", inviteAPI.includes('inviteCodeFromRequest(r) + "|" + mobile + "|" + idempotencyKey')],
  ["one published release constraint", migration.includes("ux_xz_app_releases_one_published") && migration.includes("WHERE status = 'published'")],
  ["one active user relationship", migration.includes("xz_user_relationships") || read("database/migrations/066-user-business-identity-foundation.sql").includes("ux_xz_user_relationships_current")],
  ["invite migration matches final uppercase constraint", migration.includes("invite_code !~ '^[A-Z0-9]{8,12}$'") && migration.includes("invite_code ~ '^[A-Z0-9]{8,12}$'")],
  ["PostgreSQL concurrent attribution coverage", postgresTest.includes("TestAgentInvitePostgresConcurrentAttributionAndIdempotency")],
];

const failed = checks.filter(([, passed]) => !passed);
for (const [name, passed] of checks) {
  console.log(`${passed ? "PASS" : "FAIL"} ${name}`);
}
if (failed.length) {
  process.exitCode = 1;
}
