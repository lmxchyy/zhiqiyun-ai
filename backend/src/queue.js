import amqp from "amqplib";

const queueName = "generation_tasks";

export class RabbitTaskQueue {
  constructor(rawUrl = process.env.RABBITMQ_URL) {
    this.url = rawUrl;
    this.connection = null;
    this.channel = null;
  }

  async connect() {
    if (this.channel) return this.channel;
    if (!this.url) throw new Error("RabbitMQ 未配置");
    this.connection = await amqp.connect(this.url);
    this.connection.on("close", () => { this.connection = null; this.channel = null; });
    this.connection.on("error", () => {});
    this.channel = await this.connection.createChannel();
    await this.channel.assertQueue(queueName, { durable: true });
    return this.channel;
  }

  async publish(taskId) {
    const channel = await this.connect();
    const routed = channel.sendToQueue(queueName, Buffer.from(JSON.stringify({ taskId })), {
      persistent: true,
      contentType: "application/json"
    });
    if (!routed) await new Promise((resolve) => channel.once("drain", resolve));
  }

  async consume(handler) {
    const channel = await this.connect();
    await channel.prefetch(1);
    return channel.consume(queueName, async (message) => {
      if (!message) return;
      try {
        await handler(JSON.parse(message.content.toString("utf8")));
        channel.ack(message);
      } catch (error) {
        console.error("Worker 处理失败:", error.message);
        channel.nack(message, false, false);
      }
    });
  }
}
