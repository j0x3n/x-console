import { useState } from "react";
import { History } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../lib/time";
import { QueryState } from "../overview/components/shared";
import { useMe } from "./api";
import MeDialog from "./MeDialog";
import {
  SOURCE_LABELS,
  STATE_LABELS,
  STATE_TONES,
  manualBadge,
  sinceText,
} from "./logic";
import "./i18n";
import "./me.css";

/** 今日页的“我的状态”卡片（B162）：现在的状态、从几点开始，点开可以手动改。 */
export default function TodayMeCard() {
  const t = useT();
  const language = useLanguage();
  const q = useMe();
  const [open, setOpen] = useState(false);
  if (q.isPending || q.isError) return <QueryState query={q} />;
  const s = q.data;
  const time = (d: Date) => formatTime(d, language);
  const date = (d: Date) => formatDate(d, language);
  const now = new Date();
  const source = SOURCE_LABELS[s.source];
  return (
    <div className="me-today">
      <div className="me-today-main">
        <span
          className={`xc-badge ${STATE_TONES[s.state]} me-today-state`}
          title={source ? t(source) : undefined}
        >
          {t(STATE_LABELS[s.state])}
        </span>
        {s.guessed && (
          <span
            className="xc-badge me-today-tag"
            title={t(SOURCE_LABELS.schedule)}
          >
            {t("Guessed")}
          </span>
        )}
        {s.manual && (
          <span className="xc-badge accent me-today-tag">
            {manualBadge(t, s, now, time, date)}
          </span>
        )}
      </div>
      <small className="xc-muted me-today-since">
        {t("Since {time}").replace(
          "{time}",
          sinceText(t, s.since, now, time, date),
        )}
      </small>
      <div className="me-today-actions">
        <button
          type="button"
          className="xc-btn small"
          onClick={() => setOpen(true)}
        >
          <History size={14} /> {t("Change or see history")}
        </button>
      </div>
      <MeDialog
        open={open}
        state={s}
        onClose={() => setOpen(false)}
        onError={(e) => toast({ message: errorMessage(e), tone: "error" })}
      />
    </div>
  );
}
