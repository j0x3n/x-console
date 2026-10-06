import { Fragment, useEffect, useState } from "react";
import { BellRing } from "lucide-react";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import {
  useNotifyChannels,
  usePushSubscriptions,
  type ChannelName,
} from "./api";
import { deviceTarget, mutedFromChecked } from "./mutes";
import { currentEndpoint, deviceName } from "./push";
import "./i18n";

const channelNames: Record<Exclude<ChannelName, "webpush">, string> = {
  telegram: "Telegram",
  bark: "Bark",
  serverchan: "ServerChan",
};

export interface TargetOption {
  /** 渠道名，或 webpush:<订阅编号> */
  id: string;
  label: string;
  hint?: string;
  /** 就是正在用的这台设备 */
  mine?: boolean;
}

/** 能发送的目标：每台 Web Push 设备，加上配置好的其他渠道。通知铃不在里面，它一直发。 */
export function useNotifyTargetOptions() {
  const t = useT();
  const language = useLanguage();
  const channels = useNotifyChannels();
  const subs = usePushSubscriptions();
  const [mine, setMine] = useState<string | null>(null);
  useEffect(() => {
    void currentEndpoint().then(setMine);
  }, []);
  const options: TargetOption[] = [];
  for (const s of subs.data ?? []) {
    options.push({
      id: deviceTarget(s.id),
      label: deviceName(s.userAgent ?? "") || t("Browser"),
      hint: s.lastOkAt
        ? `${t("Last delivered")} ${relativeTime(s.lastOkAt, language)}`
        : (s.lastError ?? undefined),
      mine: mine !== null && s.endpoint === mine,
    });
  }
  for (const c of channels.data ?? []) {
    if (c.name === "webpush" || !c.configured) continue;
    options.push({
      id: c.name,
      label: channelNames[c.name as Exclude<ChannelName, "webpush">] ?? c.name,
    });
  }
  return {
    options,
    loading: channels.isPending || subs.isPending,
    error: channels.isError && subs.isError,
  };
}

/**
 * “通知发到”（B113）：勾上的收，没勾的就是静音。通知铃一直发，不能取消。
 * muted 是被静音的目标，改动通过 onChange 交给外面保存。
 */
export default function NotifyTargetsField({
  muted,
  onChange,
  disabled,
}: {
  muted: string[];
  onChange: (muted: string[]) => void;
  disabled?: boolean;
}) {
  const t = useT();
  const { options, loading } = useNotifyTargetOptions();
  const checked = new Set(
    options.filter((o) => !muted.includes(o.id)).map((o) => o.id),
  );
  const toggle = (id: string, on: boolean) => {
    const next = new Set(checked);
    if (on) next.add(id);
    else next.delete(id);
    // 目标列表里没有的（已删除的设备）静音规则保持不动
    const known = options.map((o) => o.id);
    const stale = muted.filter((m) => !known.includes(m));
    onChange([...stale, ...mutedFromChecked(known, next)]);
  };
  return (
    <fieldset className="notify-targets" disabled={disabled}>
      <legend>{t("Send notifications to")}</legend>
      <label className="xc-check">
        <input type="checkbox" checked disabled readOnly />
        <span>
          <BellRing size={13} /> {t("Notification bell")}
        </span>
      </label>
      <small className="xc-check-hint">{t("Always on")}</small>
      {loading ? (
        <small className="xc-muted">{t("Loading…")}</small>
      ) : (
        options.map((o) => (
          <Fragment key={o.id}>
            <label className="xc-check">
              <input
                type="checkbox"
                checked={checked.has(o.id)}
                onChange={(e) => toggle(o.id, e.target.checked)}
              />
              <span>
                {o.label}
                {o.mine && (
                  <span className="xc-badge accent notify-mine">
                    {t("This device")}
                  </span>
                )}
              </span>
            </label>
            {o.hint && <small className="xc-check-hint">{o.hint}</small>}
          </Fragment>
        ))
      )}
    </fieldset>
  );
}
