import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  Download,
  ImageOff,
  Mail,
  MailOpen,
  Paperclip,
  Star,
  Trash2,
  WandSparkles,
} from "lucide-react";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes, formatDate, formatTime } from "../../../lib/time";
import { errorMessage } from "../../../api/client";
import { toast } from "../../../hooks/useToast";
import { useCreateReminder } from "../../reminders/api";
import {
  attachmentUrl,
  useBatchMail,
  useMailMessage,
  useTriageMailMessage,
  useUpdateMailMessage,
  type MailMessage,
} from "../api";
import {
  hasRemoteImages,
  mailDocument,
  reminderTimeFor,
  replaceCid,
  senderName,
  stripTrackingPixels,
  textToHtml,
} from "../logic";
import { triageLabel } from "../triage";

/** 阅读区：发件人、收件人、附件，正文放在沙盒 iframe 里（B53）。 */
export default function MessageView({
  id,
  onBack,
}: {
  id: number;
  onBack: () => void;
}) {
  const t = useT();
  const message = useMailMessage(id);
  const update = useUpdateMailMessage();
  const batch = useBatchMail();
  const triage = useTriageMailMessage();
  // 打开一封未读邮件时标成已读，同一封只标一次
  const marked = useRef<number | null>(null);
  useEffect(() => {
    const m = message.data;
    if (m && m.unread && marked.current !== m.id) {
      marked.current = m.id;
      update.mutate({ id: m.id, unread: false });
    }
  }, [message.data, update]);

  if (message.isPending) return <Loading />;
  if (message.isError)
    return (
      <ErrorState error={message.error} onRetry={() => message.refetch()} />
    );
  const m = message.data;
  return (
    <article className="mail-reader">
      <header className="mail-reader-head">
        <button
          type="button"
          className="xc-btn ghost small mail-back"
          onClick={onBack}
          aria-label={t("Back to list")}
        >
          <ArrowLeft size={15} />
        </button>
        <div className="mail-reader-title">
          <h2>{m.subject || t("(No subject)")}</h2>
          {m.triage && (
            <span className={`xc-badge ${triageLabel(m.triage).tone}`}>
              {t(triageLabel(m.triage).label)}
            </span>
          )}
        </div>
        <button
          type="button"
          className="xc-btn ghost small mail-sort-again"
          title={t("Sort again")}
          aria-label={t("Sort again")}
          disabled={triage.isPending}
          onClick={() =>
            triage.mutate(m.id, {
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            })
          }
        >
          <WandSparkles size={15} />
        </button>
        <button
          type="button"
          className="xc-btn ghost small"
          title={m.flagged ? t("Remove star") : t("Star")}
          aria-pressed={m.flagged}
          onClick={() => update.mutate({ id: m.id, flagged: !m.flagged })}
        >
          <Star size={15} className={m.flagged ? "mail-starred" : ""} />
        </button>
        <button
          type="button"
          className="xc-btn ghost small"
          title={t("Mark as unread")}
          onClick={() => {
            update.mutate({ id: m.id, unread: true });
            onBack();
          }}
        >
          <Mail size={15} />
        </button>
        <button
          type="button"
          className="xc-btn ghost small mail-danger"
          title={t("Delete")}
          aria-label={t("Delete")}
          disabled={batch.isPending}
          onClick={async () => {
            if (
              !(await confirmAction({
                title: `${t("Delete")} ${t("this mail")}？`,
                description: t(
                  "Mail goes to the Trash folder of your mailbox.",
                ),
              }))
            )
              return;
            batch.mutate(
              { ids: [m.id], action: "delete" },
              {
                onSuccess: (res) => {
                  if (res.errors.length) {
                    toast({ message: res.errors.join("\n"), tone: "error" });
                    return;
                  }
                  toast(t("Deleted"));
                  onBack();
                },
                onError: (e) => toast({ message: e.message, tone: "error" }),
              },
            );
          }}
        >
          <Trash2 size={15} />
        </button>
      </header>
      <MetaLines m={m} />
      {m.attachments.filter((a) => !a.contentId).length > 0 && (
        <ul className="mail-attachments">
          {m.attachments
            .filter((a) => !a.contentId)
            .map((a) => (
              <li key={a.index}>
                <a href={attachmentUrl(m.id, a.index)} download={a.name}>
                  <Paperclip size={13} />
                  <span>{a.name}</span>
                  <small>{formatBytes(a.size)}</small>
                  <Download size={13} />
                </a>
              </li>
            ))}
        </ul>
      )}
      <TriagePanel m={m} />
      <MailBody m={m} />
    </article>
  );
}

