import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../src/api/client.ts";
import * as pptApi from "../src/api/ppt.ts";
import { usePptStore } from "../src/stores/ppt.ts";

const originalAxiosAdapter = apiClient.defaults.adapter;

const outline = {
  title: "Agent 演示",
  slides: [
    { page: 1, title: "封面", summary: "一句话价值", bulletPoints: ["核心价值"], layout: "cover" },
    { page: 2, title: "方案", summary: "落地路径", bulletPoints: ["第一步"], layout: "content" }
  ]
};

const slides = [
  { id: "slide_1", page: 1, title: "封面", content: "一句话价值", bulletPoints: ["核心价值"], layout: "cover", blocks: [{ type: "title", text: "封面" }] },
  { id: "slide_2", page: 2, title: "方案", content: "落地路径", bulletPoints: ["第一步"], layout: "content", blocks: [{ type: "title", text: "方案" }] }
];

function task(stage: string, patch: Record<string, unknown> = {}) {
  const status = stage === "READY" ? "success" : stage === "GENERATING" ? "processing" : stage === "FAILED" ? "failed" : stage === "CANCELLED" ? "cancelled" : "pending";
  return {
    taskId: "ppt_agent_1",
    sessionId: "ppt_session_1",
    skillCode: "general",
    stage,
    status,
    title: "Agent 演示",
    prompt: "做一份 Agent 演示",
    slideCount: 2,
    language: "zh",
    audience: "business",
    progress: stage === "READY" ? 100 : 0,
    currentPage: stage === "READY" ? 2 : 0,
    outline: stage === "DRAFT" ? undefined : outline,
    slides: stage === "READY" ? slides : [],
    agentMessages: stage === "DRAFT" ? [] : [
      { role: "user", content: "生成两页大纲", createdAt: "2026-08-06T00:00:00Z" },
      { role: "assistant", content: "已生成大纲：Agent 演示", createdAt: "2026-08-06T00:00:01Z" }
    ],
    pptUrl: "",
    pdfUrl: "",
    errorMessage: "",
    ...patch
  };
}

function jsonData(value: unknown) {
  return typeof value === "string" ? JSON.parse(value) : value;
}

beforeEach(() => {
  setActivePinia(createPinia());
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  apiClient.defaults.adapter = originalAxiosAdapter;
});

describe("PPT Agent admin API", () => {
  it("uses the shared client with exact Agent routes, bodies, and idempotency headers", async () => {
    const requests: Array<{ method?: string; url?: string; data?: unknown; idempotencyKey?: string }> = [];
    apiClient.defaults.adapter = async (config) => {
      requests.push({
        method: config.method,
        url: config.url,
        data: jsonData(config.data),
        idempotencyKey: config.headers?.get("Idempotency-Key")
      });
      const data = config.url === "/ppt/skills"
        ? [{ code: "general", name: "通用演示", description: "通用", maxSlides: 30 }]
        : task(config.url?.endsWith("/confirm-outline") ? "GENERATING" : config.url?.endsWith("/revise-slide") ? "READY" : config.url?.includes("/messages") ? "OUTLINE_READY" : "DRAFT");
      return { data, status: 200, statusText: "OK", headers: {}, config };
    };

    const api = pptApi as unknown as {
      getPptSkills: () => Promise<unknown>;
      createPptSession: (request: unknown) => Promise<unknown>;
      postPptSessionMessage: (taskId: string, message: string, key: string) => Promise<unknown>;
      confirmPptSessionOutline: (taskId: string, key: string) => Promise<unknown>;
      getPptAgentTask: (taskId: string) => Promise<unknown>;
      revisePptSessionSlide: (taskId: string, slideId: string, instruction: string, key: string) => Promise<unknown>;
    };
    await api.getPptSkills();
    await api.createPptSession({ prompt: "deck", skillCode: "general", slideCount: 8, language: "zh", audience: "business", clientRequestId: "client-1" });
    await api.postPptSessionMessage("ppt / 1", "outline", "message-1");
    await api.confirmPptSessionOutline("ppt / 1", "confirm-1");
    await api.getPptAgentTask("ppt / 1");
    await api.revisePptSessionSlide("ppt / 1", "slide_2", "改成结论式标题", "revise-1");

    expect(requests).toEqual([
      { method: "get", url: "/ppt/skills", data: undefined, idempotencyKey: undefined },
      { method: "post", url: "/ppt/sessions", data: { prompt: "deck", skillCode: "general", slideCount: 8, language: "zh", audience: "business", clientRequestId: "client-1" }, idempotencyKey: undefined },
      { method: "post", url: "/ppt/sessions/ppt%20%2F%201/messages", data: { message: "outline" }, idempotencyKey: "message-1" },
      { method: "post", url: "/ppt/sessions/ppt%20%2F%201/confirm-outline", data: {}, idempotencyKey: "confirm-1" },
      { method: "get", url: "/ppt/tasks/ppt%20%2F%201", data: undefined, idempotencyKey: undefined },
      { method: "post", url: "/ppt/sessions/ppt%20%2F%201/revise-slide", data: { slideId: "slide_2", instruction: "改成结论式标题" }, idempotencyKey: "revise-1" }
    ]);
  });

  it("propagates Agent API failures instead of falling back to mock data", async () => {
    apiClient.defaults.adapter = async (config) => Promise.reject(new Error(`offline: ${config.url}`));
    const api = pptApi as unknown as { getPptSkills: () => Promise<unknown> };

    await expect(api.getPptSkills()).rejects.toThrow("请求失败，请稍后重试");
  });
});

