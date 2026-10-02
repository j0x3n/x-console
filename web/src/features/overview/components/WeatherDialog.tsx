import { useEffect, useState } from "react";
import { LocateFixed, MapPin, Search } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError, errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import Switch from "../../../components/ui/Switch";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  calendarKeys,
  useBriefSettings,
  useQWeatherConfig,
  resolveWeatherLocation,
  useRainAlert,
  useSaveBriefSettings,
  useSaveRainAlert,
  useSaveWeatherNotify,
  useWeatherNotify,
  useWeatherPlaces,
  type BriefLocation,
  type RainAlert,
  type WarningLevel,
  type WeatherNotify,
} from "../../calendar/api";

/** 天气小条上显示哪些内容，只存在这台设备上。 */
export interface WeatherShow {
  range: boolean;
  rain: boolean;
  place: boolean;
  /** 空气质量（B58，要和风天气） */
  air: boolean;
  /** 湿度（B90） */
  humidity: boolean;
}
const SHOW_KEY = "xc.today.weather";
const defaultShow: WeatherShow = {
  range: true,
  rain: true,
  place: false,
  air: true,
  humidity: true,
};

export function loadWeatherShow(): WeatherShow {
  try {
    const raw = localStorage.getItem(SHOW_KEY);
    if (raw) return { ...defaultShow, ...(JSON.parse(raw) as WeatherShow) };
  } catch {
    // 用默认
  }
  return defaultShow;
}

function saveWeatherShow(v: WeatherShow) {
  try {
    localStorage.setItem(SHOW_KEY, JSON.stringify(v));
  } catch {
    // 存不了就算了
  }
  window.dispatchEvent(new Event("xc:weather-show"));
}

const defaultNotify: WeatherNotify = {
  rainSoon: false,
  rainLeadMinutes: 30,
  warnings: false,
  warningMinLevel: "yellow",
  earthquakes: false,
  quakeMinMagnitude: 4.5,
  quakeRadiusKm: 500,
};

const levelChoices: [WarningLevel, string][] = [
  ["blue", "Blue and above"],
  ["yellow", "Yellow and above"],
  ["orange", "Orange and above"],
  ["red", "Red only"],
];

