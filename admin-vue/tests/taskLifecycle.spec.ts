import { describe, expect, it } from "vitest";
import { adminTaskPollingDelay, adminTaskStatusLabel, normalizeAdminTaskStatus } from "../src/utils/taskLifecycle";

describe("task lifecycle UX", () => {
  it("keeps unknown states non-failed and localizes lifecycle states", () => {
    expect(normalizeAdminTaskStatus("provider_paused")).toBe("unknown");
    expect(adminTaskStatusLabel("manual_review")).toBe("人工审核中");
    expect(adminTaskStatusLabel("cancel_requested")).toBe("取消中");
  });
  it("uses progressive polling windows", () => {
    const now = Date.now();
    expect(adminTaskPollingDelay(now - 60_000, now)).toBe(5_000);
    expect(adminTaskPollingDelay(now - 6 * 60_000, now)).toBe(20_000);
    expect(adminTaskPollingDelay(now - 16 * 60_000, now)).toBe(60_000);
  });
});
