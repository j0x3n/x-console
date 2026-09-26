import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ClipboardCopy, ClipboardPaste, ExternalLink, Lock, Moon, Power, RotateCcw, X } from "lucide-react";
import { errorMessage, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { hostsApi, type HostDetail, type PowerAction } from "../servers/api";
import { loadRecent, pushRecent, saveRecent } from "./recent";

function useCan(host: HostDetail, cap: string) {
  return host.online && host.capabilities.includes(cap);
}

/** 剪贴板：把手机上的文字发到电脑，或者取回电脑上的文字。 */
export function ClipboardCard({ host }: { host: HostDetail }) {
  const t = useT();
  const [text, setText] = useState("");
  const can = useCan(host, "clipboard");
  const send = useMutation({
    mutationFn: () =>
      unwrap(hostsApi.PUT("/hosts/{hostId}/clipboard", { params: { path: { hostId: host.id } }, body: { text } })),
    onSuccess: () => toast(t("Sent to the PC clipboard")),
    onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
  });
  const fetchIt = useMutation({
    mutationFn: () => unwrap(hostsApi.GET("/hosts/{hostId}/clipboard", { params: { path: { hostId: host.id } } })),
    onSuccess: async (data) => {
      setText(data.text);
      try {
        await navigator.clipboard.writeText(data.text);
        toast(t("Copied to this device"));
      } catch {
        toast(t("Fetched from the PC"));
      }
    },
    onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
  });
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Clipboard")}</h2>
      </div>
      <textarea
        className="xc-textarea pc-clip"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={t("Type or paste text to send to the PC")}
        aria-label={t("Clipboard")}
      />
      <div className="xc-row pc-actions">
        <button className="xc-btn primary" disabled={!can || !text || send.isPending} onClick={() => send.mutate()}>
          <ClipboardPaste size={14} /> {t("Send to PC")}
        </button>
        <button className="xc-btn" disabled={!can || fetchIt.isPending} onClick={() => fetchIt.mutate()}>
          <ClipboardCopy size={14} /> {t("Get from PC")}
        </button>
        {text && (
          <button className="xc-btn ghost" onClick={() => setText("")} aria-label={t("Clear")}>
            <X size={14} />
          </button>
        )}
      </div>
    </div>
  );
}

const powerButtons: { action: PowerAction; label: string; icon: typeof Lock; danger?: boolean }[] = [
  { action: "lock", label: "Lock screen", icon: Lock },
  { action: "sleep", label: "Sleep", icon: Moon },
  { action: "restart", label: "Restart", icon: RotateCcw, danger: true },
  { action: "shutdown", label: "Shut down", icon: Power, danger: true },
];

/** 快捷操作：锁屏、睡眠、重启、关机，打开程序或网址。 */
export function QuickActionsCard({ host }: { host: HostDetail }) {
  const t = useT();
  const canPower = useCan(host, "power");
  const canOpen = useCan(host, "open");
  const [target, setTarget] = useState("");
  const [recent, setRecent] = useState<string[]>(loadRecent);
  const power = useMutation({
    mutationFn: (action: PowerAction) =>
      withElevation(() =>
        unwrap(hostsApi.POST("/hosts/{hostId}/power", { params: { path: { hostId: host.id } }, body: { action } })),
      ),
    onSuccess: () => toast(t("Done")),
    onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
  });
  const open = useMutation({
    mutationFn: (value: string) =>
      unwrap(hostsApi.POST("/hosts/{hostId}/open", { params: { path: { hostId: host.id } }, body: { target: value } })),
    onSuccess: (_, value) => {
      const next = pushRecent(recent, value);
      setRecent(next);
      saveRecent(next);
      toast(t("Opened on the PC"));
    },
    onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
  });
  const onPower = (action: PowerAction, label: string, danger?: boolean) => {
    if (danger && !confirm(`${t(label)}?`)) return;
    power.mutate(action);
  };
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Quick actions")}</h2>
      </div>
      <div className="pc-power">
        {powerButtons.map((b) => (
          <button
            key={b.action}
            className={`xc-btn${b.danger ? " danger" : ""}`}
            disabled={!canPower || power.isPending}
            onClick={() => onPower(b.action, b.label, b.danger)}
          >
            <b.icon size={15} /> {t(b.label)}
          </button>
        ))}
      </div>
      <form
        className="xc-row pc-open"
        onSubmit={(e) => {
          e.preventDefault();
          if (target.trim()) open.mutate(target.trim());
        }}
      >
        <input
          className="xc-input"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder={t("Program, file or URL, e.g. notepad or https://…")}
          aria-label={t("Open on PC")}
        />
        <button className="xc-btn" disabled={!canOpen || !target.trim() || open.isPending}>
          <ExternalLink size={14} /> {t("Open")}
        </button>
      </form>
      {recent.length > 0 && (
        <div className="pc-recent">
          {recent.map((r) => (
            <button key={r} className="xc-badge" disabled={!canOpen || open.isPending} onClick={() => open.mutate(r)} title={r}>
              {r}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