/** 天气设置：选地区、显示内容、降雨提醒、预警推送（B58）。 */
export default function WeatherDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const qc = useQueryClient();
  const settings = useBriefSettings();
  const saveSettings = useSaveBriefSettings();
  const alert = useRainAlert();
  const saveAlert = useSaveRainAlert();
  const [place, setPlace] = useState<BriefLocation | null>(null);
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [show, setShow] = useState<WeatherShow>(loadWeatherShow);
  const [rain, setRain] = useState<RainAlert>({
    enabled: false,
    threshold: 60,
    leadHours: 2,
  });
  const [locating, setLocating] = useState(false);
  // B58：分钟降水、天气预警、地震的推送
  const notify = useWeatherNotify();
  const saveNotify = useSaveWeatherNotify();
  const qweather = useQWeatherConfig();
  const [push, setPush] = useState<WeatherNotify>(defaultNotify);
  const [error, setError] = useState("");
  const places = useWeatherPlaces(search);
  const alertMissing =
    alert.error instanceof ApiError &&
    (alert.error.status === 404 || alert.error.status === 501);

  useEffect(() => {
    if (!open) return;
    setPlace(settings.data?.location ?? null);
    setShow(loadWeatherShow());
    setQuery("");
    setSearch("");
    setError("");
  }, [open, settings.data]);
  useEffect(() => {
    if (open && alert.data) setRain(alert.data);
  }, [open, alert.data]);
  useEffect(() => {
    if (open && notify.data) setPush(notify.data);
  }, [open, notify.data]);

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
          setPlace(
            await resolveWeatherLocation(
              pos.coords.latitude,
              pos.coords.longitude,
            ),
          );
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

  const pending =
    saveSettings.isPending || saveAlert.isPending || saveNotify.isPending;
  const submit = async () => {
    setError("");
    try {
      const s = settings.data;
      if (s && JSON.stringify(place) !== JSON.stringify(s.location ?? null)) {
        await saveSettings.mutateAsync({
          enabled: s.enabled,
          time: s.time,
          channels: s.channels,
          sections: s.sections,
          location: place ?? undefined,
          aiPolish: s.aiAvailable ? s.aiPolish : undefined,
          weatherApiBase: s.weatherApiBase || undefined,
        });
        qc.invalidateQueries({ queryKey: calendarKeys.weather });
      }
      if (!alertMissing && alert.data) await saveAlert.mutateAsync(rain);
      if (notify.data) await saveNotify.mutateAsync(push);
      saveWeatherShow(show);
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog open={open} onClose={onClose} title={t("Weather settings")}>
      <div className="weather-dialog">
        <section>
          <h3>{t("Place")}</h3>
          {qweather.data && !qweather.data.keySet && (
            <p className="weather-note">请先在设置 → 早报中配置和风天气。</p>
          )}
          <div className="weather-current">
            <MapPin size={14} />
            <span>
              {place
                ? place.name || `${place.lat}, ${place.lon}`
                : t("Not set yet")}
            </span>
            <button
              type="button"
              className="xc-btn small ghost"
              onClick={locate}
              disabled={locating}
            >
              <LocateFixed size={14} /> {t("Use my location")}
            </button>
          </div>
          <form
            className="weather-search"
            onSubmit={(e) => {
              e.preventDefault();
              setSearch(query.trim());
            }}
          >
            <input
              className="xc-input"
              value={query}
              placeholder={t("City name, e.g. Shenzhen")}
              aria-label={t("Search city")}
              onChange={(e) => setQuery(e.target.value)}
            />
            <button className="xc-btn" disabled={!query.trim()}>
              <Search size={14} /> {t("Search")}
            </button>
          </form>
          {search && (
            <div className="weather-places">
              {places.isPending ? (
                <span className="weather-note">{t("Loading")}…</span>
              ) : places.isError ? (
                <span className="weather-note">
                  {errorMessage(places.error)}
                </span>
              ) : places.data.length === 0 ? (
                <span className="weather-note">{t("No matching places")}</span>
              ) : (
                places.data.map((p) => {
                  const on = place?.lat === p.lat && place?.lon === p.lon;
                  return (
                    <button
                      type="button"
                      key={`${p.lat},${p.lon}`}
                      className={on ? "on" : ""}
                      aria-pressed={on}
                      onClick={() =>
                        setPlace({
                          id: p.id,
                          lat: p.lat,
                          lon: p.lon,
                          name: p.name,
                        })
                      }
                    >
                      <b>{p.name}</b>
                      <small>
                        {[p.region, p.country].filter(Boolean).join(" · ")}
                      </small>
                    </button>
                  );
                })
              )}
            </div>
          )}
        </section>

        <section>
          <h3>{t("Show on the today page")}</h3>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={show.range}
              onChange={(e) => setShow({ ...show, range: e.target.checked })}
            />
            <span>{t("Low and high of the day")}</span>
          </label>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={show.humidity}
              onChange={(e) => setShow({ ...show, humidity: e.target.checked })}
            />
            <span>{t("Humidity")}</span>
          </label>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={show.air}
              onChange={(e) => setShow({ ...show, air: e.target.checked })}
            />
            <span>{t("Air quality")}</span>
          </label>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={show.rain}
              onChange={(e) => setShow({ ...show, rain: e.target.checked })}
            />
            <span>{t("Chance of rain")}</span>
          </label>
          <label className="xc-check">
            <input
              type="checkbox"
              checked={show.place}
              onChange={(e) => setShow({ ...show, place: e.target.checked })}
            />
            <span>{t("Place name")}</span>
          </label>
        </section>

        {notify.data && (
          <section>
            <h3>{t("Push alerts")}</h3>
            {!qweather.data?.keySet && (
              <p className="weather-note">
                {t(
                  "Rain by the minute and weather warnings need a QWeather key. Add it in Settings → Daily brief.",
                )}
              </p>
            )}
            <div className="weather-push-row">
              <Switch
                checked={push.rainSoon}
                onChange={(v) => setPush({ ...push, rainSoon: v })}
                label={t("Rain or snow is coming")}
              />
              <span>{t("Rain or snow is coming")}</span>
              <select
                className="xc-select"
                aria-label={t("Lead time")}
                value={push.rainLeadMinutes}
                disabled={!push.rainSoon}
                onChange={(e) =>
                  setPush({ ...push, rainLeadMinutes: Number(e.target.value) })
                }
              >
                {[15, 30, 60, 120].map((m) => (
                  <option key={m} value={m}>
                    {t("Ahead by")} {m} {t("min")}
                  </option>
                ))}
              </select>
            </div>
            <div className="weather-push-row">
              <Switch
                checked={push.warnings}
                onChange={(v) => setPush({ ...push, warnings: v })}
                label={t("Weather warnings")}
              />
              <span>{t("Weather warnings")}</span>
              <select
                className="xc-select"
                aria-label={t("Lowest level")}
                value={push.warningMinLevel}
                disabled={!push.warnings}
                onChange={(e) =>
                  setPush({
                    ...push,
                    warningMinLevel: e.target.value as WarningLevel,
                  })
                }
              >
                {levelChoices.map(([v, label]) => (
                  <option key={v} value={v}>
                    {t(label)}
                  </option>
                ))}
              </select>
            </div>
            <div className="weather-push-row">
              <Switch
                checked={push.earthquakes}
                onChange={(v) => setPush({ ...push, earthquakes: v })}
                label={t("Earthquakes nearby")}
              />
              <span>{t("Earthquakes nearby")}</span>
              <select
                className="xc-select"
                aria-label={t("Magnitude")}
                value={push.quakeMinMagnitude}
                disabled={!push.earthquakes}
                onChange={(e) =>
                  setPush({
                    ...push,
                    quakeMinMagnitude: Number(e.target.value),
                  })
                }
              >
                {[3, 4, 4.5, 5, 6].map((m) => (
                  <option key={m} value={m}>
                    M{m} {t("and above")}
                  </option>
                ))}
              </select>
              <select
                className="xc-select"
                aria-label={t("Within")}
                value={push.quakeRadiusKm}
                disabled={!push.earthquakes}
                onChange={(e) =>
                  setPush({ ...push, quakeRadiusKm: Number(e.target.value) })
                }
              >
                {[100, 300, 500, 1000, 2000].map((km) => (
                  <option key={km} value={km}>
                    {km} km {t("within")}
                  </option>
                ))}
              </select>
            </div>
            <p className="weather-note">
              {t(
                "Rain and warnings are checked every 5 minutes, earthquakes every 2 minutes. Each warning is sent once, and again when it is upgraded or lifted.",
              )}
            </p>
          </section>
        )}

        <section>
          <div className="weather-alert-head">
            <h3>{t("Rain alert")}</h3>
            {!alertMissing && (
              <Switch
                checked={rain.enabled}
                onChange={(v) => setRain({ ...rain, enabled: v })}
                label={t("Rain alert")}
              />
            )}
          </div>
          {alertMissing ? (
            <p className="weather-note">{t("Not live yet")}</p>
          ) : (
            <>
              <p className="weather-note">
                {t(
                  "Checked every 30 minutes. At most one notice every 6 hours.",
                )}
              </p>
              <div className="weather-alert-row">
                <label className="xc-field">
                  <span>{t("Look ahead")}</span>
                  <select
                    className="xc-select"
                    value={rain.leadHours}
                    disabled={!rain.enabled}
                    onChange={(e) =>
                      setRain({ ...rain, leadHours: Number(e.target.value) })
                    }
                  >
                    {[1, 2, 3, 4, 6].map((h) => (
                      <option key={h} value={h}>
                        {h} {t("hours")}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="xc-field">
                  <span>{t("When the chance of rain reaches")}</span>
                  <select
                    className="xc-select"
                    value={rain.threshold}
                    disabled={!rain.enabled}
                    onChange={(e) =>
                      setRain({ ...rain, threshold: Number(e.target.value) })
                    }
                  >
                    {[40, 50, 60, 70, 80, 90].map((p) => (
                      <option key={p} value={p}>
                        {p}%
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </>
          )}
        </section>
        {error && <p className="weather-error">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="button"
            className="xc-btn primary"
            disabled={pending}
            onClick={submit}
          >
            {t("Save")}
          </button>
        </div>
      </div>
    </Dialog>
  );
}
