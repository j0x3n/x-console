import QWeatherCard from "./QWeatherCard";
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
  resolveWeatherLocation,
  type BriefSectionKey,
  type BriefSettings,
  type BriefSettingsView,
} from "./api";
import { useMeReport, useMeSettings } from "../me/api";
import "./i18n";
import "./calendar.css";

export const sectionLabels: Record<
  Exclude<BriefSectionKey, "summary">,
  string
> = {
  weather: "Weather",
  calendar: "Today's calendar",
  plan: "Brief plan",
  issues: "Due issues",
  reminders: "Today's reminders",
  alerts: "Server alerts since yesterday",
  habits: "Habits",
  renewals: "Renewals",
  cards: "Cards to review",
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
  sendMode: "time" | "wake";
  channels: string[];
  sections: BriefSectionKey[];
  lat: string;
  lon: string;
  place: string;
  id?: string;
  aiPolish: boolean;
}

function toForm(v: BriefSettingsView): Form {
  return {
    enabled: v.enabled,
    time: v.time,
    sendMode: v.sendMode ?? "time",
    channels: v.channels,
    sections: v.sections,
    lat: v.location ? String(v.location.lat) : "",
    lon: v.location ? String(v.location.lon) : "",
    place: v.location?.name ?? "",
    id: v.location?.id,
    aiPolish: v.aiPolish ?? false,
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
  return (
    <div className="settings-grid">
      <BriefForm view={settings.data} />
      <QWeatherCard />
    </div>
  );
}

function BriefForm({ view }: { view: BriefSettingsView }) {
  const t = useT();
  const save = useSaveBriefSettings();
  const [form, setForm] = useState<Form>(() => toForm(view));
  const [error, setError] = useState("");
  const [locating, setLocating] = useState(false);
  useEffect(() => setForm(toForm(view)), [view]);

  const set = <K extends keyof Form>(key: K, value: Form[K]) =>
    setForm((f) => ({
      ...f,
      [key]: value,
      id: key === "lat" || key === "lon" ? undefined : f.id,
    }));
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
      async (pos) => {
        try {
          const city = await resolveWeatherLocation(
            pos.coords.latitude,
            pos.coords.longitude,
          );
          setForm((f) => ({
            ...f,
            id: city.id,
            lat: String(city.lat),
            lon: String(city.lon),
            place: city.name ?? "",
          }));
        } catch (err) {
          setError(errorMessage(err));
        } finally {
          setLocating(false);
        }
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
      sendMode: form.sendMode,
      channels: form.channels,
      sections: form.sections,
      location: hasLat
        ? { id: form.id, lat, lon, name: form.place.trim() }
        : undefined,
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
      className="xc-card brief-settings"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <div className="settings-pair">
        <div>
          <strong>{t("Send time")}</strong>
        </div>
        <div className="brief-schedule">
          <label className="brief-check">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => set("enabled", e.target.checked)}
            />
            <span>{t("Send the brief every day")}</span>
          </label>
          <span className="brief-options" role="radiogroup">
            <label className="brief-check">
              <input
                type="radio"
                name="brief-send-mode"
                checked={form.sendMode === "time"}
                onChange={() => set("sendMode", "time")}
              />
              <span>{t("At a fixed time")}</span>
            </label>
            <label className="brief-check">
              <input
                type="radio"
                name="brief-send-mode"
                checked={form.sendMode === "wake"}
                onChange={() => set("sendMode", "wake")}
              />
              <span>{t("After I wake up, at the latest")}</span>
            </label>
          </span>
          <input
            className="xc-input brief-time"
            type="time"
            aria-label={t("Time")}
            value={form.time}
            onChange={(e) => set("time", e.target.value)}
            required
          />
          {form.sendMode === "wake" && <WakeSignalHint />}
        </div>
      </div>
      <div className="settings-pair">
        <div>
          <strong>{t("Channels")}</strong>
          <small>
            {t(
              "With none picked, the routing rules in notification settings decide.",
            )}{" "}
            <Link to="/settings/notifications">
              {t("Notification settings")}
            </Link>
          </small>
        </div>
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
      </div>
      <div className="settings-pair">
        <div>
          <strong>{t("Contents")}</strong>
          <small>
            {t("A part is left out when its module is not set up.")}
          </small>
        </div>
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
        </div>
      </div>
      <div className="settings-pair">
        <div>
          <strong>{t("Weather location")}</strong>
          <small>{t("Leave empty to skip the weather.")}</small>
        </div>
        <div className="brief-location">
          <input
            className="xc-input"
            inputMode="decimal"
            aria-label={t("Latitude")}
            value={form.lat}
            onChange={(e) => set("lat", e.target.value)}
            placeholder="31.23"
          />
          <input
            className="xc-input"
            inputMode="decimal"
            aria-label={t("Longitude")}
            value={form.lon}
            onChange={(e) => set("lon", e.target.value)}
            placeholder="121.47"
          />
          <input
            className="xc-input"
            aria-label={t("Place name")}
            value={form.place}
            onChange={(e) => set("place", e.target.value)}
            placeholder={t("Shanghai")}
          />
          <button
            type="button"
            className="xc-btn"
            disabled={locating}
            onClick={locate}
          >
            <LocateFixed size={14} /> {t("Use my location")}
          </button>
        </div>
      </div>
      {error && <p className="xc-error-text">{error}</p>}
      <div className="settings-actions">
        <Link
          className="xc-btn ghost settings-actions-start"
          to="/calendar/briefs"
        >
          <Newspaper size={14} /> {t("Brief history")}
        </Link>
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
    </form>
  );
}

/** B163：没有起床信号时，起床后发其实只会在最晚时间发。 */
function WakeSignalHint() {
  const t = useT();
  const settings = useMeSettings();
  const report = useMeReport();
  if (settings.isPending || report.isPending) return null;
  const hasSignal =
    !!settings.data?.guessSleep ||
    !!settings.data?.haSleep ||
    !!report.data?.enabled;
  if (hasSignal) return null;
  return (
    <small className="xc-muted brief-wake-hint">
      {t(
        "No wake-up signal yet, so it only goes out at the latest time. Turn on phone reports or sleep guessing in My status.",
      )}
    </small>
  );
}
