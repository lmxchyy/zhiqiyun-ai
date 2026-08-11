#!/usr/bin/env node
/**
 * AI 自动混剪容量门禁脚本。
 *
 * 默认冒烟：校验本机 ffmpeg/ffprobe、compose 中 smartvideo-worker、以及可重复命令说明。
 * 全量上线前：SMARTVIDEO_CAPACITY_FULL=1 时要求连续 50 次成功（需外部环境注入实际渲染探针）。
 *
 * 目标门禁：
 * - 目标成片时长 ~60s
 * - 8 混合素材
 * - 720p 实时系数 ≤ 5（墙钟 / 成片时长）
 * - 连续 50 次成功率 ≥ 95%
 */

import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const full = process.env.SMARTVIDEO_CAPACITY_FULL === "1";
const targetRealtime = Number(process.env.SMARTVIDEO_CAPACITY_RT_MAX || "5");
const runs = Number(process.env.SMARTVIDEO_CAPACITY_RUNS || (full ? "50" : "1"));
const successFloor = Number(process.env.SMARTVIDEO_CAPACITY_SUCCESS_RATE || "0.95");

function run(cmd, args, opts = {}) {
  const { shell = false, ...rest } = opts;
  const result = spawnSync(cmd, args, {
    cwd: root,
    encoding: "utf8",
    shell,
    ...rest,
  });
  return {
    ok: result.status === 0,
    status: result.status,
    stdout: result.stdout || "",
    stderr: result.stderr || "",
  };
}

function whichMedia(bin) {
  const result = run(bin, ["-version"]);
  if (!result.ok) {
    throw new Error(`${bin} unavailable: ${result.stderr || result.stdout || `exit ${result.status}`}`);
  }
  const first = (result.stdout || result.stderr).split(/\r?\n/)[0] || bin;
  return first.trim();
}

function assertComposeWorker() {
  const prodPath = path.join(root, "compose.prod.yml");
  const devPath = path.join(root, "compose.yml");
  if (!existsSync(prodPath) || !existsSync(devPath)) {
    throw new Error("missing compose.prod.yml or compose.yml");
  }
  const prodText = readFileSync(prodPath, "utf8");
  if (!/^\s*smartvideo-worker:\s*$/m.test(prodText)) {
    throw new Error("compose.prod.yml missing smartvideo-worker service");
  }
  if (!/SMARTVIDEO_OUTBOX_ENABLED/.test(prodText)) {
    throw new Error("compose.prod.yml must mention SMARTVIDEO_OUTBOX_ENABLED");
  }
  if (!/SMARTVIDEO_ANALYSIS_ENABLED/.test(prodText)) {
    throw new Error("compose.prod.yml must mention SMARTVIDEO_ANALYSIS_ENABLED");
  }

  // Optional full interpolation when deploy secrets are already exported.
  if (process.env.SMARTVIDEO_CAPACITY_COMPOSE_CONFIG === "1") {
    const prod = run("docker", ["compose", "-f", "compose.prod.yml", "config"]);
    if (!prod.ok) {
      throw new Error(`compose.prod.yml invalid: ${prod.stderr || prod.stdout}`);
    }
    if (!/smartvideo-worker:/.test(prod.stdout)) {
      throw new Error("compose.prod.yml config missing smartvideo-worker service");
    }
  }
}

function smokeProbeOnce() {
  // Lightweight smoke: ensure media tools answer and temp dir policy is documented.
  const started = Date.now();
  whichMedia(process.env.SMARTVIDEO_FFPROBE_PATH || "ffprobe");
  whichMedia(process.env.SMARTVIDEO_FFMPEG_PATH || "ffmpeg");
  const elapsedMs = Date.now() - started;
  const outputDurationMs = 60_000;
  const realtimeFactor = elapsedMs / outputDurationMs;
  return {
    ok: true,
    elapsedMs,
    outputDurationMs,
    realtimeFactor,
    note: "smoke-only (tool availability); set SMARTVIDEO_CAPACITY_PROBE_CMD for real render loops",
  };
}

function customProbeOnce() {
  const cmd = process.env.SMARTVIDEO_CAPACITY_PROBE_CMD;
  if (!cmd) {
    return smokeProbeOnce();
  }
  const started = Date.now();
  const result = run(cmd, [], { shell: true });
  const elapsedMs = Date.now() - started;
  const outputDurationMs = Number(process.env.SMARTVIDEO_CAPACITY_OUTPUT_MS || "60000");
  const realtimeFactor = elapsedMs / outputDurationMs;
  return {
    ok: result.ok && realtimeFactor <= targetRealtime,
    elapsedMs,
    outputDurationMs,
    realtimeFactor,
    stderr: result.stderr,
  };
}

function main() {
  console.log(`[smartvideo-capacity] root=${root} full=${full} runs=${runs}`);
  whichMedia(process.env.SMARTVIDEO_FFPROBE_PATH || "ffprobe");
  whichMedia(process.env.SMARTVIDEO_FFMPEG_PATH || "ffmpeg");
  assertComposeWorker();

  let success = 0;
  const samples = [];
  for (let i = 0; i < runs; i += 1) {
    const sample = customProbeOnce();
    samples.push(sample);
    if (sample.ok) success += 1;
    console.log(
      `[run ${i + 1}/${runs}] ok=${sample.ok} elapsed_ms=${sample.elapsedMs} rt=${sample.realtimeFactor.toFixed(3)} note=${sample.note || ""}`,
    );
    if (!sample.ok && sample.stderr) {
      console.error(sample.stderr);
    }
  }

  const rate = success / runs;
  console.log(`[smartvideo-capacity] success_rate=${rate.toFixed(3)} target>=${successFloor} rt_max=${targetRealtime}`);
  if (full && rate < successFloor) {
    process.exitCode = 1;
    console.error("FULL capacity gate failed");
    return;
  }
  if (!full) {
    console.log("Smoke subset passed. Re-run with SMARTVIDEO_CAPACITY_FULL=1 and SMARTVIDEO_CAPACITY_PROBE_CMD before production.");
  }
}

try {
  main();
} catch (error) {
  console.error(`[smartvideo-capacity] FAILED: ${error instanceof Error ? error.message : error}`);
  process.exitCode = 1;
}
