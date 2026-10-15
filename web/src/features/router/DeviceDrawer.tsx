import { useEffect, useState, type FormEvent } from "react";
import { X } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Switch from "../../components/ui/Switch";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../lib/time";
import {
  COOLDOWNS,
  cooldownLabel,
  useForgetRouterDevice,
  useRouterOptions,
  useUpdateRouterDevice,
  type RouterDevice,
} from "./api";
import { sinceText } from "./DevicesView";

/** 备注最多 40 个字，和后端一致。 */
const NOTE_MAX = 40;

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** B228：一台设备的详情。备注、通知开关、再次通知间隔和设备信息。 */
export default function DeviceDrawer({
  device,
  onClose,
}: {
  device: RouterDevice;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const options = useRouterOptions();
  const update = useUpdateRouterDevice();
  const forget = useForgetRouterDevice();
  const [note, setNote] = useState(device.note);
  useEffect(() => setNote(device.note), [device.mac, device.note]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const save = (
    patch: Partial<{
      note: string;
      notify: boolean;
      cooldownMinutes: number | null;
    }>,
    done?: string,
  ) =>
    update.mutate(
      {
        mac: device.mac,
        body: {
          note: device.note,
          notify: device.notify,
          cooldownMinutes: device.cooldownMinutes,
          ...patch,
        },
      },
      { onSuccess: () => done && toast(done), onError: fail },
    );

  const onSaveNote = (e: FormEvent) => {
    e.preventDefault();
    if (note.trim() === device.note) return;
    save({ note: note.trim() }, t("Saved"));
  };

  const onForget = async () => {
    const ok = await confirmAction({
      title: t("Forget this device?"),
      description: "会删掉备注和它的流量记录。它再连上来时，面板当它是新设备。",
      confirmLabel: t("Forget device"),
    });
    if (!ok) return;
    forget.mutate(device.mac, {
      onSuccess: () => {
        toast(t("Device forgotten"));
        onClose();
      },
      onError: fail,
    });
  };

  const fallback = options.data?.notifyCooldownMinutes ?? 15;
  const globalOff = options.data && !options.data.deviceNotify;
  const when = (v: string | null | undefined) =>
    v ? `${formatDate(v, language)} ${formatTime(v, language)}` : "—";
  const conn =
    device.conn === "wifi"
      ? ["Wi-Fi", device.wifiIface].filter(Boolean).join(" · ")
      : device.conn === "wired"
        ? t("Wired")
        : "—";

  return (
    <aside
      className="router-drawer"
      role="dialog"
      aria-label={device.displayName}
    >
      <header className="router-drawer-head">
        <span className={`xc-dot ${device.online ? "ok" : ""}`} />
        <strong>{device.displayName}</strong>
        <span className="xc-spacer" />
        <button
          type="button"
          className="icon-button"
          aria-label={t("Close")}
          title={t("Close")}
          onClick={onClose}
        >
          <X size={16} />
        </button>
      </header>
      <div className="router-drawer-body">
        <p className="xc-muted router-drawer-state">
          {device.online ? t("Online") : t("Offline")} ·{" "}
          {sinceText(device, language, Date.now())}
        </p>
        <form className="xc-field" onSubmit={onSaveNote}>
          <span>{t("Device note")}</span>
          <div className="router-note-row">
            <input
              className="xc-input"
              value={note}
              maxLength={NOTE_MAX}
              placeholder={device.name ?? device.mac}
              onChange={(e) => setNote(e.target.value)}
            />
            <button
              className="xc-btn"
              disabled={update.isPending || note.trim() === device.note}
            >
              {t("Save")}
            </button>
          </div>
          <small>写了备注，页面和通知里都显示备注。</small>
        </form>
        <div className="router-drawer-row">
          <span>{t("Online and offline notifications")}</span>
          <Switch
            checked={device.notify}
            label={t("Online and offline notifications")}
            disabled={update.isPending}
            onChange={(notify) => save({ notify })}
          />
        </div>
        {globalOff && device.notify && (
          <p className="router-drawer-hint">
            设置里的“设备上下线通知”总开关关着，现在不会推送。
          </p>
        )}
        <label className="xc-field">
          <span>{t("Notify again after")}</span>
          <select
            className="xc-select"
            value={device.cooldownMinutes ?? ""}
            disabled={update.isPending}
            onChange={(e) =>
              save({
                cooldownMinutes:
                  e.target.value === "" ? null : Number(e.target.value),
              })
            }
          >
            <option value="">
              {t("Use the default")}（{cooldownLabel(fallback)}）
            </option>
            {COOLDOWNS.map((n) => (
              <option key={n} value={n}>
                {cooldownLabel(n)}
              </option>
            ))}
          </select>
          <small>
            推送过一次后，这段时间里再变化只记日志。时间到了状态和上次推送的不一样，会补推一条。
          </small>
        </label>
        <dl className="router-drawer-info">
          <dt>MAC</dt>
          <dd className="router-mono">{device.mac}</dd>
          <dt>IP</dt>
          <dd className="router-mono">{device.ip ?? "—"}</dd>
          {device.ipv6 && device.ipv6.length > 0 && (
            <>
              <dt>IPv6</dt>
              <dd className="router-mono">
                {device.ipv6.map((a) => (
                  <div key={a}>{a}</div>
                ))}
              </dd>
            </>
          )}
          <dt>{t("Hostname")}</dt>
          <dd>{device.name ?? "—"}</dd>
          <dt>{t("Connection")}</dt>
          <dd>{conn}</dd>
          <dt>{t("Lease expires")}</dt>
          <dd>{when(device.leaseExpiresAt)}</dd>
          <dt>{t("First seen")}</dt>
          <dd>{when(device.firstSeen)}</dd>
          <dt>{t("Last seen")}</dt>
          <dd>{when(device.lastSeen)}</dd>
        </dl>
        <div className="router-drawer-foot">
          <button
            type="button"
            className="xc-btn ghost danger"
            disabled={forget.isPending}
            onClick={onForget}
          >
            {t("Forget device")}
          </button>
        </div>
      </div>
    </aside>
  );
}
