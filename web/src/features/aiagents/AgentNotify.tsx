import { useState } from "react";
import { Bell } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import Switch from "../../components/ui/Switch";
import { Loading, NotLive } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useAgentNotify, useSaveAgentNotify, type AiAgentNotify } from "./api";
import "./i18n";
import "./aiagents.css";

/** B87：六类 Agent 通知，顺序和用户说的一样。 */
const KINDS: { key: keyof AiAgentNotify; label: string; hint: string }[] = [
  {
    key: "received",
    label: "Got a task",
    hint: "A card is assigned to an agent.",
  },
  {
    key: "started",
    label: "Started a task",
    hint: "The agent starts working.",
  },
  {
    key: "decision",
    label: "Needs your decision",
    hint: "The agent asks for permission or asks you a question, or a coding task waits for review.",
  },
  {
    key: "prOpened",
    label: "Opened a pull request",
    hint: "The agent opened a pull request.",
  },
  { key: "done", label: "Finished a task", hint: "The agent is done." },
  {
    key: "failed",
    label: "Failed or stopped",
    hint: "The task failed, timed out or was stopped.",
  },
];

/** Agent 通知的开关，改了马上保存。设置 → 通知和 Agent 页的“通知”弹窗共用。 */
export function AgentNotifyFields() {
  const t = useT();
  const notify = useAgentNotify();
  const save = useSaveAgentNotify();
  if (notify.isPending) return <Loading />;
  if (notify.isError)
    return isNotLive(notify.error) ? (
      <NotLive name="Agent 通知" />
    ) : (
      <p className="xc-error-text">{errorMessage(notify.error)}</p>
    );
  const value = notify.data;
  return (
    <ul className="aiagent-notify">
      {KINDS.map((k) => (
        <li key={k.key}>
          <span>
            <strong>{t(k.label)}</strong>
            <small>{t(k.hint)}</small>
          </span>
          <Switch
            checked={value[k.key]}
            label={t(k.label)}
            disabled={save.isPending}
            onChange={(checked) =>
              save.mutate(
                { ...value, [k.key]: checked },
                {
                  onSuccess: () => toast(t("Saved")),
                  onError: (e) =>
                    toast({ message: errorMessage(e), tone: "error" }),
                },
              )
            }
          />
        </li>
      ))}
    </ul>
  );
}

/** 设置 → 通知里的卡片。 */
export function AgentNotifyCard() {
  const t = useT();
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Agent notifications")}</h2>
      </div>
      <p className="xc-muted notify-help">
        {t("Where they go follows the rules above.")}
      </p>
      <AgentNotifyFields />
    </section>
  );
}

/** Agent 页顶栏的“通知”按钮。 */
export function AgentNotifyButton() {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        className="xc-btn"
        title={t("Agent notifications")}
        onClick={() => setOpen(true)}
      >
        <Bell size={14} /> <span>{t("Notifications")}</span>
      </button>
      {open && (
        <Dialog
          open
          onClose={() => setOpen(false)}
          title={t("Agent notifications")}
        >
          <AgentNotifyFields />
        </Dialog>
      )}
    </>
  );
}
