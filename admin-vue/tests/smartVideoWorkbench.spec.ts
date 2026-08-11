import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { adminModules } from "../src/stores/admin";
import { resolveModuleIdFromPath, resolveModulePath } from "../src/config/moduleRegistry";
import { useSmartVideoStore } from "../src/stores/smartVideo";

vi.mock("../src/api/smartVideo", () => ({
  listSmartVideoProjects: vi.fn(async () => [
    {
      id: "proj_1",
      tenantId: "t1",
      userId: "u1",
      title: "开业短视频",
      requirement: "15秒",
      status: "DRAFT",
      currentVersion: 0,
      createdAt: "2026-08-11T00:00:00Z",
      updatedAt: "2026-08-11T00:00:00Z"
    }
  ]),
  createSmartVideoProject: vi.fn(async (input: { title: string; requirement: string }) => ({
    id: "proj_new",
    tenantId: "t1",
    userId: "u1",
    title: input.title,
    requirement: input.requirement,
    status: "DRAFT",
    currentVersion: 0,
    createdAt: "2026-08-11T00:00:00Z",
    updatedAt: "2026-08-11T00:00:00Z"
  })),
  updateSmartVideoProject: vi.fn(),
  deleteSmartVideoProject: vi.fn(),
  getSmartVideoProject: vi.fn(),
  listSmartVideoAssets: vi.fn(async () => []),
  addSmartVideoAsset: vi.fn(),
  reorderSmartVideoAssets: vi.fn(),
  deleteSmartVideoAsset: vi.fn(),
  analyzeSmartVideoProject: vi.fn(),
  getSmartVideoAnalysis: vi.fn(),
  retrySmartVideoAssetAnalysis: vi.fn(),
  createSmartVideoPlanTask: vi.fn(),
  getSmartVideoPlanTask: vi.fn(),
  listSmartVideoVersions: vi.fn(async () => []),
  getSmartVideoVersion: vi.fn(),
  reviseSmartVideoVersion: vi.fn(),
  confirmSmartVideoVersion: vi.fn(),
  estimateSmartVideoRender: vi.fn(),
  createSmartVideoExport: vi.fn(),
  getSmartVideoRenderTask: vi.fn(),
  cancelSmartVideoRenderTask: vi.fn(),
  retrySmartVideoRenderTask: vi.fn(),
  uploadSmartVideoBlob: vi.fn()
}));

describe("smart video module registration", () => {
  it("registers independent user module paths", () => {
    const module = adminModules.find((item) => item.id === "userSmartVideo");
    expect(module?.path).toBe("/app/smart-video");
    expect(module?.endpoint).toBe("");
    expect(resolveModulePath("userSmartVideo")).toBe("/app/smart-video");
    expect(resolveModuleIdFromPath("/app/smart-video")).toBe("userSmartVideo");
    expect(resolveModuleIdFromPath("/app/ai-montage")).toBe("userSmartVideo");
    expect(resolveModuleIdFromPath("/app/smart-video/projects/abc")).toBe("userSmartVideo");
  });
});

describe("smart video store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("loads project list and starts create workspace", async () => {
    const store = useSmartVideoStore();
    await store.initialize();
    expect(store.projects).toHaveLength(1);
    expect(store.phase).toBe("list");

    store.startCreate();
    expect(store.phase).toBe("draft");
    store.title = "门店混剪";
    store.requirement = "开业宣传";
    await store.saveProjectMeta();
    expect(store.project?.id).toBe("proj_new");
    expect(store.project?.title).toBe("门店混剪");
  });
});
