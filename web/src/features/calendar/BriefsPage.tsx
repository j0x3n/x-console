import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Eye, Newspaper, Send, Settings2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatTime } from "../../lib/time";
import Markdown from "../../components/markdown/Markdown";
import {
  useBrief,
  useBriefSettings,
  useBriefs,
  useGenerateBrief,
  type Brief,
} from "./api";
import { dayKey, parseDayKey } from "./dates";

/** 早报历史：左边按日期列出，右边显示内容。也能立即预览或发送一份。 */
export default function BriefsPage() {
  const t = useT();
  const language = useLanguage();
  const [params, setParams] = useSearchParams();
  const list = useBriefs();
  const settings = useBriefSettings();
  const generate = useGenerateBrief();
  const [preview, setPreview] = useState<Brief | null>(null);
  const selected = params.get("date") ?? list.data?.[0]?.date ?? null;
  const brief = useBrief(preview ? null : selected);

  const select = (date: string) => {
    setPreview(null);
    setParams({ date }, { replace: true });
  };
  const run = (send: boolean) =>
    generate.mutate(send, {
      onSuccess: (b) => {
        if (send) {
          toast(t("Brief sent"));
          select(b.date);
        } else {
          setPreview(b);
        }
      },
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });

  const next = settings.data?.nextRunAt;
  const shown = preview ?? brief.data;

  return (
    <div className="xc-stack">
      <div className="xc-row brief-toolbar">
        <span className="xc-muted">
          {settings.data && !settings.data.enabled
            ? t("Scheduled sending is off.")
            : next
              ? `${t("Next brief")}: ${describeNext(next, language)}`
              : ""}
        </span>
        <span className="xc-spacer" />
        <Link className="xc-btn ghost small" to="/settings/brief">
          <Settings2 size={14} /> {t("Brief settings")}
        </Link>
        <button
          className="xc-btn small"
          disabled={generate.isPending}
          onClick={() => run(false)}
        >
          <Eye size={14} /> {t("Preview")}
        </button>
        <button
          className="xc-btn small primary"
          disabled={generate.isPending}
          onClick={() => run(true)}
        >
          <Send size={14} /> {t("Send now")}
        </button>
      </div>

      {list.isPending ? (
        <Loading />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => list.refetch()} />
      ) : list.data.length === 0 && !preview ? (
        <EmptyState title={t("No briefs yet")} icon={<Newspaper size={28} />}>
          <span>
            {t(
              "The first one arrives at the set time. You can preview it now.",
            )}
          </span>
        </EmptyState>
      ) : (
        <div className="brief-layout">
          <nav className="xc-card brief-list" aria-label={t("History")}>
            {preview && (
              <button className="active">
                <span>{t("Preview")}</span>
                <span className="xc-badge info">{t("Not saved")}</span>
              </button>
            )}
            {list.data.map((b) => (
              <button
                key={b.date}
                className={!preview && b.date === selected ? "active" : ""}
                onClick={() => select(b.date)}
              >
                <span>{formatDay(b.date, language)}</span>
                {b.sentAt ? (
                  <span className="xc-muted">
                    {formatTime(b.sentAt, language)}
                  </span>
                ) : (
                  <span className="xc-badge warn">{t("Not sent")}</span>
                )}
              </button>
            ))}
          </nav>
          <article className="xc-card brief-content">
            {shown ? (
              <Markdown source={shown.content} />
            ) : brief.isError ? (
              <ErrorState error={brief.error} />
            ) : (
              <Loading />
            )}
          </article>
        </div>
      )}
    </div>
  );
}

function formatDay(date: string, language: "zh" | "en"): string {
  const d = parseDayKey(date);
  if (!d) return date;
  return d.toLocaleDateString(language === "zh" ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    weekday: "short",
  });
}

function describeNext(iso: string, language: "zh" | "en"): string {
  const d = new Date(iso);
  const today = dayKey(new Date());
  const tomorrow = dayKey(new Date(Date.now() + 86_400_000));
  const time = formatTime(d, language);
  if (dayKey(d) === today)
    return `${language === "zh" ? "今天" : "today"} ${time}`;
  if (dayKey(d) === tomorrow)
    return `${language === "zh" ? "明天" : "tomorrow"} ${time}`;
  return `${formatDay(dayKey(d), language)} ${time}`;
}
