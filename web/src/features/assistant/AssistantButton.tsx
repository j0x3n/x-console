import { Sparkles } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { useAiSessions } from "./api";
import { sessionBadge } from "./sessions";
import { useAssistant } from "./store";
import "./i18n";

/**
 * B183：顶栏的 AI 按钮，⌘J 也能开关。有等你确认的操作时显示数量，
 * 有 Agent 在工作时显示一个会动的点。
 */
export default function AssistantButton() {
  const t = useT();
  const open = useAssistant((s) => s.open);
  const toggle = useAssistant((s) => s.toggle);
  const sessions = useAiSessions();
  const { pending, working } = sessionBadge(sessions.data);
  const hint = [
    pending ? t("{n} waiting for you").replace("{n}", String(pending)) : "",
    working ? t("{n} at work").replace("{n}", String(working)) : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const label = `${t("AI")} (⌘J)${hint ? ` · ${hint}` : ""}`;
  return (
    <button
      type="button"
      className={`icon-button ai-topbar${open ? " on" : ""}`}
      aria-label={label}
      title={label}
      aria-expanded={open}
      onClick={toggle}
    >
      <Sparkles size={16} />
      {pending > 0 ? (
        <span className="ai-topbar-count">{pending > 9 ? "9+" : pending}</span>
      ) : (
        working > 0 && <i className="ai-topbar-dot" aria-hidden />
      )}
    </button>
  );
}
