import { useWorkspaceStore } from "../stores/workspace-store";
import { outcomeText } from "../lib/outcomeText";
import type { DecisionHandler, Language, Notify } from "../types/domain";

export function useWorkspaceState(language: Language, showToast: Notify) {
  const state = useWorkspaceStore();
  const onAction: DecisionHandler = (item, action, details) => {
    const entry = state.applyDecision(item, action, details);
    if (!entry) return;
    showToast({
      message: outcomeText(entry, language),
      subtitle:
        action === "reassigned"
          ? item.company
          : `${item.company} · ${language === "zh" ? "已从今日待办移除" : "removed from Today"}`,
      agent: item.agent,
      undoKey: entry.key,
    });
  };
  const onUndoDecision = (key: string) => {
    const entry = state.undoDecision(key);
    if (entry)
      showToast({
        message: language === "zh" ? "操作已撤销" : "Action undone",
        agent: entry.item.agent,
      });
  };
  return { ...state, onAction, onUndoDecision };
}
