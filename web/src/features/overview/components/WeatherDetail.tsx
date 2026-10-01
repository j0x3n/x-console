import { useState } from "react";
import { Link } from "react-router";
import {
  Activity,
  CloudRain,
  Moon,
  RefreshCw,
  Settings2,
  Sunrise,
  Sunset,
  TriangleAlert,
  Wind,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError, errorMessage, unwrap } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { formatTime, relativeTime } from "../../../lib/time";
import {
  briefApi,
  useWeatherExtra,
  weatherExtraKey,
  type Weather,
  type WeatherExtra,
} from "../../calendar/api";
import {
  airTone,
  compareYesterday,
  sortWarnings,
  warningLabel,
  warningTone,
} from "../weather";

/*
 * B58：点今日页的天气条打开。上面是现在的天气，下面是和风天气的预警、
 * 两小时降水、空气、生活指数、日出日落，以及附近的地震。
 */
export default function WeatherDetail({
  weather,
  onClose,
  onSettings,
}: {
  weather: Weather | undefined;
  onClose: () => void;
  onSettings: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const extra = useWeatherExtra();
  const [refreshing, setRefreshing] = useState(false);
  const notLive =
    extra.error instanceof ApiError &&
    (extra.error.status === 404 || extra.error.status === 501);
  const x = extra.data;

  const refresh = async () => {
    setRefreshing(true);
    try {
      const data = await unwrap(
        briefApi.GET("/weather/extra", {
          params: { query: { refresh: true } },
        }),
      );
      qc.setQueryData(weatherExtraKey, data);
    } catch (err) {
      toast({ message: errorMessage(err), tone: "error" });
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <Dialog open wide onClose={onClose} title={t("Weather")}>
      <div className="weather-detail">
        {weather && (
          <header className="weather-detail-now">
            <b>{Math.round(weather.temperature)}°</b>
            <span>
              {weather.location && <strong>{weather.location}</strong>}
              <span>
                {weather.summary} · {Math.round(weather.low)}°/
                {Math.round(weather.high)}°
                {x?.yesterday && ` · ${compareYesterday(weather, x.yesterday)}`}
              </span>
            </span>
          </header>
        )}

        {notLive ? (
          <p className="weather-note">
            {t(
              "Warnings, rain by the minute and earthquakes are not live yet.",
            )}
          </p>
        ) : extra.isPending ? (
          <p className="weather-note">{t("Loading")}…</p>
        ) : extra.isError ? (
          <p className="weather-error">{errorMessage(extra.error)}</p>
        ) : (
          x && <ExtraSections x={x} language={language} />
        )}

        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onSettings}>
            <Settings2 size={14} /> {t("Weather settings")}
          </button>
          <span className="xc-spacer" />
          {x && (
            <small className="weather-note weather-detail-time">
              {t("Updated at")} {formatTime(x.fetchedAt, language)}
            </small>
          )}
          {!notLive && (
            <button
              type="button"
              className="xc-btn"
              disabled={refreshing}
              onClick={refresh}
            >
              <RefreshCw size={14} className={refreshing ? "xc-spin" : ""} />{" "}
              {t("Refresh")}
            </button>
          )}
        </div>
      </div>
    </Dialog>
  );
}

function ExtraSections({
  x,
  language,
}: {
  x: WeatherExtra;
  language: "zh" | "en";
}) {
  const t = useT();
  const [openWarning, setOpenWarning] = useState<string | null>(null);
  const warnings = sortWarnings(x.warnings);
  const points = x.minutely?.points ?? [];
  const max = Math.max(0.5, ...points.map((p) => p.precip));
  return (
    <>
      {!x.configured && (
        <p className="weather-note weather-detail-setup">
          {t(
            "Add a QWeather key to see warnings, rain by the minute, air quality and more.",
          )}{" "}
          <Link to="/settings/brief">{t("Go to settings")}</Link>
        </p>
      )}

      {warnings.length > 0 && (
        <section>
          <h3>
            <TriangleAlert size={14} /> {t("Weather warnings")}
          </h3>
          <ul className="weather-warnings">
            {warnings.map((w) => (
              <li key={w.id}>
                <button
                  type="button"
                  aria-expanded={openWarning === w.id}
                  onClick={() =>
                    setOpenWarning(openWarning === w.id ? null : w.id)
                  }
                >
                  <span className={`xc-badge ${warningTone(w.level)}`}>
                    {warningLabel(w)}
                  </span>
                  <span className="weather-warning-sender">{w.sender}</span>
                  <small>{relativeTime(w.issuedAt, language)}</small>
                </button>
                {openWarning === w.id && <p>{w.text}</p>}
              </li>
            ))}
          </ul>
        </section>
      )}

      {x.minutely && (
        <section>
          <h3>
            <CloudRain size={14} /> {t("Next two hours")}
          </h3>
          <p className="weather-detail-line">{x.minutely.summary}</p>
          {points.some((p) => p.precip > 0) && (
            <div
              className="weather-minutely"
              role="img"
              aria-label={x.minutely.summary}
            >
              {points.map((p) => (
                <span
                  key={p.time}
                  className={p.kind === "snow" ? "snow" : ""}
                  style={{ height: `${Math.max(2, (p.precip / max) * 100)}%` }}
                  title={`${formatTime(p.time, language)} · ${p.precip} mm`}
                />
              ))}
            </div>
          )}
        </section>
      )}

      {(x.air || x.astronomy) && (
        <section className="weather-detail-facts">
          {x.air && (
            <div>
              <small>
                <Wind size={13} /> {t("Air quality")}
              </small>
              <strong className={`weather-tone-${airTone(x.air.level)}`}>
                {x.air.aqi} {x.air.category}
              </strong>
              {x.air.primary && (
                <small>
                  {t("Main pollutant")} {x.air.primary}
                </small>
              )}
            </div>
          )}
          {x.astronomy?.sunrise && (
            <div>
              <small>
                <Sunrise size={13} /> {t("Sunrise")}
              </small>
              <strong>{x.astronomy.sunrise}</strong>
            </div>
          )}
          {x.astronomy?.sunset && (
            <div>
              <small>
                <Sunset size={13} /> {t("Sunset")}
              </small>
              <strong>{x.astronomy.sunset}</strong>
            </div>
          )}
          {x.astronomy?.moonPhase && (
            <div>
              <small>
                <Moon size={13} /> {t("Moon")}
              </small>
              <strong>{x.astronomy.moonPhase}</strong>
            </div>
          )}
        </section>
      )}

      {x.indices.length > 0 && (
        <section>
          <h3>{t("Daily tips")}</h3>
          <div className="weather-indices">
            {x.indices.map((i) => (
              <div key={i.type} title={i.text}>
                <small>{i.name}</small>
                <strong>{i.category}</strong>
              </div>
            ))}
          </div>
        </section>
      )}

      <section>
        <h3>
          <Activity size={14} /> {t("Earthquakes nearby")}
        </h3>
        {x.earthquakes.length === 0 ? (
          <p className="weather-note">{t("No earthquakes nearby in 3 days")}</p>
        ) : (
          <ul className="weather-quakes">
            {x.earthquakes.map((q) => (
              <li key={q.id}>
                <b className={q.magnitude >= 5 ? "weather-tone-danger" : ""}>
                  M{q.magnitude.toFixed(1)}
                </b>
                <span>{q.place}</span>
                <small>
                  {Math.round(q.distanceKm)} km ·{" "}
                  {relativeTime(q.time, language)}
                </small>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}
