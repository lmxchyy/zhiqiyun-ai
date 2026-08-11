import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import ts from "typescript";

const root = process.cwd();
const storePath = path.join(root, "admin-vue", "src", "stores", "admin.ts");
const navigationPath = path.join(root, "admin-vue", "src", "config", "adminNavigation.ts");
const appPath = path.join(root, "admin-vue", "src", "App.vue");

const storeSource = fs.readFileSync(storePath, "utf8");
const moduleBlock = storeSource.match(/export const adminModules:[\s\S]*?= (\[[\s\S]*?\n\]);\s*\nexport const useAdminStore/);
if (!moduleBlock) throw new Error("无法解析 adminModules 注册表");

const modules = Function(`"use strict"; return (${moduleBlock[1]});`)();
if (!modules.length) throw new Error("adminModules 注册表为空");

const navigationSource = fs.readFileSync(navigationPath, "utf8")
  .replace(/import \{ adminModules, type AdminModule \} from "\.\.\/stores\/admin";/, `const adminModules = ${JSON.stringify(modules)};`)
  .replace(/import \{ moduleById, moduleIdsForDomain, moduleIdsForSurface \} from "\.\/moduleRegistry";/, `
    const moduleById = (moduleId) => adminModules.find((module) => module.id === moduleId);
    const moduleIdsForDomain = (domain) => adminModules.filter((module) => module.domain === domain).map((module) => module.id);
    const moduleIdsForSurface = (surface) => adminModules.filter((module) => (module.surface || "admin") === surface).map((module) => module.id);
  `);
const transpiled = ts.transpileModule(navigationSource, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 }
}).outputText;
const registry = await import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);

const moduleIds = new Set(modules.map((module) => module.id));
const nonAdminIds = new Set([...registry.userModuleIds, ...registry.agentModuleIds, ...registry.operationCenterModuleIds]);
const expectedAdminIds = new Set(modules.map((module) => module.id).filter((id) => !nonAdminIds.has(id)));
const assigned = new Map();
const errors = [];
const groupIds = new Set();
const sectionIds = new Set();
const registeredPaths = new Map();
const pricingModule = modules.find((module) => module.id === "pricePlanGovernance");
if (!pricingModule) {
  errors.push("缺少套餐与价格配置模块 pricePlanGovernance");
} else {
  if (pricingModule.path !== "/admin/catalog/price-plans") errors.push("pricePlanGovernance 浏览器路径必须为 /admin/catalog/price-plans");
  if (pricingModule.permission !== "pricing:plan:view") errors.push("pricePlanGovernance 必须使用精确权限 pricing:plan:view");
  if (pricingModule.domain) errors.push("pricePlanGovernance 不得归入 billing 或其他旧 domain");
}
const requiredArchitectureFiles = [
  "admin-vue/src/components/admin/AdminDataTable.vue",
  "admin-vue/src/components/admin/Customer360Center.vue",
  "admin-vue/src/components/admin/OrderFulfillmentCenter.vue",
  "admin-vue/src/components/admin/AdminExceptionCenter.vue",
  "admin-vue/src/components/admin/AdminExperienceInsights.vue",
  "admin-vue/src/components/navigation/GlobalCommandPalette.vue",
  "admin-vue/src/composables/useAdminGlobalSearch.ts",
  "admin-vue/src/composables/useAdminExperienceTracking.ts"
];

for (const relativePath of requiredArchitectureFiles) {
  if (!fs.existsSync(path.join(root, relativePath))) errors.push(`缺少后台体验架构文件: ${relativePath}`);
}

