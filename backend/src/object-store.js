import { Client } from "minio";

const bucket = "xianzhi-assets";

export class ObjectStore {
  constructor({ endpoint = process.env.S3_ENDPOINT, accessKey = process.env.S3_ACCESS_KEY, secretKey = process.env.S3_SECRET_KEY } = {}) {
    this.enabled = Boolean(endpoint && accessKey && secretKey);
    if (!this.enabled) return;
    const url = new URL(endpoint);
    this.client = new Client({
      endPoint: url.hostname,
      port: Number(url.port || (url.protocol === "https:" ? 443 : 80)),
      useSSL: url.protocol === "https:",
      accessKey,
      secretKey
    });
    this.ready = null;
  }

  async ensure() {
    if (!this.enabled) return false;
    if (!this.ready) {
      this.ready = this.client.bucketExists(bucket).then(async (exists) => {
        if (!exists) await this.client.makeBucket(bucket);
        return true;
      });
    }
    return this.ready;
  }

  async put(key, content, contentType) {
    await this.ensure();
    const buffer = Buffer.isBuffer(content) ? content : Buffer.from(content);
    await this.client.putObject(bucket, key, buffer, buffer.length, { "Content-Type": contentType });
    return key;
  }

  async get(key) {
    await this.ensure();
    return this.client.getObject(bucket, key);
  }
}
