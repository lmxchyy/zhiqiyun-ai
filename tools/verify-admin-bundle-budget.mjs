import fs from "node:fs";
import path from "node:path";

const assetsDir = path.resolve("admin-vue", "dist", "assets");
const indexHTMLPath = path.resolve("admin-vue", "dist", "index.html");
if (!fs.existsSync(assetsDir)) throw new Error("请先执行 admin-vue 生产构建");
const files = fs.readdirSync(assetsDir);
const entry = files.find((name) => /^index-.*\.js$/.test(name));
if (!entry) throw new Error("未找到后台入口 JS");
const entryBytes = fs.statSync(path.join(assetsDir, entry)).size;
const limitBytes = 500 * 1024;
if (entryBytes > limitBytes) throw new Error(`后台主入口超出 500KB：${(entryBytes / 1024).toFixed(2)}KB`);
for (const chunk of ["PricePlanGovernance", "pricePlanAdmin", "EnterpriseManagement", "BillingDomain", "AdminDataTable"]) {
  if (!files.some((name) => name.startsWith(`${chunk}-`) && name.endsWith(".js"))) throw new Error(`领域组件未形成异步分包：${chunk}`);
}
if (!files.some((name) => name.startsWith("PptDocumentGeneration-") && name.endsWith(".js"))) throw new Error("PPT 页面未形成异步分包");
const indexHTML = fs.readFileSync(indexHTMLPath, "utf8");
if (/assets\/[^"']*ppt[^"']*\.(?:js|css)/i.test(indexHTML)) throw new Error("PPT 资源仍被首屏 HTML 预加载");
if (/assets\/(?:PricePlanGovernance|pricePlanAdmin)-[^"']*\.js/i.test(indexHTML)) throw new Error("价格治理资源仍被首屏 HTML 预加载");
const logoAsset = files.find((name) => /^xianzhi-ai-logo-.*\.webp$/.test(name));
if (!logoAsset) throw new Error("后台品牌 Logo 未输出 WebP 资源");
const logoBytes = fs.statSync(path.join(assetsDir, logoAsset)).size;
if (logoBytes > 50 * 1024) throw new Error(`后台品牌 Logo 超出 50KB：${(logoBytes / 1024).toFixed(2)}KB`);
if (!indexHTML.includes("xianzhi-ai-logo.webp") || indexHTML.includes("xianzhi-ai-logo.png")) throw new Error("后台 favicon 未切换到 WebP");
console.log(`后台包体预算通过：主入口 ${(entryBytes / 1024).toFixed(2)}KB，领域组件已按需分包，PPT 未首屏预加载，Logo ${(logoBytes / 1024).toFixed(2)}KB。`);