const appSource = fs.readFileSync(appPath, "utf8");
for (const componentName of ["AdminDataTable", "Customer360Center", "OrderFulfillmentCenter", "GlobalCommandPalette"]) {
  if (!appSource.includes(componentName)) errors.push(`App.vue 尚未接入 ${componentName}`);
}
for (const pricingNavigationGuard of [
  "canAccessAdminModule",
  "authorizedAdminModuleId(moduleId)",
  "enforceActiveAdminModuleAccess()",
  "canNavigateToModule(moduleId)"
]) {
  if (!appSource.includes(pricingNavigationGuard)) errors.push(`套餐与价格配置缺少权限导航防线: ${pricingNavigationGuard}`);
}
const dataTableSource = fs.readFileSync(path.join(root, "admin-vue/src/components/admin/AdminDataTable.vue"), "utf8");
for (const capability of ["保存当前视图", "列配置", "批量操作", "导出当前结果"]) {
  if (!dataTableSource.includes(capability)) errors.push(`统一列表缺少能力: ${capability}`);
}
const workspaceAPISource = fs.readFileSync(path.join(root, "admin-vue/src/api/adminWorkspaces.ts"), "utf8");
for (const searchType of ["enterprise", "generation_task", "payment", "invoice"]) {
  if (!workspaceAPISource.includes(searchType)) errors.push(`全局搜索缺少类型: ${searchType}`);
}
for (const experienceBoundary of ["navigator.webdriver", "synthetic", "minimumActiveDays"]) {
  if (!workspaceAPISource.includes(experienceBoundary)) errors.push(`体验数据缺少自动化隔离字段: ${experienceBoundary}`);
}
const experienceInsightsSource = fs.readFileSync(path.join(root, "admin-vue/src/components/admin/AdminExperienceInsights.vue"), "utf8");
for (const experienceGuard of ["真人体验事件", "真实样本仍在积累", "当前不建议合并入口"]) {
  if (!experienceInsightsSource.includes(experienceGuard)) errors.push(`低频入口决策缺少样本保护: ${experienceGuard}`);
}
for (const deprecatedTerm of ["平台治理", "商品与套餐", "组织与账号"]) {
  if (navigationSource.includes(deprecatedTerm) || storeSource.includes(deprecatedTerm)) errors.push(`仍存在旧后台术语: ${deprecatedTerm}`);
}

for (const module of modules) {
  if (moduleIds.has(module.id) && modules.filter((candidate) => candidate.id === module.id).length > 1) {
    errors.push(`重复的模块 ID: ${module.id}`);
  }
  for (const routePath of [module.path, ...(module.aliases || [])].filter(Boolean)) {
    if (registeredPaths.has(routePath)) errors.push(`路径 ${routePath} 同时属于 ${registeredPaths.get(routePath)} 和 ${module.id}`);
    registeredPaths.set(routePath, module.id);
  }
  if (!module.path) errors.push(`模块缺少浏览器路径: ${module.id}`);
  if (module.domain === "enterprise" && (!module.path || !module.permission)) errors.push(`企业模块缺少路径或权限: ${module.id}`);
  if (module.domain === "billing" && !module.path) errors.push(`计费模块缺少浏览器路径: ${module.id}`);
}

for (const group of registry.adminNavigationGroups) {
  if (groupIds.has(group.id)) errors.push(`重复的导航分组 ID: ${group.id}`);
  groupIds.add(group.id);
  for (const section of group.sections) {
    if (sectionIds.has(section.id)) errors.push(`重复的导航区块 ID: ${section.id}`);
    sectionIds.add(section.id);
    if (!section.moduleIds.includes(section.primaryModuleId)) errors.push(`${section.id} 的 primaryModuleId 不在 moduleIds 中`);
    for (const tabId of section.tabModuleIds || []) {
      if (!section.moduleIds.includes(tabId)) errors.push(`${section.id} 的 Tab ${tabId} 不属于该区块`);
    }
    for (const moduleId of section.moduleIds) {
      if (!moduleIds.has(moduleId)) errors.push(`${section.id} 引用了不存在的模块 ${moduleId}`);
      if (assigned.has(moduleId)) errors.push(`${moduleId} 同时属于 ${assigned.get(moduleId)} 和 ${section.id}`);
      assigned.set(moduleId, section.id);
    }
  }
}

const pricingNavigation = registry.adminNavigationSectionForModule("pricePlanGovernance");
if (!pricingNavigation) {
  errors.push("pricePlanGovernance 缺少独立导航区块");
} else if (pricingNavigation.section.moduleIds.length !== 1 || pricingNavigation.section.moduleIds[0] !== "pricePlanGovernance") {
  errors.push("pricePlanGovernance 必须位于只包含自身的独立导航区块");
}

for (const moduleId of expectedAdminIds) {
  if (!assigned.has(moduleId)) errors.push(`后台模块未分配导航区块: ${moduleId}`);
}
for (const moduleId of assigned.keys()) {
  if (!expectedAdminIds.has(moduleId)) errors.push(`非后台模块被放入后台导航: ${moduleId}`);
}

if (errors.length) {
  console.error(errors.map((error) => `- ${error}`).join("\n"));
  process.exit(1);
}

const sectionCount = registry.adminNavigationGroups.reduce((total, group) => total + group.sections.length, 0);
console.log(`后台导航校验通过：${registry.adminNavigationGroups.length} 个业务域，${sectionCount} 个侧栏入口，覆盖 ${assigned.size} 个后台模块。`);
