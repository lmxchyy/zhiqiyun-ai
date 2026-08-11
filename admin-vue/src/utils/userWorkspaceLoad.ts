/** Modules that paint an empty shell before the heavy workspace payload returns. */
export function usesInstantWorkspace(moduleId: string): boolean {
  return ["userDashboard", "userAiImage", "userWirelessCanvas", "userWorks", "userVideoGeneration"].includes(moduleId);
}

/**
 * Keep first-paint payloads small. Backend defaults match these caps;
 * raising them here without a product need will reintroduce slow loads.
 */
export function moduleListQuery(moduleId: string): Record<string, number> | undefined {
  if (moduleId === "userDashboard") {
    return { taskLimit: 30, assetLimit: 30 };
  }
  if (["userAiImage", "userWirelessCanvas", "userWorks", "userVideoGeneration"].includes(moduleId)) {
    return { taskLimit: 40, assetLimit: 40 };
  }
  return undefined;
}
