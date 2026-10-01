import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  Download,
  ImageOff,
  Mail,
  MailOpen,
  Paperclip,
  Star,
} from "lucide-react";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes, formatDate, formatTime } from "../../../lib/time";
import {
  attachmentUrl,
  useMailMessage,
  useUpdateMailMessage,
  type MailMessage,
} from "../api";
import {
  hasRemoteImages,
  mailDocument,
  replaceCid,
  senderName,
  textToHtml,
} from "../logic";

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
        <h2>{m.subject || t("(No subject)")}</h2>
        <span className="xc-spacer" />
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

/** 正文。外部图片默认不加载，点按钮后才显示，免得对方知道你看了邮件。 */
function MailBody({ m }: { m: MailMessage }) {
  const t = useT();
  const [showImages, setShowImages] = useState(false);
  const [height, setHeight] = useState(240);
  const frame = useRef<HTMLIFrameElement>(null);
  useEffect(() => setShowImages(false), [m.id]);
  const html = m.html
    ? replaceCid(m.html, m.attachments, (i) => attachmentUrl(m.id, i))
    : textToHtml(m.text);
  const remote = !!m.html && hasRemoteImages(m.html);
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
          <span>{t("Remote images are hidden to protect your privacy.")}</span>
          <button
            type="button"
            className="xc-btn small"
            onClick={() => setShowImages(true)}
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
