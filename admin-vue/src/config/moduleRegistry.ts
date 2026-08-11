import { adminModules, type AdminModule } from "../stores/admin";

export type ConsoleSurface = NonNullable<AdminModule["surface"]>;

export interface ModulePathContext {
  enterpriseId?: string;
}

const moduleIndex = new Map(adminModules.map((module) => [module.id, module]));

export function moduleById(moduleId: string) {
  return moduleIndex.get(moduleId);
}

export function moduleIdsForSurface(surface: ConsoleSurface) {
  return adminModules
    .filter((module) => (module.surface || "admin") === surface)
    .map((module) => module.id);
}

export function moduleIdsForDomain(domain: NonNullable<AdminModule["domain"]>) {
  return adminModules.filter((module) => module.domain === domain).map((module) => module.id);
}

export function modulePermission(moduleId: string) {
  return moduleById(moduleId)?.permission || "";
}

export function resolveModuleIdFromPath(pathname: string) {
  const normalized = normalizePath(pathname);
  if (normalized.startsWith("/app/ppt-generation/generate/") || normalized.startsWith("/app/ppt-generation/presentation/")) {
    return "userPptGeneration";
  }
  if (normalized.startsWith("/app/smart-video/")) {
    return "userSmartVideo";
  }
  const direct = adminModules.find((module) => module.path === normalized || module.aliases?.includes(normalized));
  if (direct) return direct.id;
  const enterpriseMatch = normalized.match(/^\/admin\/enterprises\/([^/]+)(?:\/([^/]+))?$/);
  if (!enterpriseMatch || enterpriseMatch[1] === "certifications") return "";
  const suffix = enterpriseMatch[2] || "";
  return adminModules.find((module) => module.domain === "enterprise" && module.enterpriseSuffix === suffix)?.id || "";
}

export function resolveModulePath(moduleId: string, context: ModulePathContext = {}) {
  const module = moduleById(moduleId);
  if (!module?.path) return "";
  if (!module.path.includes(":enterpriseId")) return module.path;
  return context.enterpriseId
    ? module.path.replace(":enterpriseId", encodeURIComponent(context.enterpriseId))
    : "/admin/enterprises";
}

export function moduleAliases(moduleId: string) {
  return moduleById(moduleId)?.aliases || [];
}

function normalizePath(pathname: string) {
  const value = pathname.split("?")[0].replace(/\/+$/, "");
  return value || "/";
}
