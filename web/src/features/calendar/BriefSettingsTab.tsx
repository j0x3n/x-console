import { useEffect, useState } from "react";
import { Link } from "react-router";
import { LocateFixed, Newspaper } from "lucide-react";
import { errorMessage } from "../../api/client";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useBriefSettings,
  useSaveBriefSettings,
  type BriefSectionKey,
  type BriefSettings,
  type BriefSettingsView,
} from "./api";
import "./i18n";
import "./calendar.css";

export const sectionLabels: Record<
  Exclude<BriefSectionKey, "summary">,
  string
> = {
  weather: "Weather",
  calendar: "Today's calendar",
  issues: "Due issues",
  reminders: "Today's reminders",
  alerts: "Server alerts since yesterday",
  habits: "Habits",
  renewals: "Renewals",
};
const sectionKeys = Object.keys(sectionLabels) as Exclude<
  BriefSectionKey,
  "summary"
>[];

const channelLabels: Record<string, string> = {
  webpush: "Browser push",
  telegram: "Telegram",
  bark: "Bark",
  serverchan: "ServerChan",
};

interface Form {
  enabled: boolean;
  time: string;
  channels: string[];
  sections: BriefSectionKey[];
  lat: string;
  lon: string;
  place: string;
  aiPolish: boolean;
  weatherApiBase: string;
}

function toForm(v: BriefSettingsView): Form {
  return {
    enabled: v.enabled,
    time: v.time,
    channels: v.channels,
    sections: v.sections,
    lat: v.location ? String(v.location.lat) : "",
    lon: v.location ? String(v.location.lon) : "",
    place: v.location?.name ?? "",
    aiPolish: v.aiPolish ?? false,
    weatherApiBase: v.weatherApiBase ?? "",
  };
}

/** 设置页的“早报”标签。 */
export default function BriefSettingsTab() {
  const settings = useBriefSettings();
  if (settings.isPending) return <Loading />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  return <BriefForm view={settings.data} />;
}

