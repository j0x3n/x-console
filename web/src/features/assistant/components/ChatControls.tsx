import { useEffect, useRef, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useAiModels,
  useConversationSettings,
  type AiModel,
  type AiPermission,
  type ChatSettings,
} from "../api";

const PERMISSIONS: { value: AiPermission; label: string }[] = [
  { value: "manual", label: "Manual permission" },
  { value: "write", label: "Write permission" },
  { value: "all", label: "Allow all" },
];
const EFFORTS = [
  { value: "off", label: "Off" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

/**
 * 浮窗输入框上方的三个下拉框（B60）：权限、模型、思考程度。
 * 有对话时改了马上存，下一条消息生效；还没有对话时改的是新对话的设置。
 * 浮窗太窄时收进一个“…”按钮。
 */
export default function ChatControls({
  conversationId,
  value,
  defaultModel,
  onDraft,
}: {
  conversationId: number | null;
  value: ChatSettings & { until?: string | null };
  defaultModel: string;
  onDraft: (next: ChatSettings) => void;
}) {
  const t = useT();
  const models = useAiModels();
  const save = useConversationSettings();
  const ref = useRef<HTMLDivElement>(null);
  const [compact, setCompact] = useState(false);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const el = ref.current?.parentElement;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(([entry]) =>
      setCompact(entry.contentRect.width < 340),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const change = (patch: Partial<ChatSettings>) => {
    const next = { ...value, ...patch };
    if (conversationId == null) {
      onDraft(next);
      return;
    }
    save.mutate(
      { id: conversationId, body: patch },
      {
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  const all = value.permission === "all";
  const selects = (
    <>
      <select
        className={`xc-select small ai-control${all ? " warn" : ""}`}
        aria-label={t("Permission")}
        value={value.permission}
        onChange={(e) => change({ permission: e.target.value as AiPermission })}
      >
        {PERMISSIONS.map((p) => (
          <option key={p.value} value={p.value}>
            {t(p.label)}
          </option>
        ))}
      </select>
      <select
        className="xc-select small ai-control"
        aria-label={t("Model")}
        value={value.model}
        onChange={(e) => change({ model: e.target.value })}
      >
        <option value="">
          {t("Default")}
          {defaultModel ? `（${defaultModel}）` : ""}
        </option>
        {(models.data ?? []).map((m: AiModel) => (
          <option
            key={`${m.providerId}:${m.id}`}
            value={`${m.providerId}:${m.id}`}
          >
            {m.name ?? m.id}
          </option>
        ))}
      </select>
      <select
        className="xc-select small ai-control"
        aria-label={t("Reasoning effort")}
        value={value.effort}
        onChange={(e) => change({ effort: e.target.value })}
      >
        <option value="">{t("Default")}</option>
        {EFFORTS.map((e) => (
          <option key={e.value} value={e.value}>
            {t(e.label)}
          </option>
        ))}
      </select>
      {all && (
        <small className="ai-control-note">{t("Valid for 2 hours")}</small>
      )}
    </>
  );

  return (
    <div className="ai-controls" ref={ref}>
      {compact ? (
        <>
          <button
            type="button"
            className={`icon-button${all ? " warn" : ""}`}
            aria-label={t("Chat settings")}
            title={t("Chat settings")}
            aria-expanded={open}
            onClick={() => setOpen((v) => !v)}
          >
            <MoreHorizontal size={15} />
          </button>
          {open && <div className="ai-controls-pop">{selects}</div>}
        </>
      ) : (
        selects
      )}
    </div>
  );
}
