import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Clipboard, ClipboardCopy, ClipboardPaste, ExternalLink, Lock, Moon, Power, RotateCcw, X } from "lucide-react";
import { errorMessage, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { hostsApi, type HostDetail, type PowerAction } from "../servers/api";
import { loadRecent, pushRecent, saveRecent } from "./recent";

function useCan(host: HostDetail, cap: string) {
  return host.online && host.capabilities.includes(cap);
}

/** 剪贴板：把手机上的文字发到电脑，或者取回电脑上的文字。 */
function ClipboardBody({ host }: { host: HostDetail }) {
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
    <>
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
    </>
  );
}

const powerButtons: { action: PowerAction; label: string; hint: string; icon: typeof Lock; danger?: boolean }[] = [
  { action: "lock", label: "Lock screen", hint: "The screen of this PC will lock:", icon: Lock },
  { action: "sleep", label: "Sleep", hint: "This PC will go to sleep:", icon: Moon },
  {
    action: "restart",
    label: "Restart",
    hint: "This PC will restart. Unsaved work will be lost:",
    icon: RotateCcw,
    danger: true,
  },
  {
    action: "shutdown",
    label: "Shut down",
    hint: "This PC will shut down. Unsaved work will be lost:",
    icon: Power,
    danger: true,
  },
];

/** 打开程序、文件或网址，下面是最近打开过的。 */
function OpenBody({ host, onDone }: { host: HostDetail; onDone: () => void }) {
  const t = useT();
  const canOpen = useCan(host, "open");
  const [target, setTarget] = useState("");
  const [recent, setRecent] = useState<string[]>(loadRecent);
  const open = useMutation({
    mutationFn: (value: string) =>
      unwrap(hostsApi.POST("/hosts/{hostId}/open", { params: { path: { hostId: host.id } }, body: { target: value } })),
    onSuccess: (_, value) => {
      const next = pushRecent(recent, value);
      setRecent(next);
      saveRecent(next);
      toast(t("Opened on the PC"));
      onDone();
    },
    onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
  });
  return (
    <>
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
          autoFocus
          onChange={(e) => setTarget(e.target.value)}
          placeholder={t("Program, file or URL, e.g. notepad or https://…")}
          aria-label={t("Open on PC")}
        />
        <button className="xc-btn primary" disabled={!canOpen || !target.trim() || open.isPending}>
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
    </>
  );
}

/** 页头右边的一排按钮：锁屏、睡眠、重启、关机、打开、剪贴板。 */
export function PcQuickBar({ host }: { host: HostDetail }) {
  const t = useT();
  const canPower = useCan(host, "power");
  const [dialog, setDialog] = useState<"clipboard" | "open" | null>(null);
  const [asking, setAsking] = useState<(typeof powerButtons)[number] | null>(null);
  const power = useMutation({
    mutationFn: (action: PowerAction) =>
      withElevation(() =>
        unwrap(hostsApi.POST("/hosts/{hostId}/power", { params: { path: { hostId: host.id } }, body: { action } })),
      ),
    onSuccess: () => {
      toast(t("Done"));
      setAsking(null);
    },
    onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
  });
  return (
    <div className="pc-bar" role="toolbar" aria-label={t("Quick actions")}>
      <button className="xc-btn small" onClick={() => setDialog("clipboard")} title={t("Clipboard")}>
        <Clipboard size={14} /> <span>{t("Clipboard")}</span>
      </button>
      <button className="xc-btn small" onClick={() => setDialog("open")} title={t("Open on PC")}>
        <ExternalLink size={14} /> <span>{t("Open")}</span>
      </button>
      <span className="pc-bar-sep" aria-hidden="true" />
      {powerButtons.map((b) => (
        <button
          key={b.action}
          className={`xc-btn small${b.danger ? " danger" : ""}`}
          disabled={!canPower || power.isPending}
          title={t(b.label)}
          onClick={() => setAsking(b)}
        >
          <b.icon size={14} /> <span>{t(b.label)}</span>
        </button>
      ))}
      <Dialog
        open={asking !== null}
        onClose={() => setAsking(null)}
        title={asking ? `${t(asking.label)}？` : ""}
      >
        <p className="pc-confirm">
          {asking && t(asking.hint)} <b>{host.name}</b>
        </p>
        <div className="xc-dialog-actions">
          <button className="xc-btn ghost" onClick={() => setAsking(null)}>
            {t("Cancel")}
          </button>
          <button
            className={`xc-btn ${asking?.danger ? "danger" : "primary"}`}
            disabled={power.isPending}
            onClick={() => asking && power.mutate(asking.action)}
          >
            {asking && t(asking.label)}
          </button>
        </div>
      </Dialog>
      <Dialog open={dialog === "clipboard"} onClose={() => setDialog(null)} title={t("Clipboard")}>
        <ClipboardBody host={host} />
      </Dialog>
      <Dialog open={dialog === "open"} onClose={() => setDialog(null)} title={t("Open on PC")}>
        <OpenBody host={host} onDone={() => setDialog(null)} />
      </Dialog>
    </div>
  );
}
