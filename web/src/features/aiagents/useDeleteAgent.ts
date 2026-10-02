import { confirmAction } from "../../components/ui/ConfirmDialog";
import { useT } from "../../contexts/LanguageContext";
import { useAgentMutations, type AiAgent } from "./api";

/**
 * 删除 Agent：先二次确认，确认后才删（B86）。列表页和详情页共用。
 * 返回是否删掉了。正在跑任务时后端回 409，错误提示由全局处理。
 */
export function useDeleteAgent() {
  const t = useT();
  const { remove } = useAgentMutations();
  return async (a: AiAgent): Promise<boolean> => {
    const busy = a.runningTasks > 0;
    const ok = await confirmAction({
      title: `${t("Delete agent")}“${a.name}”？`,
      description: busy
        ? t("It is working on a task now. Stop the task first.")
        : t(
            "Its finished tasks stay. Cards keep it as a member until you remove it.",
          ),
      confirmLabel: t("Delete"),
    });
    if (!ok) return false;
    try {
      await remove.mutateAsync(a.id);
      return true;
    } catch {
      return false;
    }
  };
}
