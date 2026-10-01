import { Timer } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useCurrentFocus, useStartFocus } from "./api";
import { useFocusPanel } from "./hooks";
import "./i18n";

/** Issue 详情页上的“开始专注”：直接开始 25 分钟，关联这个 Issue。 */
export default function StartFocusButton({
  issueKey,
  compact,
}: {
  issueKey: string;
  /** 只显示图标（B55，卡片详情右栏用） */
  compact?: boolean;
}) {
  const t = useT();
  const current = useCurrentFocus();
  const start = useStartFocus();
  const openFor = useFocusPanel((s) => s.openFor);
  const running = current.data ?? null;
  const onThis = running?.issueKey === issueKey;
  return (
    <button
      className="xc-btn"
      disabled={start.isPending || onThis}
      aria-label={
        compact ? (onThis ? t("Focusing") : t("Start focus")) : undefined
      }
      title={
        running && !onThis
          ? t("Another focus session is running")
          : compact
            ? onThis
              ? t("Focusing")
              : t("Start focus")
            : undefined
      }
      onClick={() => {
        if (running) {
          openFor(issueKey);
          return;
        }
        start.mutate(
          { minutes: 25, issueKey },
          {
            onSuccess: () => toast(`${t("Focus started")} · 25 ${t("min")}`),
            onError: (err) =>
              toast({ message: errorMessage(err), tone: "error" }),
          },
        );
      }}
    >
      <Timer size={14} />
      {!compact && (onThis ? t("Focusing") : t("Start focus"))}
    </button>
  );
}
