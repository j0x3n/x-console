import { useState } from "react";
import { Link } from "react-router";
import { errorMessage, isNotLive } from "../../api/client";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCardSettings,
  usePutCardSettings,
  type CardSettings,
  type CardSettingsInput,
} from "./api";
import "./i18n";
import "./cards.css";

/**
 * 复习卡片的设置（B130）：每天的上限、目标记忆保持率、Telegram 推送和 AI。
 * 复习页的“设置”标签和设置页的“复习卡片”标签都用它。
 */
export default function CardsSettingsTab() {
  const t = useT();
  const settings = useCardSettings();
  const save = usePutCardSettings();

  if (settings.isPending) return <Loading />;
  if (settings.isError && isNotLive(settings.error))
    return <NotLive name={t("Review cards")} />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );

  const apply = (patch: CardSettingsInput) =>
    save.mutate(patch, {
      onSuccess: () => toast(t("Saved")),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });

  return (
    <div className="cards-settings">
      <LimitsCard data={settings.data} busy={save.isPending} onSave={apply} />
      <PushCard data={settings.data} busy={save.isPending} onSave={apply} />
      <section className="xc-card">
        <div className="xc-card-head">
          <h3>{t("Write cards with AI")}</h3>
        </div>
        <div className="cards-option">
          <span>{t("Let AI write a draft when I make a card")}</span>
          <Switch
            checked={settings.data.aiEnabled}
            label={t("Let AI write a draft when I make a card")}
            disabled={save.isPending}
            onChange={(aiEnabled) => apply({ aiEnabled })}
          />
        </div>
        <p className="xc-muted">
          {t(
            "When you make a card, the words you selected and the title of the note or article are sent to your AI provider. Nothing else is sent. Turn this off and you write the cards yourself.",
          )}
        </p>
        {!settings.data.aiAvailable && (
          <p className="xc-muted">
            {t("No AI model is set up yet.")}{" "}
            <Link to="/settings/assistant">{t("Set up AI")}</Link>
          </p>
        )}
      </section>
    </div>
  );
}

function LimitsCard({
  data,
  busy,
  onSave,
}: {
  data: CardSettings;
  busy: boolean;
  onSave: (patch: CardSettingsInput) => void;
}) {
  const t = useT();
  const [fresh, setFresh] = useState(String(data.newPerDay));
  const [reviews, setReviews] = useState(String(data.reviewPerDay));
  const [retention, setRetention] = useState(
    String(Math.round(data.retention * 100)),
  );
  const num = (s: string) => (s.trim() === "" ? NaN : Number(s));
  const valid =
    Number.isInteger(num(fresh)) &&
    num(fresh) >= 0 &&
    num(fresh) <= 500 &&
    Number.isInteger(num(reviews)) &&
    num(reviews) >= 0 &&
    num(reviews) <= 5000 &&
    Number.isInteger(num(retention)) &&
    num(retention) >= 70 &&
    num(retention) <= 97;
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h3>{t("Daily limits")}</h3>
      </div>
      <label className="xc-field">
        <span>{t("New cards per day")}</span>
        <input
          className="xc-input"
          type="number"
          min={0}
          max={500}
          value={fresh}
          onChange={(e) => setFresh(e.target.value)}
        />
      </label>
      <label className="xc-field">
        <span>{t("Reviews per day")}</span>
        <input
          className="xc-input"
          type="number"
          min={0}
          max={5000}
          value={reviews}
          onChange={(e) => setReviews(e.target.value)}
        />
      </label>
      <label className="xc-field">
        <span>{t("Target retention (%)")}</span>
        <input
          className="xc-input"
          type="number"
          min={70}
          max={97}
          value={retention}
          onChange={(e) => setRetention(e.target.value)}
        />
        <small>
          {t(
            "How likely you should still remember a card when it comes back. Higher means shorter intervals and more reviews. 90 is a good start.",
          )}
        </small>
      </label>
      <div className="xc-dialog-actions">
        <button
          className="xc-btn primary"
          disabled={busy || !valid}
          onClick={() =>
            onSave({
              newPerDay: num(fresh),
              reviewPerDay: num(reviews),
              retention: num(retention) / 100,
            })
          }
        >
          {t("Save")}
        </button>
      </div>
    </section>
  );
}

function PushCard({
  data,
  busy,
  onSave,
}: {
  data: CardSettings;
  busy: boolean;
  onSave: (patch: CardSettingsInput) => void;
}) {
  const t = useT();
  const [time, setTime] = useState(data.pushTime);
  const valid = /^([01]\d|2[0-3]):[0-5]\d$/.test(time);
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h3>{t("Daily reminder")}</h3>
      </div>
      <div className="cards-option">
        <span>{t("Remind me when cards are waiting")}</span>
        <Switch
          checked={data.pushEnabled}
          label={t("Remind me when cards are waiting")}
          disabled={busy}
          onChange={(pushEnabled) => onSave({ pushEnabled })}
        />
      </div>
      <label className="xc-field">
        <span>{t("Time of the reminder")}</span>
        <input
          className="xc-input cards-time"
          type="time"
          value={time}
          onChange={(e) => setTime(e.target.value)}
        />
        <small>
          {t(
            "Sent once a day, only when cards are waiting. It goes to the channels you chose for cards in Settings → Notifications. With Telegram you can answer the cards in the chat.",
          )}
        </small>
      </label>
      <div className="xc-dialog-actions">
        <button
          className="xc-btn primary"
          disabled={busy || !valid || time === data.pushTime}
          onClick={() => onSave({ pushTime: time })}
        >
          {t("Save")}
        </button>
      </div>
    </section>
  );
}
