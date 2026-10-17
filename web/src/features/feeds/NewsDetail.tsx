import { useEffect, useState } from "react";
import { ExternalLink } from "lucide-react";
import { errorMessage } from "../../api/client";
import Markdown from "../../components/markdown/Markdown";
import Dialog from "../../components/ui/Dialog";
import { Spinner } from "../../components/ui/States";
import { Segmented } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import { useNewsItem } from "./api";
import SaveLink from "./SaveLink";
import "./i18n";

type Version = "cn" | "original";

/** 一条新闻的详情：摘要、标签、正文（有译文时可以切换）、配图。 */
export default function NewsDetail({
  id,
  onClose,
}: {
  id: number | null;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const query = useNewsItem(id);
  const [version, setVersion] = useState<Version>("cn");
  useEffect(() => setVersion("cn"), [id]);
  if (id == null) return null;
  const item = query.data;
  const title = item ? item.titleCn || item.title : t("News");
  const content = item?.content;
  const hasCn = !!content?.markdownCn;
  const hasOriginal = !!content?.markdown;
  const showing =
    version === "cn" && hasCn
      ? content?.markdownCn
      : hasOriginal
        ? content?.markdown
        : content?.markdownCn;
  const summary = item ? item.summaryCn || item.summary : null;
  const when = item?.fetchedAt ?? item?.publishedAt;

  return (
    <Dialog open onClose={onClose} title={title} wide>
      {query.isPending && <Spinner />}
      {query.isError && (
        <p className="xc-error-text">{errorMessage(query.error)}</p>
      )}
      {item && (
        <>
          <p className="feeds-meta">
            <a href={item.url} target="_blank" rel="noopener noreferrer">
              <ExternalLink size={13} /> {item.source || item.url}
            </a>
            <span className="xc-muted">
              {item.category}
              {when && ` · ${relativeTime(when, language)}`}
            </span>
          </p>
          {item.titleCn && item.titleCn !== item.title && (
            <p className="xc-muted feeds-original-title">{item.title}</p>
          )}
          {summary && <p className="feeds-summary">{summary}</p>}
          {item.tags.length > 0 && (
            <p className="feeds-tagline">
              {item.tags.map((tag) => (
                <span key={tag} className="xc-badge">
                  {tag}
                </span>
              ))}
            </p>
          )}
          {hasCn && hasOriginal && (
            <Segmented<Version>
              label={t("Translation")}
              value={version}
              onChange={setVersion}
              options={[
                { value: "cn", label: t("Translation") },
                { value: "original", label: t("Original text") },
              ]}
            />
          )}
          {showing ? (
            <Markdown source={showing} className="feeds-article" />
          ) : (
            <p className="xc-muted">
              {t("The article text has not been fetched yet.")}
            </p>
          )}
          {content?.truncated && (
            <p className="xc-muted">
              {t("The article text is too long. Only the first part is shown.")}
            </p>
          )}
          {item.media.filter((m) => m.type !== "video").length > 0 && (
            <div className="feeds-media">
              {item.media
                .filter((m) => m.type !== "video")
                .map((m) => (
                  <a
                    key={m.url}
                    href={m.url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <img
                      src={m.url}
                      alt=""
                      loading="lazy"
                      referrerPolicy="no-referrer"
                      width={m.width ?? undefined}
                      height={m.height ?? undefined}
                    />
                  </a>
                ))}
            </div>
          )}
          <div className="xc-dialog-actions">
            <a
              className="xc-btn ghost"
              href={item.url}
              target="_blank"
              rel="noopener noreferrer"
            >
              <ExternalLink size={14} /> {t("Open the original page")}
            </a>
            <span className="xc-spacer" />
            <SaveLink url={item.url} label={title} />
          </div>
        </>
      )}
    </Dialog>
  );
}
