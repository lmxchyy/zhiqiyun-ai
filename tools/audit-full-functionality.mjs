import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const requireFromUni = createRequire(path.join(repoRoot, "apps", "user-uni", "package.json"));
const { parse: parseSfc } = requireFromUni("@vue/compiler-sfc");
const { baseParse, NodeTypes } = requireFromUni("@vue/compiler-dom");
const ts = requireFromUni("typescript");

const userRoot = path.join(repoRoot, "apps", "user-uni", "src");
const adminRoot = path.join(repoRoot, "admin-vue", "src");
const outputRoot = path.join(repoRoot, "docs", "acceptance");
const serverPath = path.join(repoRoot, "backend-go", "internal", "httpserver", "server.go");

const rel = file => path.relative(repoRoot, file).replaceAll("\\", "/");
const read = file => fs.readFileSync(file, "utf8");
const uniq = values => [...new Set(values.filter(Boolean))];

function walk(root, extension) {
  const files = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const full = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...walk(full, extension));
    else if (entry.name.endsWith(extension)) files.push(full);
  }
  return files.sort();
}

function parsePagesJSON() {
  const file = path.join(userRoot, "pages.json");
  const source = read(file);
  const tabBarSource = source.match(/"tabBar"\s*:\s*\{([\s\S]*)$/)?.[1] || "";
  const tabPages = new Set([...tabBarSource.matchAll(/"pagePath"\s*:\s*"([^"]+)"/g)].map(match => match[1]));
  const pages = [];
  for (const match of source.matchAll(/\{\s*"path"\s*:\s*"([^"]+)"\s*,\s*"style"\s*:\s*\{([\s\S]*?)\}\s*\}/g)) {
    const title = match[2].match(/"navigationBarTitleText"\s*:\s*"([^"]+)"/)?.[1] || path.basename(match[1]);
    pages.push({ path: match[1], title, tabBar: tabPages.has(match[1]) });
  }
  return {
    pages,
    tabPages,
  };
}

function backendRoutes() {
  const source = read(serverPath);
  const routes = [];
  const matcher = /([A-Za-z0-9_]+)\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)"\s*,\s*(?:wrapF\()?([^),\n]+)[^\n]*\)/g;
  for (const match of source.matchAll(matcher)) {
    const group = match[1];
    const prefix = group === "v1" ? "/api/v1" : group === "adminGroup" ? "/api/v1/admin" : "";
    routes.push({ method: match[2], path: `${prefix}${match[3]}`, handler: match[4], line: source.slice(0, match.index).split("\n").length });
  }
  return routes;
}

function routePattern(routePath) {
  return new RegExp(`^${routePath.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replace(/:([A-Za-z0-9_]+)/g, "[^/?]+")}(?:\\?.*)?$`);
}

function normalizeEndpoint(value) {
  return value
    .replace(/\$\{[^}]+\}/g, ":id")
    .replace(/\?\$\{[^}]+\}/g, "")
    .trim();
}

