import net from "node:net";

function tcpCheck(rawUrl, fallbackPort) {
  if (!rawUrl) return Promise.resolve({ configured: false, reachable: false });
  const url = new URL(rawUrl);
  const port = Number(url.port || fallbackPort);
  return new Promise((resolve) => {
    const socket = net.createConnection({ host: url.hostname, port });
    const done = (reachable, error = null) => {
      socket.destroy();
      resolve({ configured: true, reachable, host: url.hostname, port, error });
    };
    socket.setTimeout(800);
    socket.once("connect", () => done(true));
    socket.once("timeout", () => done(false, "timeout"));
    socket.once("error", (error) => done(false, error.code || error.message));
  });
}

async function httpCheck(rawUrl, path) {
  if (!rawUrl) return { configured: false, reachable: false };
  try {
    const response = await fetch(`${rawUrl}${path}`, { signal: AbortSignal.timeout(1000) });
    return { configured: true, reachable: response.ok, url: rawUrl, status: response.status };
  } catch (error) {
    return { configured: true, reachable: false, url: rawUrl, error: error.cause?.code || error.message };
  }
}

export async function infrastructureHealth() {
  const dependencies = {
    postgres: await tcpCheck(process.env.DATABASE_URL, 5432),
    redis: await tcpCheck(process.env.REDIS_URL, 6379),
    rabbitmq: await tcpCheck(process.env.RABBITMQ_URL, 5672),
    minio: await httpCheck(process.env.S3_ENDPOINT, "/minio/health/live")
  };
  const configured = Object.values(dependencies).filter((item) => item.configured);
  return {
    status: configured.every((item) => item.reachable) ? "UP" : "DEGRADED",
    time: new Date().toISOString(),
    dependencies
  };
}