function MetaLines({ m }: { m: MailMessage }) {
  const t = useT();
  const language = useLanguage();
  const list = (xs: MailMessage["to"]) =>
    xs.map((x) => senderName(x)).join("、");
  return (
    <div className="mail-meta">
      <div>
        <strong>{senderName(m.from)}</strong>
        <small>&lt;{m.from.address}&gt;</small>
        <span className="xc-spacer" />
        <small>
          {formatDate(m.date, language)} {formatTime(m.date, language)}
        </small>
      </div>
      {m.to.length > 0 && (
        <small>
          {t("Mail to")}：{list(m.to)}
        </small>
      )}
      {m.cc.length > 0 && (
        <small>
          {t("Cc")}：{list(m.cc)}
        </small>
      )}
    </div>
  );
}

/**
 * 正文。外部图片默认显示，追踪打开用的 1 像素图片先去掉。
 * AI 分成营销的邮件不加载外部图片，点按钮后才显示，免得对方知道你打开了邮件。
 */
function MailBody({ m }: { m: MailMessage }) {
  const t = useT();
  const [allowImages, setAllowImages] = useState(false);
  const [height, setHeight] = useState(240);
  const frame = useRef<HTMLIFrameElement>(null);
  useEffect(() => setAllowImages(false), [m.id]);
  const clean = m.html ? stripTrackingPixels(m.html) : null;
  const html =
    clean !== null
      ? replaceCid(clean, m.attachments, (i) => attachmentUrl(m.id, i))
      : textToHtml(m.text);
  const remote = clean !== null && hasRemoteImages(clean);
  const showImages = allowImages || m.triage?.category !== "promo";
  const doc = useMemo(
    () => mailDocument(html, { origin: location.origin, showImages }),
    [html, showImages],
  );
  const fit = () => {
    const body = frame.current?.contentDocument?.body;
    if (body) setHeight(Math.min(20000, body.scrollHeight + 16));
  };
  return (
    <div className="mail-body">
      {remote && !showImages && (
        <div className="mail-images-off">
          <ImageOff size={14} />
          <span>{t("Remote images in promo mail are not loaded.")}</span>
          <button
            type="button"
            className="xc-btn small"
            onClick={() => setAllowImages(true)}
          >
            <MailOpen size={14} /> {t("Show images")}
          </button>
        </div>
      )}
      {/* 不给 allow-scripts：正文里的脚本不会运行。allow-same-origin 让内嵌图片能带上登录状态 */}
      <iframe
        ref={frame}
        title={m.subject || t("Mail body")}
        sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
        srcDoc={doc}
        style={{ height }}
        onLoad={fit}
      />
    </div>
  );
}

/** B136：AI 分拣的摘要，邮件里的日期可以一键加提醒。 */
function TriagePanel({ m }: { m: MailMessage }) {
  const t = useT();
  const remind = useCreateReminder();
  const result = m.triage;
  const addReminder = (date: string, text: string) => {
    const at = reminderTimeFor(date);
    if (!at) return;
    remind.mutate(
      {
        title: `${text || t("Mail deadline")}：${m.subject || t("(No subject)")}`,
        at: at.toISOString(),
        link: `/mail?m=${m.id}`,
      },
      {
        onSuccess: () => toast(t("Reminder added")),
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };
  if (!result || (!result.summary && result.dates.length === 0)) return null;
  return (
    <section className="mail-triage" aria-label={t("AI sorting")}>
      <div className="mail-triage-head">{result.summary}</div>
      {result.dates.length > 0 && (
        <ul className="mail-triage-dates">
          {result.dates.map((d) => {
            const at = reminderTimeFor(d.date);
            return (
              <li key={`${d.date}-${d.text}`}>
                <span>
                  {d.date} {d.text}
                </span>
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={!at || remind.isPending}
                  title={at ? undefined : t("It is too late to remind")}
                  aria-label={`${t("Mail remind me")} ${d.date} ${d.text}`}
                  onClick={() => addReminder(d.date, d.text)}
                >
                  {t("Mail remind me")}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
