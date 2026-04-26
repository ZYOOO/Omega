import type { FeishuApprovalCard, FeishuDeliveryNotice } from "./feishuAdapter";

export type FeishuCard = FeishuApprovalCard | FeishuDeliveryNotice;

export interface FeishuClient {
  sendCard(card: FeishuCard): Promise<void>;
}

export class InMemoryFeishuClient implements FeishuClient {
  public readonly cards: FeishuCard[] = [];

  async sendCard(card: FeishuCard): Promise<void> {
    this.cards.push(card);
  }
}
