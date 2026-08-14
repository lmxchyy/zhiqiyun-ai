export function exactModuleSchemaPath(
  moduleCode: string,
  modelName: string,
  guest: boolean,
) {
  const normalizedModuleCode = String(moduleCode || "").trim();
  const normalizedModelName = String(modelName || "").trim();
  if (!normalizedModuleCode) throw new Error("moduleCode is required");
  if (!normalizedModelName) throw new Error("modelName is required");

  const route = guest ? "/api/v1/public/module-schema" : "/api/v1/module-schema";
  return `${route}?module_code=${encodeURIComponent(normalizedModuleCode)}&model_name=${encodeURIComponent(normalizedModelName)}`;
}
