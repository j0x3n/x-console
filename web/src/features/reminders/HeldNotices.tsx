import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { unwrap } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { everyMinuteOnPhone } from "../me/phone";
import { remindersApi, type HeldNotification } from "./api";
import "./i18n";
import "./reminders.css";

/**
 * B164：手机上的面板打开着时，取走等“回到电脑前”的通知，在这里列出来。
 * 取走的就不再推到外部渠道。
 */
export default function HeldNotices() {
  const t = useT();
  const language = useLanguage();
  const navigate = useNavigate();
  const [items, setItems] = useState<HeldNotification[]>([]);
  useEffect(
    () =>
      everyMinuteOnPhone(() => {
        unwrap(remindersApi.POST("/notify/digest/claim"))
          .then((res) => {
            if (res.items.length > 0) setItems((old) => [...old, ...res.items]);
          })
          .catch(() => {});
      }),
    [],
  );
  const close = () => setItems([]);
  return (
    <Dialog
      open={items.length > 0}
      onClose={close}
      title={t("While you were away from the computer")}
      footer={
        <button className="xc-btn small primary" onClick={close}>
          {t("Got it")}
        </button>
      }
    >
      <ul className="held-list">
        {items.map((it) => (
          <li key={it.id}>
            <button
              className="held-item"
              disabled={!it.link}
              onClick={() => {
                close();
                navigate(it.link);
              }}
            >
              <strong>{it.title}</strong>
              {it.body && <span className="xc-muted">{it.body}</span>}
              <small className="xc-muted">
                {relativeTime(it.createdAt, language)}
              </small>
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}
