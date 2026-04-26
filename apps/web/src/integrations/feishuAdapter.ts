export interface FeishuApprovalCardInput {
  missionId: string;
  operationTitle: string;
  reason: string;
}

export interface FeishuApprovalCard {
  cardType: "approval";
  title: string;
  missionId: string;
  actions: Array<"Approve" | "Request changes" | "Pause">;
  body: string;
}

export interface FeishuDeliveryNoticeInput {
  missionId: string;
  summary: string;
  proofFiles: string[];
}

export interface FeishuDeliveryNotice {
  cardType: "notice";
  title: string;
  body: string;
  proofFiles: string[];
}

export function createFeishuApprovalCard(input: FeishuApprovalCardInput): FeishuApprovalCard {
  return {
    cardType: "approval",
    title: `${input.operationTitle} checkpoint`,
    missionId: input.missionId,
    actions: ["Approve", "Request changes", "Pause"],
    body: input.reason
  };
}

export function createFeishuDeliveryNotice(input: FeishuDeliveryNoticeInput): FeishuDeliveryNotice {
  return {
    cardType: "notice",
    title: `Mission ${input.missionId} delivery update`,
    body: input.summary,
    proofFiles: input.proofFiles
  };
}
