export interface WorksPerformanceFields {
  serialWait?: boolean;
  source?: string;
  requestUrl?: string;
  duplicate?: boolean;
  cacheHit?: boolean;
  itemCount?: number;
  note?: string;
}

export interface WorksPerformanceEvent extends WorksPerformanceFields {
  step: string;
  startTime: string;
  endTime: string;
  durationMs: number;
}

interface WorksPerformanceRuntime {
  __XIANZHI_WORKS_PERF__?: boolean;
  __XIANZHI_WORKS_PERF_EVENTS__?: WorksPerformanceEvent[];
  __XIANZHI_WORKS_TAB_CLICK_AT__?: number;
}

const runtime = globalThis as typeof globalThis & WorksPerformanceRuntime;
const rawEnv = (import.meta as unknown as { env?: Record<string, unknown> }).env || {};

export function worksPerformanceEnabled() {
  return Boolean(rawEnv.DEV || runtime.__XIANZHI_WORKS_PERF__);
}

export function worksTabClickStartedAt() {
  const startedAt = Number(runtime.__XIANZHI_WORKS_TAB_CLICK_AT__ || 0);
  return startedAt > 0 && Date.now() - startedAt <= 15000 ? startedAt : 0;
}

export function beginWorksPerformanceStep(step: string, fields: WorksPerformanceFields = {}) {
  const startedAt = Date.now();
  return {
    startedAt,
    end(overrides: WorksPerformanceFields = {}) {
      return recordWorksPerformance(step, startedAt, Date.now(), { ...fields, ...overrides });
    },
  };
}

export function recordWorksPerformance(
  step: string,
  startedAt: number,
  endedAt: number,
  fields: WorksPerformanceFields = {},
) {
  const event: WorksPerformanceEvent = {
    step,
    startTime: new Date(startedAt).toISOString(),
    endTime: new Date(endedAt).toISOString(),
    durationMs: Math.max(0, Number((endedAt - startedAt).toFixed(2))),
    serialWait: Boolean(fields.serialWait),
    source: fields.source || "",
    requestUrl: fields.requestUrl || "",
    duplicate: Boolean(fields.duplicate),
    cacheHit: Boolean(fields.cacheHit),
    itemCount: Number(fields.itemCount || 0),
    note: fields.note || "",
  };
  if (!worksPerformanceEnabled()) return event;
  const events = runtime.__XIANZHI_WORKS_PERF_EVENTS__ || [];
  events.push(event);
  if (events.length > 200) events.splice(0, events.length - 200);
  runtime.__XIANZHI_WORKS_PERF_EVENTS__ = events;
  console.info(`[works-perf] ${JSON.stringify(event)}`);
  return event;
}

export function clearWorksPerformanceEvents() {
  runtime.__XIANZHI_WORKS_PERF_EVENTS__ = [];
}
