import { createRuntimeStore } from "./runtime-store.js";
import { Platform } from "./platform.js";
import { GeoSource } from "./geo-source.js";

const store = await createRuntimeStore();
const platform = new Platform(store, { geoSource: new GeoSource() });
const intervalMs = Math.max(5000, Number(process.env.GEO_SCHEDULER_INTERVAL_MS || 30000));

async function tick() {
  await store.refresh();
  const tasks = await platform.runDueGeoSchedules();
  await store.flush();
  if (tasks.length) console.log(`GEO scheduler completed ${tasks.length} monitor task(s)`);
}

console.log(`GEO scheduler started with ${intervalMs}ms interval`);
await tick();
setInterval(() => tick().catch((error) => console.error("GEO scheduler tick failed", error)), intervalMs);
