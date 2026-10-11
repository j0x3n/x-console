import { Download, Save } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { ApiError, errorMessage, isNotLive } from "../../api/client";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  fetchSuggestions,
  useFocusBlockSettings,
  usePutFocusBlockSettings,
  type FocusBlockComputer,
  type FocusBlockInput,
  type FocusBlockSettings,
} from "./api";
import { addApps, joinLines, parseApps, parseDomains, sameList } from "./lists";
import "./i18n";
import "./focusblock.css";

/** 设置里的“专注屏蔽”标签（B131）：总开关、电脑、名单、休息时是否屏蔽。 */
export default function FocusBlockSettingsTab() {
  const t = useT();
  const settings = useFocusBlockSettings();
  if (settings.isPending) return <Loading />;
  if (settings.isError && isNotLive(settings.error))
    return <NotLive name={t("Focus block")} />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  const d = settings.data;
  // 服务端的名单变了（保存后、别处改了），输入框跟着换成新的。
  const signature = JSON.stringify([d.domains, d.apps, d.haStart, d.haEnd]);
  return <Form key={signature} data={d} />;
}

const stateBadge: Record<FocusBlockComputer["state"], string> = {
  active: "ok",
  permission: "warn",
  failed: "danger",
  offline: "",
  idle: "info",
  unsupported: "warn",
  off: "",
};

