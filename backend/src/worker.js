import { createRuntimeStore } from "./runtime-store.js";
import { Platform } from "./platform.js";
import { RabbitTaskQueue } from "./queue.js";
import { RedisCache } from "./redis.js";
import { ObjectStore } from "./object-store.js";
import { ModelGateway } from "./model-gateway.js";

const store = await createRuntimeStore();
const cache = new RedisCache();
const objectStore = new ObjectStore();
const modelGateway = new ModelGateway();
const platform = new Platform(store, { cache, objectStore, modelGateway, workerMode: true });
const queue = new RabbitTaskQueue();

console.log("先知 AI 生成任务 Worker 已启动");

await queue.consume(async (message) => {
  let task = null;
  for (let attempt = 0; attempt < 5 && !task; attempt += 1) {
    await store.refresh();
    task = store.state.generationTasks.find((item) => item.id === message.taskId);
    if (!task) await new Promise((resolve) => setTimeout(resolve, 150));
  }
  if (!task) throw new Error(`Generation task ${message.taskId} is not visible in the runtime store`);
  await platform.executeGatewayGeneration(message.taskId);
  await store.flush();
});