function methodInventory(script, filename) {
  const methods = new Map();
  const source = ts.createSourceFile(filename, script, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const visit = node => {
    if (ts.isImportDeclaration(node) && node.importClause) {
      if (node.importClause.name) methods.set(node.importClause.name.text, { empty: false, text: "" });
      const bindings = node.importClause.namedBindings;
      if (bindings && ts.isNamedImports(bindings)) {
        for (const item of bindings.elements) methods.set(item.name.text, { empty: false, text: "" });
      }
      if (bindings && ts.isNamespaceImport(bindings)) methods.set(bindings.name.text, { empty: false, text: "" });
    }
    if ((ts.isFunctionDeclaration(node) && node.name) || ts.isMethodDeclaration(node)) {
      const name = node.name?.getText(source);
      if (name) methods.set(name, { empty: !node.body || node.body.statements.length === 0, text: node.getText(source) });
    }
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer && (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer))) {
      const body = node.initializer.body;
      methods.set(node.name.text, { empty: ts.isBlock(body) && body.statements.length === 0, text: node.initializer.getText(source) });
    }
    if (ts.isVariableDeclaration(node) && ts.isObjectBindingPattern(node.name) && node.initializer) {
      for (const element of node.name.elements) {
        if (ts.isIdentifier(element.name)) methods.set(element.name.text, { empty: false, text: node.initializer.getText(source) });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return methods;
}

function literalText(node) {
  if (node.type === NodeTypes.TEXT) return node.content.trim();
  if (node.type === NodeTypes.INTERPOLATION) return `{{ ${node.content.loc.source.trim()} }}`;
  return (node.children || []).map(literalText).join(" ").replace(/\s+/g, " ").trim();
}

function attr(node, name) {
  const found = node.props.find(prop => prop.type === NodeTypes.ATTRIBUTE && prop.name === name);
  return found?.value?.content || "";
}

function directive(node, name, arg = "") {
  return node.props.find(prop => prop.type === NodeTypes.DIRECTIVE && prop.name === name && (!arg || prop.arg?.content === arg));
}

function eventDirectives(node) {
  return node.props.filter(prop => prop.type === NodeTypes.DIRECTIVE && prop.name === "on");
}

function inferExpected(tag, label, events) {
  const text = `${label} ${events}`;
  if (/删除/.test(text)) return "二次确认后删除并反馈结果";
  if (/提交|保存|生成|登录|注册|充值|提现|支付/.test(text)) return "校验输入、提交请求并展示成功或失败反馈";
  if (/下载|导出/.test(text)) return "下载或导出目标文件并反馈结果";
  if (/复制/.test(text)) return "复制内容并提示成功或失败";
  if (/分享/.test(text)) return "调起分享能力";
  if (/返回/.test(text)) return "返回上一页，无法返回时进入业务首页";
  if (/详情|查看|订单|作品|客户|分润|佣金|提现/.test(text)) return "携带业务 ID 进入对应页面或详情";
  if (["input", "textarea", "picker", "switch", "slider"].includes(tag)) return "更新输入状态并触发校验或筛选";
  return "触发组件声明的业务动作并提供可见反馈";
}

function handlerEvidence(expression, methods) {
  const methodName = expression.match(/^[A-Za-z_$][\w$]*/)?.[0] || "";
  const method = methods.get(methodName);
  const text = method?.text || expression;
  const navs = [...text.matchAll(/(?:uni\.)?(navigateTo|redirectTo|switchTab|reLaunch|navigateBack|push|replace)\s*\(\s*\{?[^)]*?(?:url\s*:\s*)?[`'"]([^`'"]+)[`'"]/gs)]
    .map(match => `${match[1]} ${normalizeEndpoint(match[2])}`);
  const apiCalls = [...text.matchAll(/(?:api|adminRequest|request)(?:<[^>]+>)?\s*\(\s*(?:\{[\s\S]*?url\s*:\s*)?[`'"]([^`'"]+)[`'"]/g)]
    .map(match => normalizeEndpoint(match[1]));
  const methodsUsed = uniq([...text.matchAll(/\b([A-Za-z_$][\w$]*)\s*\(/g)].map(match => match[1]));
  return { methodName, method, navs: uniq(navs), apiCalls: uniq(apiCalls), methodsUsed };
}

function scanVue(file, pageByFile, routes) {
  const source = read(file);
  const { descriptor } = parseSfc(source, { filename: file });
  const template = descriptor.template?.content || "";
  const script = `${descriptor.script?.content || ""}\n${descriptor.scriptSetup?.content || ""}`;
  const methods = methodInventory(script, file);
  const page = pageByFile.get(path.resolve(file));
  const scope = rel(file).startsWith("admin-vue/") ? "管理后台" : page ? "小程序页面" : "小程序组件";
  const rows = [];
  const findings = [];
  if (!template.trim()) return { rows, findings, endpoints: [] };
  let ast;
  try {
    ast = baseParse(template, { comments: false });
  } catch (error) {
    findings.push({ severity: "P0", file: rel(file), line: descriptor.template?.loc.start.line || 1, issue: `模板无法解析：${error.message}`, suggestion: "修复模板语法后重新扫描" });
    return { rows, findings, endpoints: [] };
  }
  const interactiveTags = new Set(["button", "input", "textarea", "picker", "switch", "slider", "navigator", "form", "scroll-view"]);
  const visit = (node, ancestors = []) => {
    if (node.type === NodeTypes.ELEMENT) {
      const events = eventDirectives(node);
      const interactive = interactiveTags.has(node.tag) || events.length > 0 || attr(node, "role") === "button";
      if (interactive) {
        const label = attr(node, "aria-label") || attr(node, "title") || literalText(node).slice(0, 80) || attr(node, "placeholder") || node.tag;
        const eventText = events.map(item => `${item.arg?.content || "event"}: ${item.exp?.content || ""}`).join("; ");
        const expression = events[0]?.exp?.content || "";
        const evidence = handlerEvidence(expression, methods);
        const line = (descriptor.template?.loc.start.line || 1) + node.loc.start.line - 1;
        const disabled = Boolean(attr(node, "disabled") || directive(node, "bind", "disabled"));
        const conditions = [directive(node, "if")?.exp?.content, directive(node, "show")?.exp?.content].filter(Boolean).join("; ");
        const issues = [];
        const isSubmit = node.tag === "button" && attr(node, "type") === "submit";
        const delegatedInteractive = ancestors.some(parent => ["el-dropdown", "el-upload"].includes(parent.tag));
        if (["button", "navigator"].includes(node.tag) && events.length === 0 && !attr(node, "open-type") && !isSubmit && !delegatedInteractive) issues.push("没有绑定操作事件");
        const isDirectHandler = /^[A-Za-z_$][\w$]*(?:\([^)]*\))?$/.test(expression.trim());
        if (isDirectHandler && evidence.methodName && !evidence.method && !expression.includes("$emit") && !expression.includes("=") && !expression.includes("uni.")) issues.push(`处理函数 ${evidence.methodName} 未在当前文件定义`);
        if (evidence.method?.empty) issues.push(`处理函数 ${evidence.methodName} 为空`);
        if (/fail\s*:\s*\(?.*?\)?\s*=>\s*(?:undefined|\{\s*\})/s.test(evidence.method?.text || expression)) issues.push("失败回调被静默忽略");
        rows.push({ scope, page: page?.title || path.basename(file, ".vue"), pagePath: page ? `/${page.path}` : "-", component: `${node.tag}.${String(attr(node, "class") || "interactive").split(/\s+/)[0]}`, file: rel(file), line, expected: inferExpected(node.tag, label, eventText), label, event: eventText || "未绑定", method: evidence.methodName || "内联表达式/原生行为", route: evidence.navs.join("；") || "-", api: evidence.apiCalls.join("；") || "-", requestMethod: evidence.apiCalls.length ? "由处理函数确定" : "-", params: expression || "-", state: issues.length ? "待修复" : disabled ? "条件可用" : "已绑定", issue: issues.join("；") || "-", suggestion: issues.length ? "补齐事件、失败反馈并执行点击回归" : "运行时点击验证" , conditions });
        for (const issue of issues) findings.push({ severity: issue.includes("未在") || issue.includes("为空") ? "P0" : "P1", file: rel(file), line, issue: `${label}: ${issue}`, suggestion: "补齐实际业务处理并提供用户反馈" });
      }
      for (const child of node.children || []) visit(child, [...ancestors, node]);
    } else {
      for (const child of node.children || []) visit(child, ancestors);
    }
  };
  visit(ast);

  for (const match of script.matchAll(/(?:api|adminRequest|request)(?:<[^>]+>)?\s*\(\s*(?:\{[\s\S]*?url\s*:\s*)?[`'"]([^`'"]+)[`'"]/g)) {
    const endpoint = normalizeEndpoint(match[1]);
    if (!endpoint.startsWith("/")) continue;
    const full = endpoint.startsWith("/api/") ? endpoint : rel(file).startsWith("admin-vue/") ? `/api/v1${endpoint}` : endpoint;
    const matched = routes.find(route => routePattern(route.path).test(full));
    if (!matched && full.startsWith("/api/v1/")) findings.push({ severity: "P0", file: rel(file), line: script.slice(0, match.index).split("\n").length, issue: `前端接口未匹配后端路由：${full}`, suggestion: "核对请求地址、方法或补充后端路由" });
  }
  return { rows, findings, endpoints: [] };
}

function adminModuleInventory(routes) {
  const file = path.join(adminRoot, "stores", "admin.ts");
  const source = read(file);
  const rows = [];
  for (const match of source.matchAll(/\{\s*id:\s*"([^"]+)",\s*title:\s*"([^"]+)",\s*endpoint:\s*"([^"]*)"\s*\}/g)) {
    const endpoint = match[3] ? `/api/v1${match[3]}` : "";
    const matched = endpoint ? routes.find(route => routePattern(route.path).test(endpoint)) : null;
    rows.push({ id: match[1], title: match[2], endpoint: endpoint || "组件内加载", backend: matched ? `${matched.method} ${matched.path} → ${matched.handler}` : endpoint ? "未匹配" : "组件专用接口", state: matched || !endpoint ? "已映射" : "待修复" });
  }
  return rows;
}

function markdownTable(headers, rows) {
  const safe = value => String(value ?? "-").replaceAll("|", "\\|").replaceAll("\n", " ");
  return [`| ${headers.join(" | ")} |`, `| ${headers.map(() => "---").join(" | ")} |`, ...rows.map(row => `| ${row.map(safe).join(" | ")} |`)].join("\n");
}

function writeReports() {
  const { pages, tabPages } = parsePagesJSON();
  const routes = backendRoutes();
  const pageByFile = new Map(pages.map(page => [path.resolve(userRoot, `${page.path}.vue`), page]));
  const vueFiles = [...walk(path.join(userRoot, "pages"), ".vue"), ...walk(path.join(userRoot, "components"), ".vue"), ...walk(adminRoot, ".vue")];
  const inventory = [];
  const findings = [];
  for (const file of vueFiles) {
    const result = scanVue(file, pageByFile, routes);
    inventory.push(...result.rows);
    findings.push(...result.findings);
  }
  for (const page of pages) {
    const file = path.resolve(userRoot, `${page.path}.vue`);
    if (!fs.existsSync(file)) findings.push({ severity: "P0", file: rel(file), line: 1, issue: `pages.json 注册页面不存在：${page.path}`, suggestion: "修正路径或补齐页面文件" });
  }
  const registeredFiles = new Set(pages.map(page => path.resolve(userRoot, `${page.path}.vue`)));
  for (const file of walk(path.join(userRoot, "pages"), ".vue")) {
    if (!registeredFiles.has(path.resolve(file))) findings.push({ severity: "P1", file: rel(file), line: 1, issue: "页面文件未在 pages.json 注册", suggestion: "注册为独立页面或明确标记为非小程序页面" });
  }
  const adminModules = adminModuleInventory(routes);

  fs.mkdirSync(outputRoot, { recursive: true });
  const checklistHeaders = ["范围", "页面名称", "页面路径", "组件", "文件", "行", "预期行为", "当前事件", "调用方法", "跳转路由", "请求接口", "请求方法", "请求参数", "状态", "问题", "修复建议"];
  fs.writeFileSync(path.join(outputRoot, "页面功能验收清单.md"), [
    "# 页面功能验收清单",
    "",
    `生成时间：${new Date().toISOString()}`,
    "",
    `- 小程序注册页面：${pages.length}`,
    `- 小程序 TabBar 页面：${tabPages.size}`,
    `- 扫描 Vue 文件：${vueFiles.length}`,
    `- 可交互项：${inventory.length}`,
    `- 后端路由：${routes.length}`,
    "",
    markdownTable(checklistHeaders, inventory.map(item => [item.scope, item.page, item.pagePath, item.component, item.file, item.line, item.expected, item.event, item.method, item.route, item.api, item.requestMethod, item.params, item.state, item.issue, item.suggestion])),
    "",
  ].join("\n"), "utf8");

  fs.writeFileSync(path.join(outputRoot, "无响应组件清单.md"), [
    "# 无响应组件清单",
    "",
    "此清单是静态扫描基线；修复后仍需运行时点击确认。",
    "",
    markdownTable(["优先级", "文件", "行", "问题", "修复建议"], findings.map(item => [item.severity, item.file, item.line, item.issue, item.suggestion])),
    "",
  ].join("\n"), "utf8");

  fs.writeFileSync(path.join(outputRoot, "前后端接口映射表.md"), [
    "# 前后端接口映射表",
    "",
    "## 管理后台模块",
    "",
    markdownTable(["模块 ID", "模块名称", "前端接口", "后端映射", "状态"], adminModules.map(item => [item.id, item.title, item.endpoint, item.backend, item.state])),
    "",
    "## 后端路由总表",
    "",
    markdownTable(["方法", "路由", "处理函数", "注册行"], routes.map(route => [route.method, route.path, route.handler, `server.go:${route.line}`])),
    "",
  ].join("\n"), "utf8");

  const summary = { pages: pages.length, tabPages: tabPages.size, vueFiles: vueFiles.length, interactiveItems: inventory.length, findings: findings.length, backendRoutes: routes.length, adminModules: adminModules.length };
  fs.writeFileSync(path.join(outputRoot, "scan-summary.json"), `${JSON.stringify(summary, null, 2)}\n`, "utf8");
  console.log(JSON.stringify(summary, null, 2));
}

writeReports();
