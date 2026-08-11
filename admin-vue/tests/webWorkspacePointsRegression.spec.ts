import { describe, expect, it } from "vitest";
import { resolveSidebarPlanPoints } from "../src/utils/sidebarPlanPoints";
import { moduleListQuery, usesInstantWorkspace } from "../src/utils/userWorkspaceLoad";

describe("web workspace points regression", () => {
  it("keeps available and lifetime total distinct after consumption", () => {
    const points = resolveSidebarPlanPoints({
      account: { available: 1169, frozen: 0, total: 5000, totalUsed: 3831, totalGranted: 5000 }
    });
    expect(points.available).toBe(1169);
    expect(points.total).toBe(5000);
    expect(points.available).not.toBe(points.total);
    expect(points.percent).toBeLessThan(100);
  });

  it("does not collapse total back to available when API only returns available+frozen", () => {
    const points = resolveSidebarPlanPoints({
      account: { available: 70, frozen: 0, totalUsed: 30 },
      summary: { availablePoints: 70 }
    });
    expect(points.available).toBe(70);
    expect(points.total).toBe(100);
  });

  it("prefers dashboard summary.totalPoints while account snapshot is still loading", () => {
    const points = resolveSidebarPlanPoints({
      summary: { availablePoints: 200, totalPoints: 800 }
    });
    expect(points.available).toBe(200);
    expect(points.total).toBe(800);
  });
});

describe("web workspace first-paint load regression", () => {
  it("keeps homepage and image workspaces on the instant shell path", () => {
    expect(usesInstantWorkspace("userDashboard")).toBe(true);
    expect(usesInstantWorkspace("userAiImage")).toBe(true);
    expect(usesInstantWorkspace("userUsage")).toBe(false);
  });

  it("caps first-paint list sizes so heavy asset signing cannot return", () => {
    expect(moduleListQuery("userDashboard")).toEqual({ taskLimit: 30, assetLimit: 30 });
    expect(moduleListQuery("userAiImage")).toEqual({ taskLimit: 40, assetLimit: 40 });
    expect(moduleListQuery("userWirelessCanvas")).toEqual({ taskLimit: 40, assetLimit: 40 });
    expect(moduleListQuery("userWorks")).toEqual({ taskLimit: 40, assetLimit: 40 });
    expect(moduleListQuery("userVideoGeneration")).toEqual({ taskLimit: 40, assetLimit: 40 });
    expect(moduleListQuery("userUsage")).toBeUndefined();
  });
});