function Form({ data }: { data: FocusBlockSettings }) {
  const t = useT();
  const save = usePutFocusBlockSettings();
  const [domains, setDomains] = useState(joinLines(data.domains));
  const [apps, setApps] = useState(joinLines(data.apps));
  const [haStart, setHaStart] = useState(data.haStart);
  const [haEnd, setHaEnd] = useState(data.haEnd);
  const [importing, setImporting] = useState(false);

  const change = (patch: FocusBlockInput, after?: () => void) =>
    save.mutate(patch, {
      onSuccess: () => {
        toast(t("Saved"));
        after?.();
      },
      onError: (e) => {
        if (e instanceof ApiError && e.code === "elevation_canceled") return;
        toast({ message: errorMessage(e), tone: "error" });
      },
    });

  const listsDirty =
    !sameList(parseDomains(domains), data.domains) ||
    !sameList(parseApps(apps), data.apps);
  const haDirty =
    haStart.trim() !== data.haStart || haEnd.trim() !== data.haEnd;

  const toggleHost = (id: string, on: boolean) =>
    change({
      hostIds: on
        ? [...data.hostIds, id]
        : data.hostIds.filter((h) => h !== id),
    });

  const importApps = async () => {
    setImporting(true);
    try {
      const found = await fetchSuggestions();
      const next = addApps(apps, found);
      if (next.added === 0) {
        toast(t("No new entertainment program found"));
        return;
      }
      setApps(next.text);
      toast(
        `${next.added} ${t("programs added. Press Save lists to use them.")}`,
      );
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    } finally {
      setImporting(false);
    }
  };

  return (
    <div className="settings-grid focusblock">
      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Focus block")}</h3>
        </div>
        <p className="xc-muted">
          {t(
            "Block sites and programs on your computers while a pomodoro runs.",
          )}
        </p>
        <div className="focusblock-option">
          <span>{t("Turn on focus block")}</span>
          <Switch
            checked={data.enabled}
            label={t("Turn on focus block")}
            disabled={save.isPending}
            onChange={(enabled) => change({ enabled })}
          />
        </div>
        <div className="focusblock-option">
          <span>
            {t("Keep blocking in the break")}
            <small className="xc-muted">
              {t(
                "After the time is up and until you press Finish. Off by default, so the break is free.",
              )}
            </small>
          </span>
          <Switch
            checked={data.blockInBreak}
            label={t("Keep blocking in the break")}
            disabled={save.isPending}
            onChange={(blockInBreak) => change({ blockInBreak })}
          />
        </div>
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Computers to block")}</h3>
        </div>
        {data.computers.length === 0 ? (
          <p className="xc-muted">
            {t("No paired computer yet. Pair an agent in Devices first.")}{" "}
            <Link to="/settings/devices">{t("Devices & agents")}</Link>
          </p>
        ) : (
          <ul className="focusblock-computers">
            {data.computers.map((c) => (
              <li key={c.id}>
                <label className="xc-check">
                  <input
                    type="checkbox"
                    checked={c.selected}
                    disabled={!c.supported || save.isPending}
                    onChange={(e) => toggleHost(c.id, e.target.checked)}
                  />
                  <span>{c.name}</span>
                  <small className="xc-muted">{c.os}</small>
                  <span className={`xc-badge ${stateBadge[c.state]}`}>
                    {stateLabel(c.state, t)}
                  </span>
                </label>
                {c.message && (
                  <small className="focusblock-message xc-muted">
                    {c.message}
                  </small>
                )}
                {c.refused.length > 0 && (
                  <small className="focusblock-message focusblock-refused">
                    {c.refused.join("；")}
                  </small>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Sites to block")}</h3>
        </div>
        <div className="xc-field">
          <textarea
            className="xc-textarea focusblock-list"
            rows={7}
            value={domains}
            aria-label={t("Sites to block")}
            placeholder="youtube.com&#10;bilibili.com"
            onChange={(e) => setDomains(e.target.value)}
          />
          <small className="xc-muted">
            {t("One site per line. You can paste a whole address.")}
          </small>
        </div>
        <label className="xc-check">
          <input
            type="checkbox"
            checked={data.ipv6}
            disabled={save.isPending}
            onChange={(e) => change({ ipv6: e.target.checked })}
          />
          <span>{t("Also block IPv6")}</span>
        </label>
        <h3 className="focusblock-subhead">{t("Programs to end")}</h3>
        <div className="xc-field">
          <textarea
            className="xc-textarea focusblock-list"
            rows={7}
            value={apps}
            aria-label={t("Programs to end")}
            placeholder="steam&#10;cloudmusic"
            onChange={(e) => setApps(e.target.value)}
          />
          <small className="xc-muted">
            {t(
              "One program name per line, like steam. Capital letters and .exe do not matter. Only the name is compared, never the path.",
            )}
          </small>
        </div>
        <div className="focusblock-actions">
          <button
            type="button"
            className="xc-btn"
            disabled={importing}
            title={t("Programs that Computer time sorted as entertainment")}
            onClick={importApps}
          >
            <Download size={14} /> {t("Import entertainment programs")}
          </button>
          <button
            type="button"
            className="xc-btn primary"
            disabled={!listsDirty || save.isPending}
            onClick={() =>
              change({ domains: parseDomains(domains), apps: parseApps(apps) })
            }
          >
            <Save size={14} /> {t("Save lists")}
          </button>
        </div>
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Home Assistant (optional)")}</h3>
        </div>
        <p className="xc-muted">
          {t(
            "Turn on a scene or script when focus starts, and another when the block ends. Leave empty to skip.",
          )}
        </p>
        <label className="xc-field">
          <span>{t("Scene or script at the start")}</span>
          <input
            className="xc-input"
            value={haStart}
            placeholder="scene.focus"
            onChange={(e) => setHaStart(e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Scene or script at the end")}</span>
          <input
            className="xc-input"
            value={haEnd}
            placeholder="script.focus_off"
            onChange={(e) => setHaEnd(e.target.value)}
          />
        </label>
        {data.haMessage && <p className="xc-muted">{data.haMessage}</p>}
        <div className="focusblock-actions">
          <button
            type="button"
            className="xc-btn primary"
            disabled={!haDirty || save.isPending}
            onClick={() =>
              change({ haStart: haStart.trim(), haEnd: haEnd.trim() })
            }
          >
            <Save size={14} /> {t("Save Home Assistant")}
          </button>
        </div>
      </section>

      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Good to know")}</h3>
        </div>
        <ul className="focusblock-notes">
          <li>
            {t(
              "Secure DNS (DNS over HTTPS) in the browser can skip the hosts file. Turn it off in the browser settings.",
            )}
          </li>
          <li>
            {t("Only computers are covered. Phones and tablets are not.")}
          </li>
          <li>
            {t(
              "Changing the hosts file needs administrator rights on Windows and root on Linux. Without them, only programs are ended.",
            )}
          </li>
          <li>{t("Pages that are already open may need a reload.")}</li>
          <li>
            {t(
              "If the panel or the agent stops, the agent restores everything by itself 5 minutes after the planned end.",
            )}
          </li>
        </ul>
      </section>
    </div>
  );
}

function stateLabel(
  state: FocusBlockComputer["state"],
  t: (key: string) => string,
): string {
  switch (state) {
    case "active":
      return t("Block is on");
    case "permission":
      return t("No permission");
    case "failed":
      return t("Block failed");
    case "offline":
      return t("Offline");
    case "idle":
      return t("Waiting for focus");
    case "unsupported":
      return t("Update the agent");
    default:
      return "";
  }
}
