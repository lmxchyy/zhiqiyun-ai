import { describe, expect, it } from "vitest";
import { adminNavigationGroups, adminNavigationSectionForModule } from "../src/config/adminNavigation";
import { moduleById, resolveModuleIdFromPath } from "../src/config/moduleRegistry";
import { adminDefaultOpenTabIds } from "../src/stores/admin";

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

  it("maps admin root routes to analytics while retaining the legacy analysis route", () => {
    expect(moduleById("analysis")?.path).toBe("/admin/overview");
    expect(moduleById("analysis")?.aliases || []).not.toContain("/admin");
    expect(moduleById("analytics")?.path).toBe("/admin/analytics");
    expect(moduleById("analytics")?.aliases).toEqual(expect.arrayContaining(["/admin", "/admin/"]));
    expect(resolveModuleIdFromPath("/admin")).toBe("analytics");
    expect(resolveModuleIdFromPath("/admin/")).toBe("analytics");
    expect(resolveModuleIdFromPath("/admin/analytics")).toBe("analytics");
    expect(resolveModuleIdFromPath("/admin/overview")).toBe("analysis");
  });

  it("keeps analytics as the admin default tab without removing analysis", () => {
    expect(adminDefaultOpenTabIds).toEqual(["analytics"]);
    expect(adminNavigationSectionForModule("analysis")?.section.moduleIds).toEqual(
      expect.arrayContaining(["analytics", "analysis"]),
    );
  });
});
