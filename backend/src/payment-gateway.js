import crypto from "node:crypto";

const supportedChannels = new Set(["wechat", "alipay"]);

export class PaymentGateway {
  constructor({
    defaultSecret = process.env.PAYMENT_CALLBACK_SECRET,
    wechatSecret = process.env.WECHAT_PAY_CALLBACK_SECRET,
    alipaySecret = process.env.ALIPAY_CALLBACK_SECRET
  } = {}) {
    this.secrets = { wechat: wechatSecret || defaultSecret, alipay: alipaySecret || defaultSecret };
  }

  canonical(channel, payload) {
    return [channel, payload.eventId, payload.orderId, Number(payload.amount), payload.status].join("|");
  }

  sign(channel, payload) {
    if (!supportedChannels.has(channel)) throw new Error("Payment channel is not supported");
    const secret = this.secrets[channel];
    if (!secret) throw new Error("Payment callback secret is not configured");
    return crypto.createHmac("sha256", secret).update(this.canonical(channel, payload)).digest("hex");
  }

  verify(channel, payload, signature) {
    const expected = this.sign(channel, payload);
    const received = String(signature || "");
    if (received.length !== expected.length || !crypto.timingSafeEqual(Buffer.from(received), Buffer.from(expected))) {
      throw new Error("Payment callback signature is invalid");
    }
    return true;
  }
}