describe("PPT Agent store", () => {
  it.each([
    ["DRAFT", "idle"],
    ["OUTLINE_READY", "outline_ready"],
    ["GENERATING", "generating"],
    ["READY", "success"],
    ["FAILED", "failed"],
    ["CANCELLED", "cancelled"]
  ])("maps authoritative %s stage to %s without treating cancellation as success", (stage, expectedStatus) => {
    const store = usePptStore();
    (store as unknown as { applyAgentTask: (value: unknown) => void }).applyAgentTask(task(stage));

    expect(store.status).toBe(expectedStatus);
  });

  it("creates a session, sends a message, and polls only server stages until READY", async () => {
    vi.useFakeTimers();
    const requests: string[] = [];
    let pollCount = 0;
    apiClient.defaults.adapter = async (config) => {
      requests.push(`${config.method} ${config.url}`);
      let data: unknown;
      if (config.url === "/ppt/sessions") data = task("DRAFT");
      else if (config.url?.endsWith("/messages")) data = task("OUTLINE_READY");
      else if (config.url?.endsWith("/confirm-outline")) data = task("GENERATING", { progress: 0, currentPage: 0 });
      else if (config.url?.startsWith("/ppt/tasks/")) {
        pollCount += 1;
        data = pollCount === 1
          ? task("GENERATING", { progress: 50, currentPage: 1, slides: [slides[0]] })
          : task("READY");
      } else data = [];
      return { data, status: 200, statusText: "OK", headers: {}, config };
    };
    const store = usePptStore();
    store.prompt = "做一份 Agent 演示";
    store.slideCount = 2;
    store.audience = "business";
    (store as unknown as { skillCode: string }).skillCode = "general";

    await store.generateOutlineFlow();
    expect(store.taskId).toBe("ppt_agent_1");
    expect(store.status).toBe("outline_ready");
    expect(store.outline).toEqual(outline);

    const confirmation = store.confirmOutlineAndGeneratePpt();
    await vi.runAllTimersAsync();
    await confirmation;

    expect(store.status).toBe("success");
    expect(store.progress).toBe(100);
    expect(store.currentPage).toBe(2);
    expect(store.slides).toEqual(slides);
    expect(requests.filter((item) => item.startsWith("get /ppt/tasks/"))).toHaveLength(2);
  });

  it("stops polling on CANCELLED and never marks the task successful", async () => {
    vi.useFakeTimers();
    let pollCount = 0;
    apiClient.defaults.adapter = async (config) => {
      const data = config.url?.endsWith("/confirm-outline")
        ? task("GENERATING")
        : task("CANCELLED", { progress: 25, currentPage: 1 });
      if (config.url?.startsWith("/ppt/tasks/")) pollCount += 1;
      return { data, status: 200, statusText: "OK", headers: {}, config };
    };
    const store = usePptStore();
    (store as unknown as { applyAgentTask: (value: unknown) => void }).applyAgentTask(task("OUTLINE_READY"));

    const confirmation = store.confirmOutlineAndGeneratePpt();
    await vi.advanceTimersByTimeAsync(1_500);
    await confirmation;

    expect(pollCount).toBe(1);
    expect(store.status).toBe("cancelled");
    expect(store.progress).toBe(25);
    expect(store.currentPage).toBe(1);
    expect(store.error).toEqual(expect.objectContaining({ title: "PPT生成已取消" }));
  });

  it("blocks Agent save and confirm when local outline edits are not persisted by the backend contract", async () => {
    const requests: string[] = [];
    apiClient.defaults.adapter = async (config) => {
      requests.push(`${config.method} ${config.url}`);
      return { data: task("GENERATING"), status: 200, statusText: "OK", headers: {}, config };
    };
    const store = usePptStore();
    (store as unknown as { applyAgentTask: (value: unknown) => void }).applyAgentTask(task("OUTLINE_READY"));
    store.updateOutlineSlide(0, { title: "本地编辑后的标题" });

    await store.saveOutline();
    await store.confirmOutlineAndGeneratePpt();

    expect(requests).toEqual([]);
    expect((store as unknown as { outlineDirty: boolean }).outlineDirty).toBe(true);
    expect(store.status).toBe("outline_ready");
    expect(store.error).toEqual({
      title: "大纲尚未同步",
      message: "请通过 Agent 消息修订大纲或恢复服务端版本"
    });
  });

  it("applies revise-slide only to the requested slide from the returned task snapshot", async () => {
    const returnedSlides = [
      { ...slides[0], title: "不应覆盖的其它页" },
      { ...slides[1], title: "修订后的结论", content: "只修改第二页" }
    ];
    apiClient.defaults.adapter = async (config) => ({
      data: task("READY", { slides: returnedSlides }),
      status: 200,
      statusText: "OK",
      headers: {},
      config
    });
    const store = usePptStore();
    (store as unknown as { applyAgentTask: (value: unknown) => void }).applyAgentTask(task("READY"));
    store.selectSlide(1);
    const firstSlide = store.slides[0];

    await (store as unknown as { reviseCurrentSlide: (instruction: string) => Promise<unknown> }).reviseCurrentSlide("改成结论式标题");

    expect(store.slides[0]).toBe(firstSlide);
    expect(store.slides[0].title).toBe("封面");
    expect(store.slides[1]).toEqual(returnedSlides[1]);
  });
});
