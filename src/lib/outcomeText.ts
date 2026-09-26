import type { DecisionEntry, Language } from "../types/domain";
export function outcomeText(entry: DecisionEntry, language: Language) {
  const { item, action } = entry;
  const zh = language === "zh";
  if (item.kind === "draft") {
    return action === "approved"
      ? zh
        ? "已向 Priya Raman 发送跟进邮件"
        : "Follow-up sent to Priya Raman"
      : zh
        ? "已跳过跟进邮件"
        : "Follow-up skipped";
  }
  if (item.kind === "health") {
    if (action === "reassigned")
      return zh
        ? `续约沟通已交给 ${entry.assignee}`
        : `Check-in reassigned to ${entry.assignee}`;
    return action === "approved"
      ? zh
        ? "已批准 Oakline 续约沟通"
        : "Oakline renewal check-in approved"
      : zh
        ? "已忽略 Oakline 续约沟通"
        : "Oakline renewal check-in dismissed";
  }
  if (item.kind === "stage") {
    return action === "approved"
      ? zh
        ? "Brightwell Labs 已移至谈判阶段"
        : "Brightwell Labs moved to Negotiation"
      : zh
        ? "Brightwell Labs 保持方案阶段"
        : "Brightwell Labs kept in Proposal";
  }
  return zh ? "已处理决策" : "Decision handled";
}
