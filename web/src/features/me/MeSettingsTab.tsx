import { useState } from "react";
import { Link } from "react-router";
import { Save } from "lucide-react";
import { ApiError, errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { copyText } from "../../lib/errors";
import { useHAStatus } from "../home/api";
import {
  useCreateMeReportToken,
  useDeleteMeReport,
  useMeReport,
  useMeSettings,
  useSaveMeSettings,
  type MeReportToken,
  type MeSettings,
  type MeSettingsInput,
} from "./api";
import { validHomeEntity, validSleepEntity } from "./logic";
import NicknameCard from "../settings/NicknameCard";
import "./i18n";
import "./me.css";

/** 设置里的“我的状态”标签（B162）：工作电脑、推测睡觉、手机上报、Home Assistant。 */
export default function MeSettingsTab() {
  const t = useT();
  const settings = useMeSettings();
  if (settings.isPending) return <Loading />;
  if (settings.isError && isNotLive(settings.error))
    return <NotLive name={t("My status")} />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  return <Form data={settings.data} />;
}

const HOST_BADGE: Record<string, string> = {
  active: "ok",
  idle: "info",
  locked: "info",
  unknown: "warn",
  offline: "",
};
const HOST_LABEL: Record<string, string> = {
  active: "In use",
  idle: "Idle",
  locked: "Screen locked",
  unknown: "Not sure",
  offline: "Offline",
};

function Form({ data }: { data: MeSettings }) {
  const t = useT();
  const save = useSaveMeSettings();

  const change = (patch: Partial<MeSettingsInput>) =>
    save.mutate(
      {
        workHosts: data.workHosts,
        guessSleep: data.guessSleep,
        haHome: data.haHome,
        haSleep: data.haSleep,
        ...patch,
      },
      {
        onSuccess: () => toast(t("Saved")),
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );

  const toggleHost = (id: string, on: boolean) =>
    change({
      workHosts: on
        ? [...data.workHosts, id]
        : data.workHosts.filter((h) => h !== id),
    });

  return (
    <div className="settings-grid me-settings">
      <NicknameCard />

      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Work computers")}</h3>
        </div>
        <p className="settings-note">
          {t(
            "Working means one of these is in use. If you pick none, every computer that reports idle time counts.",
          )}
        </p>
        {data.hosts.length === 0 ? (
          <div className="settings-line">
            <small>
              {t(
                "No computer reports idle time yet. Pair a Windows agent in Devices first.",
              )}
            </small>
            <Link className="xc-btn small" to="/settings/devices">
              {t("Devices & agents")}
            </Link>
          </div>
        ) : (
          <ul className="me-hosts">
            {data.hosts.map((h) => (
              <li key={h.id}>
                <label className="xc-check">
                  <input
                    type="checkbox"
                    checked={data.workHosts.includes(h.id)}
                    disabled={save.isPending}
                    onChange={(e) => toggleHost(h.id, e.target.checked)}
                  />
                  <span>{h.name}</span>
                </label>
                <span
                  className={`xc-badge ${HOST_BADGE[h.state ?? "offline"]}`}
                >
                  {t(HOST_LABEL[h.state ?? "offline"] ?? "Offline")}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="xc-card">
        <div className="settings-line">
          <div>
            <strong>{t("Guess that I am asleep")}</strong>
            <small>
              {t(
                "From 1 hour before your bedtime to your wake-up time, when no work computer was used for 30 minutes and you are not out. It is marked Guessed.",
              )}{" "}
              <Link to="/habits?schedule=1">
                {t("Edit the daily schedule")}
              </Link>
            </small>
          </div>
          <Switch
            checked={data.guessSleep}
            label={t("Guess that I am asleep")}
            disabled={save.isPending}
            onChange={(guessSleep) => change({ guessSleep })}
          />
        </div>
      </section>

      <PhoneReport />
      <HomeAssistant data={data} saving={save.isPending} onSave={change} />
    </div>
  );
}

function PhoneReport() {
  const t = useT();
  const language = useLanguage();
  const status = useMeReport();
  const create = useCreateMeReportToken();
  const remove = useDeleteMeReport();
  const [issued, setIssued] = useState<MeReportToken | null>(null);
  const [copied, setCopied] = useState(false);

  if (status.isPending) return <Loading />;
  if (status.isError)
    return (
      <section className="xc-card">
        <ErrorState error={status.error} onRetry={() => status.refetch()} />
      </section>
    );
  const s = status.data;
  const when = (iso: string) =>
    new Date(iso).toLocaleString(language === "zh" ? "zh-CN" : undefined);

  const onCreate = async () => {
    if (s.enabled) {
      const ok = await confirmAction({
        title: t("Create a new token?"),
        description: t(
          "The old token stops working at once. Update your shortcuts with the new one.",
        ),
        confirmLabel: t("Create a new token"),
        danger: false,
      });
      if (!ok) return;
    }
    create.mutate(undefined, {
      onSuccess: (data) => {
        setIssued(data);
        setCopied(false);
        toast(t("Report token created"));
      },
      onError: (e) => {
        if (e instanceof ApiError && e.code === "elevation_canceled") return;
        toast({ message: errorMessage(e), tone: "error" });
      },
    });
  };
  const onDelete = async () => {
    const ok = await confirmAction({
      title: t("Turn off phone report?"),
      description: t(
        "The token stops working. Your shortcuts will get an error until you make a new token.",
      ),
      confirmLabel: t("Turn off"),
    });
    if (!ok) return;
    remove.mutate(undefined, {
      onSuccess: () => {
        setIssued(null);
        toast(t("Phone report turned off"));
      },
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  };

  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h3>{t("Phone report")}</h3>
        <span className={`xc-badge ${s.enabled ? "ok" : ""}`}>
          {s.enabled ? t("On") : t("Off")}
        </span>
      </div>
      <p className="settings-note">
        {t(
          "An iPhone shortcut (or any script) posts one event with a token. It sends no location, only the event name.",
        )}
      </p>
      <p className="me-report-state">
        {!s.enabled
          ? t("Not turned on")
          : s.lastReportAt
            ? `${t("Last report")}: ${when(s.lastReportAt)}${s.lastEvent ? ` (${s.lastEvent})` : ""}`
            : t("No report received yet")}
      </p>
      <p className="xc-muted me-report-url">
        {t("Address")}: <code className="me-url">{s.reportUrl}</code>
      </p>
      <table className="me-events">
        <tbody>
          {EVENTS.map(([event, text]) => (
            <tr key={event}>
              <td>
                <code>{event}</code>
              </td>
              <td className="xc-muted">{t(text)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="me-actions">
        {s.enabled && (
          <button
            type="button"
            className="xc-btn"
            disabled={remove.isPending}
            onClick={onDelete}
          >
            {t("Turn off")}
          </button>
        )}
        <button
          type="button"
          className="xc-btn primary"
          disabled={create.isPending}
          onClick={onCreate}
        >
          {s.enabled ? t("Create a new token") : t("Create report token")}
        </button>
      </div>
      {issued && (
        <>
          <p>
            {t(
              "The token is shown only once. Copy it now, or copy the command.",
            )}
          </p>
          <p className="me-token">
            <code>{issued.token}</code>
          </p>
          <div className="me-code">
            <button
              type="button"
              className="xc-btn small"
              onClick={async () => {
                if (await copyText(issued.example)) {
                  setCopied(true);
                  window.setTimeout(() => setCopied(false), 2000);
                }
              }}
            >
              {copied ? t("Copied") : t("Copy")}
            </button>
            <pre>{issued.example}</pre>
          </div>
        </>
      )}
      <ShortcutSteps />
      <SignalHelp />
    </section>
  );
}

const EVENTS: [string, string][] = [
  ["sleep.start", "Sleep focus mode on, or bedtime starts"],
  ["sleep.end", "Sleep focus mode off, wake up, or alarm stopped"],
  ["home.arrive", "Arrive home, or join the home Wi-Fi"],
  ["home.leave", "Leave home, or leave the home Wi-Fi"],
];

function ShortcutSteps() {
  const t = useT();
  return (
    <details className="settings-help">
      <summary>{t("iPhone shortcut steps")}</summary>
      <ol className="me-steps">
        <li>
          {t(
            "Open the Shortcuts app, tap Automation, then the + at the top right.",
          )}
        </li>
        <li>
          {t(
            "Pick a trigger. For example Sleep > Bedtime Starts, or Focus > Sleep > When Turning On.",
          )}
        </li>
        <li>{t("Choose Run Immediately and turn off Notify When Run.")}</li>
        <li>
          {t(
            "Add the action Get Contents of URL. Put the address above in the URL, set the method to POST and add the header Authorization with the value Bearer plus your token. Set the request body to JSON and add one field event with the value sleep.start.",
          )}
        </li>
        <li>
          {t(
            "Make more in the same way: sleep.end when the Sleep focus mode turns off or the alarm stops, home.arrive when you arrive home, home.leave when you leave home.",
          )}
        </li>
      </ol>
    </details>
  );
}

/** 手机和 Home Assistant 两种来源怎么配合，平时不用看，收起来。 */
function SignalHelp() {
  const t = useT();
  return (
    <details className="settings-help">
      <summary>{t("Which signal to use")}</summary>
      <p className="settings-note">
        {t(
          "Your computers tell when you are working. Your phone tells the rest. Two ways work together.",
        )}
      </p>
      <ul className="me-split">
        <li>
          {t(
            "Home and out: use Home Assistant (person or device_tracker). The Home Assistant app finds your place in the background more reliably.",
          )}
        </li>
        <li>
          {t(
            "Sleep: use an iPhone shortcut with the Sleep focus mode or the bedtime. Home Assistant cannot tell whether a focus mode is the Sleep one.",
          )}
        </li>
        <li>
          {t(
            "If both report the same thing within 2 minutes, it counts once. If they disagree, the last one received wins.",
          )}
        </li>
      </ul>
    </details>
  );
}

function HomeAssistant({
  data,
  saving,
  onSave,
}: {
  data: MeSettings;
  saving: boolean;
  onSave: (patch: Partial<MeSettingsInput>) => void;
}) {
  const t = useT();
  const ha = useHAStatus();
  const [home, setHome] = useState(data.haHome);
  const [sleep, setSleep] = useState(data.haSleep);
  const connected = ha.data?.connected === true;
  const dirty = home.trim() !== data.haHome || sleep.trim() !== data.haSleep;
  const valid = validHomeEntity(home) && validSleepEntity(sleep);

  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h3>{t("Home Assistant (optional)")}</h3>
      </div>
      {ha.isPending ? (
        <Loading />
      ) : !connected ? (
        <div className="settings-line">
          <small>{t("Connect Home Assistant in Settings first.")}</small>
          <Link className="xc-btn small" to="/settings/homeassistant">
            {t("Home Assistant")}
          </Link>
        </div>
      ) : (
        <>
          <p className="settings-note">
            {t("Leave a field empty to skip it.")}
          </p>
          <div className="settings-form">
            <label className="xc-field">
              <span>{t("At-home entity")}</span>
              <input
                className="xc-input"
                value={home}
                placeholder="person.me"
                aria-invalid={!validHomeEntity(home)}
                onChange={(e) => setHome(e.target.value)}
              />
              <small className="xc-muted">
                {t(
                  "person.xxx or device_tracker.xxx. The state home means arrived. Any other state means left.",
                )}
              </small>
            </label>
            <label className="xc-field">
              <span>{t("Sleep focus entity")}</span>
              <input
                className="xc-input"
                value={sleep}
                placeholder="binary_sensor.iphone_focus"
                aria-invalid={!validSleepEntity(sleep)}
                onChange={(e) => setSleep(e.target.value)}
              />
              <small className="xc-muted">
                {t(
                  "The focus sensor of the Home Assistant app. On means asleep. Off means awake.",
                )}
              </small>
              <small className="me-warn">
                {t(
                  "This sensor is on for every focus mode, not only Sleep. If you also use other focus modes in the daytime, leave it empty and use a shortcut instead.",
                )}
              </small>
            </label>
          </div>
          <div className="me-actions">
            <button
              type="button"
              className="xc-btn primary"
              disabled={!dirty || !valid || saving}
              onClick={() =>
                onSave({ haHome: home.trim(), haSleep: sleep.trim() })
              }
            >
              <Save size={14} /> {t("Save")}
            </button>
          </div>
        </>
      )}
    </section>
  );
}