function BriefForm({ view }: { view: BriefSettingsView }) {
  const t = useT();
  const save = useSaveBriefSettings();
  const [form, setForm] = useState<Form>(() => toForm(view));
  const [error, setError] = useState("");
  const [locating, setLocating] = useState(false);
  useEffect(() => setForm(toForm(view)), [view]);

  const set = <K extends keyof Form>(key: K, value: Form[K]) =>
    setForm((f) => ({ ...f, [key]: value }));
  const toggle = <T extends string>(list: T[], item: T) =>
    list.includes(item) ? list.filter((x) => x !== item) : [...list, item];

  const locate = () => {
    if (!navigator.geolocation) {
      toast({
        message: t("This browser cannot share its location."),
        tone: "error",
      });
      return;
    }
    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        setLocating(false);
        setForm((f) => ({
          ...f,
          lat: pos.coords.latitude.toFixed(4),
          lon: pos.coords.longitude.toFixed(4),
        }));
      },
      () => {
        setLocating(false);
        toast({
          message: t("Could not get the location. Enter it by hand."),
          tone: "error",
        });
      },
      { timeout: 10_000 },
    );
  };

  const submit = () => {
    setError("");
    const hasLat = form.lat.trim() !== "";
    const hasLon = form.lon.trim() !== "";
    if (hasLat !== hasLon)
      return setError(t("Enter both latitude and longitude."));
    const lat = Number(form.lat);
    const lon = Number(form.lon);
    if (hasLat && (!Number.isFinite(lat) || !Number.isFinite(lon))) {
      return setError(t("Latitude and longitude must be numbers."));
    }
    const body: BriefSettings = {
      enabled: form.enabled,
      time: form.time,
      channels: form.channels,
      sections: form.sections,
      location: hasLat ? { lat, lon, name: form.place.trim() } : undefined,
      weatherApiBase: form.weatherApiBase.trim() || undefined,
    };
    // AI 润色：M12 提供 Polisher 后 aiAvailable 为 true，这里才显示和提交。
    if (view.aiAvailable) body.aiPolish = form.aiPolish;
    save.mutate(body, {
      onSuccess: () => toast(t("Saved")),
      onError: (err) => setError(errorMessage(err)),
    });
  };

  return (
    <form
      className="xc-stack brief-settings"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <section className="xc-card brief-settings-card">
        <h3>{t("Schedule")}</h3>
        <label className="brief-check">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={(e) => set("enabled", e.target.checked)}
          />
          <span>{t("Send the brief every day")}</span>
        </label>
        <label className="xc-field brief-time">
          <span>{t("Time")}</span>
          <input
            className="xc-input"
            type="time"
            value={form.time}
            onChange={(e) => set("time", e.target.value)}
            required
          />
        </label>
        <div className="xc-field">
          <span>{t("Channels")}</span>
          <div className="brief-options">
            {view.availableChannels.map((name) => (
              <label key={name} className="brief-check">
                <input
                  type="checkbox"
                  checked={form.channels.includes(name)}
                  onChange={() => set("channels", toggle(form.channels, name))}
                />
                <span>{t(channelLabels[name] ?? name)}</span>
              </label>
            ))}
          </div>
          <small>
            {t(
              "With none picked, the routing rules in notification settings decide.",
            )}{" "}
            <Link to="/settings/notifications">
              {t("Notification settings")}
            </Link>
          </small>
        </div>
      </section>

      <section className="xc-card brief-settings-card">
        <h3>{t("Contents")}</h3>
        <div className="brief-options">
          {sectionKeys.map((key) => (
            <label key={key} className="brief-check">
              <input
                type="checkbox"
                checked={form.sections.includes(key)}
                onChange={() => set("sections", toggle(form.sections, key))}
              />
              <span>{t(sectionLabels[key])}</span>
            </label>
          ))}
        </div>
        <small className="xc-muted">
          {t("A part is left out when its module is not set up.")}
        </small>
        {view.aiAvailable && (
          <label className="brief-check">
            <input
              type="checkbox"
              checked={form.aiPolish}
              onChange={(e) => set("aiPolish", e.target.checked)}
            />
            <span>{t("Start with a short AI summary")}</span>
          </label>
        )}
      </section>

      <section className="xc-card brief-settings-card">
        <h3>{t("Weather location")}</h3>
        <div className="brief-location">
          <label className="xc-field">
            <span>{t("Latitude")}</span>
            <input
              className="xc-input"
              inputMode="decimal"
              value={form.lat}
              onChange={(e) => set("lat", e.target.value)}
              placeholder="31.23"
            />
          </label>
          <label className="xc-field">
            <span>{t("Longitude")}</span>
            <input
              className="xc-input"
              inputMode="decimal"
              value={form.lon}
              onChange={(e) => set("lon", e.target.value)}
              placeholder="121.47"
            />
          </label>
          <label className="xc-field">
            <span>{t("Place name")}</span>
            <input
              className="xc-input"
              value={form.place}
              onChange={(e) => set("place", e.target.value)}
              placeholder={t("Shanghai")}
            />
          </label>
        </div>
        <div className="xc-row">
          <button
            type="button"
            className="xc-btn small"
            disabled={locating}
            onClick={locate}
          >
            <LocateFixed size={14} /> {t("Use my location")}
          </button>
          <small className="xc-muted">
            {t("Leave empty to skip the weather.")}
          </small>
        </div>
        <details className="brief-advanced">
          <summary>{t("Advanced")}</summary>
          <label className="xc-field">
            <span>{t("Open-Meteo address")}</span>
            <input
              className="xc-input"
              value={form.weatherApiBase}
              onChange={(e) => set("weatherApiBase", e.target.value)}
              placeholder="https://api.open-meteo.com"
            />
          </label>
        </details>
      </section>

      {error && <p className="xc-error-text">{error}</p>}
      <div className="xc-row">
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
        <Link className="xc-btn ghost" to="/calendar/briefs">
          <Newspaper size={14} /> {t("Brief history")}
        </Link>
      </div>
    </form>
  );
}
