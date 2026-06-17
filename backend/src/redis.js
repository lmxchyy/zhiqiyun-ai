import { createClient } from "redis";

export class RedisCache {
  constructor(rawUrl = process.env.REDIS_URL) {
    this.url = rawUrl;
    this.client = null;
    this.connecting = null;
  }

  async connect() {
    if (!this.url) return null;
    if (this.client?.isReady) return this.client;
    if (this.connecting) return this.connecting;
    this.client = createClient({ url: this.url });
    this.client.on("error", (error) => console.error("Redis 缓存错误:", error.message));
    this.connecting = this.client.connect().then(() => this.client).finally(() => { this.connecting = null; });
    return this.connecting;
  }

  async setJson(key, value, ttlSeconds = 3600) {
    const client = await this.connect();
    if (client) await client.set(key, JSON.stringify(value), { EX: ttlSeconds });
  }

  async delete(key) {
    const client = await this.connect();
    if (client) await client.del(key);
  }
}
