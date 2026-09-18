import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { GENERATION_QUEUED_LABEL, GENERATION_QUEUED_TOAST, generationDisplayStatus, isGenerationQueued } from "../packages/shared-types/src/generationStatus.ts";
const source = path => readFileSync(new URL(`../${path}`, import.meta.url), "utf8");

test("queue copy and canonical status are shared, terminals retain truth", () => {
  assert.equal(GENERATION_QUEUED_LABEL, "排队中 · 等待运行位");
  assert.equal(GENERATION_QUEUED_TOAST, "任务已排队，等待运行");
  assert.equal(generationDisplayStatus({status:"PROCESSING",taskStatus:"QUEUED"}), "QUEUED");
  for(const status of ["RUNNING","DISPATCHING","SUCCEEDED","FAILED","CANCELLED"]) {
    assert.equal(generationDisplayStatus({status}), status);
    assert.equal(isGenerationQueued({status}), false);
  }
});
test("web image/video submissions and mini submissions use queued response semantics", () => {
  const web=source("admin-vue/src/App.vue");
  assert.match(web, /isGenerationQueued\(createdTask\) \? GENERATION_QUEUED_TOAST/);
  assert.match(web, /isGenerationQueued\(task\) \? GENERATION_QUEUED_TOAST/);
  assert.match(web, /isGenerationQueued\(task\) \? GENERATION_QUEUED_LABEL/);
  const mini=source("apps/user-uni/src/components/MiniProgramRoleWorkbench.vue");
  assert.match(mini, /taskStatus === "QUEUED" \? GENERATION_QUEUED_TOAST/);
  assert.match(mini, /latestGenerationTask.value\?\.status !== "QUEUED" && generationProgress/);
  assert.match(source("apps/user-uni/src/features/assets/api.ts"), /generationDisplayStatus\(raw\)/);
});
test("both runtimes use the same startup gate; consumers are independent", () => {
  for(const path of ["backend-go/cmd/api/async_runtime.go","backend-go/cmd/generation-worker/main.go"]) {
    const text=source(path); assert.match(text,/RunConfiguredGenerationScheduler/);
    assert.match(text,/RunGenerationVideoCanaryWorker/);
  }
  assert.match(source("backend-go/cmd/generation-worker/main.go"),/net.Listen\("tcp", cfg.GenerationWorkerMetricsAddr\)/);
  assert.match(source("backend-go/internal/app/generation/service.go"),/FairScheduler\s+bool\s+`json:"-"`/);
});
