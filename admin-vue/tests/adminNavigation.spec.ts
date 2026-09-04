import { describe, expect, it } from "vitest";
import { adminNavigationGroups, adminNavigationSectionForModule } from "../src/config/adminNavigation";
import { moduleById, resolveModuleIdFromPath } from "../src/config/moduleRegistry";

describe("admin management overview navigation", () => {
  it("uses the analytics dashboard as the default entry and retains legacy tabs", () => {
    const section = adminNavigationGroups
      .find((group) => group.id === "overview")
      ?.sections.find((item) => item.id === "management-overview");

    expect(section).toMatchObject({
      primaryModuleId: "analytics",
      moduleIds: ["analytics", "analysis", "workbench", "dashboard"],
    });
    expect(section?.moduleIds).toEqual(expect.arrayContaining(["analysis", "workbench", "dashboard"]));
    expect(adminNavigationSectionForModule("analysis")?.section.id).toBe("management-overview");
  });

  it("keeps the direct analytics route wired to the dashboard module", () => {
    expect(moduleById("analytics")?.path).toBe("/admin/analytics");
    expect(resolveModuleIdFromPath("/admin/analytics")).toBe("analytics");
  });
});
