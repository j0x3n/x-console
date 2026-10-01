import { useEffect, useState, type FormEvent } from "react";
import { ExternalLink } from "lucide-react";
import { ApiError, errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { useQWeatherConfig, useSaveQWeatherConfig } from "./api";

/*
 * 设置 → 早报里的“和风天气”（B58）。填了以后今日页能看到天气预警、
 * 两小时降水、空气质量、生活指数，也能推送。key 只写不读。
 */
export default function QWeatherCard() {
  const t = useT();
  const language = useLanguage();
  const config = useQWeatherConfig();
  const save = useSaveQWeatherConfig();
  const [host, setHost] = useState("");
  const [key, setKey] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    if (config.data) setHost(config.data.apiHost);
  }, [config.data]);
  const notLive =
    config.error instanceof ApiError &&
    (config.error.status === 404 || config.error.status === 501);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setError("");
    const apiHost = host
      .trim()
      .replace(/^https?:\/\//, "")
      .replace(/\/+$/, "");
    if (!apiHost) return setError(t("Please enter the API host"));
    if (!config.data?.keySet && !key.trim())
      return setError(t("Please enter the API key"));
    save.mutate(
      { apiHost, ...(key.trim() ? { apiKey: key.trim() } : {}) },
      {
        onSuccess: () => {
          setKey("");
          toast(t("QWeather is connected"));
        },
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };

  const remove = async () => {
    if (
      !(await confirmAction({
        title: t("Remove the QWeather key?"),
        description: t(
          "Warnings, rain by the minute and air quality stop showing. Earthquakes still work.",
        ),
        confirmLabel: t("Remove"),
      }))
    )
      return;
    save.mutate(
      { apiHost: config.data?.apiHost ?? "", clearKey: true },
      {
        onSuccess: () => toast(t("Removed")),
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };

  return (
    <form
      className="xc-card brief-settings-card brief-qweather"
      onSubmit={submit}
    >
      <h3>{t("QWeather")}</h3>
      <p className="xc-muted brief-qweather-note">
        {t(
          "Adds weather warnings, rain for the next two hours, air quality, daily tips and sunrise times. The free plan is enough.",
        )}{" "}
        <a
          href="https://console.qweather.com"
          target="_blank"
          rel="noreferrer noopener"
        >
          {t("QWeather console")} <ExternalLink size={12} />
        </a>
      </p>
      {notLive ? (
        <p className="xc-muted">{t("Not live yet")}</p>
      ) : (
        <>
          <label className="xc-field">
            <span>API Host</span>
            <input
              className="xc-input"
              value={host}
              onChange={(e) => setHost(e.target.value)}
              placeholder="abc1234xyz.re.qweatherapi.com"
              spellCheck={false}
              autoCapitalize="off"
            />
            <small>{t("Console → Settings → API Host")}</small>
          </label>
          <label className="xc-field">
            <span>API KEY</span>
            <input
              className="xc-input"
              type="password"
              autoComplete="off"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder={
                config.data?.keySet
                  ? t("Saved. Leave empty to keep it.")
                  : undefined
              }
            />
            <small>
              {t("Console → Project → Credentials → API KEY")}
              {config.data?.checkedAt &&
                ` · ${t("Checked")} ${relativeTime(config.data.checkedAt, language)}`}
            </small>
          </label>
          {error && <p className="xc-error-text">{error}</p>}
          <div className="xc-row">
            <button className="xc-btn primary" disabled={save.isPending}>
              {save.isPending ? t("Checking…") : t("Save and check")}
            </button>
            {config.data?.keySet && (
              <button
                type="button"
                className="xc-btn ghost danger"
                disabled={save.isPending}
                onClick={remove}
              >
                {t("Remove key")}
              </button>
            )}
          </div>
        </>
      )}
    </form>
  );
}
