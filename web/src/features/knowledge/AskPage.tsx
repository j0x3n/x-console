import { useState, type FormEvent } from "react";
import { Link } from "react-router";
import { ExternalLink, Library, Settings } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useAskKnowledge,
  useKnowledgeSettings,
  useKnowledgeStatus,
  useSyncKnowledge,
  type KnowledgeAnswer,
  type KnowledgeHit,
} from "./api";
import { clip, isExternal, scoreText, SOURCE_LABELS } from "./format";
import KnowledgeSettingsDialog from "./KnowledgeSettingsDialog";
import "./i18n";
import "./knowledge.css";

/** 个人资料问答（B139）：问一句，找出相关的几段原文，AI 根据它们回答并列出出处。 */
export default function AskPage() {
  const t = useT();
  const status = useKnowledgeStatus();
  const settings = useKnowledgeSettings();
  const [settingsOpen, setSettingsOpen] = useState(false);

  let body;
  if (status.isPending) body = <Loading />;
  else if (status.isError && isNotLive(status.error))
    body = <NotLive name={t("Ask my data")} icon={<Library size={28} />} />;
  else if (status.isError)
    body = <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  else if (!status.data.enabled)
    body = (
      <EmptyState title={t("Ask off title")} icon={<Library size={28} />}>
        <span>{t("Ask off body")}</span>
        <button
          className="xc-btn primary"
          onClick={() => setSettingsOpen(true)}
        >
          {t("Ask turn on")}
        </button>
      </EmptyState>
    );
  else
    body = (
      <>
        <IndexCard status={status.data} />
        <Asker />
      </>
    );

  return (
    <div className="xc-page ask-page">
      <PageHeading
        title={t("Ask my data")}
        aside={
          <button
            className="xc-btn small"
            onClick={() => setSettingsOpen(true)}
          >
            <Settings size={14} /> {t("Ask settings")}
          </button>
        }
      />
      {body}
      <KnowledgeSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings.data}
      />
    </div>
  );
}

function IndexCard({
  status,
}: {
  status: NonNullable<ReturnType<typeof useKnowledgeStatus>["data"]>;
}) {
  const t = useT();
  const sync = useSyncKnowledge();
  const last = status.lastSyncAt
    ? new Date(status.lastSyncAt).toLocaleString()
    : t("Ask never synced");
  return (
    <section className="xc-card ask-index">
      <StatStrip label={t("Ask index")}>
        <StatCard
          label={t("Ask index")}
          value={status.docs}
          unit={t("Ask index docs")}
          foot={`${status.chunks} ${t("Ask index chunks")}${
            status.pending
              ? ` · ${status.pending} ${t("Ask index pending")}`
              : ""
          }`}
        />
        {status.sources.map((s) => (
          <StatCard
            key={s.source}
            label={t(SOURCE_LABELS[s.source] ?? s.source)}
            value={s.docs}
          />
        ))}
      </StatStrip>
      <div className="ask-index-foot">
        <span className="xc-muted">
          {t("Ask last sync")}：{last}
        </span>
        <button
          className="xc-btn small"
          disabled={status.syncing || sync.isPending}
          onClick={() =>
            sync.mutate(undefined, {
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            })
          }
        >
          {status.syncing ? t("Ask syncing") : t("Ask sync now")}
        </button>
      </div>
      {!status.available && (
        <p className="xc-error" role="alert">
          {t("Ask no provider")}
        </p>
      )}
      {status.lastError && (
        <p className="xc-error" role="alert">
          {t("Ask last error")}：{status.lastError}
        </p>
      )}
    </section>
  );
}

function Asker() {
  const t = useT();
  const ask = useAskKnowledge();
  const [question, setQuestion] = useState("");
  const [result, setResult] = useState<KnowledgeAnswer | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const q = question.trim();
    if (!q) return;
    ask.mutate(q, {
      onSuccess: setResult,
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  };

  return (
    <>
      <form className="xc-card ask-form" onSubmit={submit}>
        <textarea
          className="xc-textarea"
          rows={2}
          value={question}
          maxLength={500}
          aria-label={t("Ask question")}
          placeholder={t("Ask question hint")}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (
              e.key === "Enter" &&
              !e.shiftKey &&
              !e.nativeEvent.isComposing
            ) {
              e.preventDefault();
              submit(e);
            }
          }}
        />
        <div className="ask-form-actions">
          <button
            type="submit"
            className="xc-btn small primary"
            disabled={ask.isPending || !question.trim()}
          >
            {ask.isPending ? t("Ask thinking") : t("Ask button")}
          </button>
        </div>
      </form>
      {result && <Result result={result} />}
    </>
  );
}

function Result({ result }: { result: KnowledgeAnswer }) {
  const t = useT();
  return (
    <section className="xc-card ask-result" aria-label={t("Ask answer")}>
      {result.answer ? (
        <>
          <p className="ask-answer">{result.answer}</p>
          {result.sources.length > 0 && (
            <p className="xc-muted ask-hint">{t("Ask answer check")}</p>
          )}
        </>
      ) : (
        result.sources.length > 0 && (
          <p className="xc-muted">{t("Ask no answer")}</p>
        )
      )}
      {result.sources.length > 0 && (
        <>
          <h2 className="ask-title">{t("Ask sources")}</h2>
          <ol className="ask-sources-list">
            {result.sources.map((h, i) => (
              <SourceRow key={i} hit={h} />
            ))}
          </ol>
        </>
      )}
    </section>
  );
}

function SourceRow({ hit }: { hit: KnowledgeHit }) {
  const t = useT();
  const open = isExternal(hit.link) ? (
    <a href={hit.link} target="_blank" rel="noreferrer noopener">
      {t("Ask source open")} <ExternalLink size={12} />
    </a>
  ) : (
    <Link to={hit.link}>{t("Ask source open")}</Link>
  );
  return (
    <li className="ask-source">
      <div className="ask-source-head">
        <strong>{hit.title}</strong>
        <span className="xc-badge">
          {t(SOURCE_LABELS[hit.source] ?? hit.source)}
        </span>
        <span className="xc-muted">{scoreText(hit.score)}</span>
        {open}
      </div>
      <p className="ask-snippet xc-muted">{clip(hit.snippet)}</p>
    </li>
  );
}
