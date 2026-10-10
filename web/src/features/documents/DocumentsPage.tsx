import { useState } from "react";
import { useSearchParams } from "react-router";
import { Files, Plus } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { SearchBox, Toolbar } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { useDocuments, type DocumentItem, type DocumentKind } from "./api";
import DocumentDetail from "./DocumentDetail";
import DocumentDialog from "./DocumentDialog";
import {
  KINDS,
  daysText,
  isCustomKind,
  kindIcon,
  kindName,
  matchesView,
  statusTone,
  type View,
} from "./format";
import "./i18n";
import "./documents.css";

function shortDate(value: string, language: string): string {
  return new Date(`${value}T00:00:00`).toLocaleDateString(
    language === "zh" ? "zh-CN" : "en",
    { year: "numeric", month: "short", day: "numeric" },
  );
}

/** 证件档案（B115）：护照、合同、保险、物品保修，到期前提醒。 */
export default function DocumentsPage() {
  const t = useT();
  const language = useLanguage();
  const [params, setParams] = useSearchParams();
  const [archived, setArchived] = useState(false);
  const [q, setQ] = useState("");
  const [openId, setOpenId] = useState<number | null>(null);
  const list = useDocuments(archived);

  const view = (params.get("view") ?? "all") as View;
  const kindParam = params.get("kind");
  const kind =
    kindParam &&
    ((KINDS as string[]).includes(kindParam) || isCustomKind(kindParam))
      ? (kindParam as DocumentKind)
      : null;
  const creating = params.get("new") === "1";
  const setCreating = (on: boolean) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (on) next.set("new", "1");
        else next.delete("new");
        return next;
      },
      { replace: true },
    );

  const all = list.data?.items ?? [];
  const summary = list.data?.summary;
  const needle = q.trim().toLowerCase();
  const items = all.filter(
    (d) =>
      matchesView(d, view) &&
      (!kind || d.kind === kind) &&
      (!needle ||
        `${d.name}\n${d.holder}\n${d.serial}\n${d.notes}`
          .toLowerCase()
          .includes(needle)),
  );
  const open = all.find((d) => d.id === openId) ?? null;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Documents")} icon={<Files size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (all.length === 0)
    body = (
      <EmptyState title={t("No documents yet")} icon={<Files size={28} />}>
        <span>
          {t(
            "Keep passports, IDs, visas, contracts, insurance and warranties here. You get a reminder before each one expires.",
          )}
        </span>
        <button
          className="xc-btn primary small"
          onClick={() => setCreating(true)}
        >
          <Plus size={14} /> {t("New document")}
        </button>
      </EmptyState>
    );
  else if (items.length === 0)
    body = (
      <EmptyState title={t("No document matches")} icon={<Files size={28} />} />
    );
  else
    body = (
      <div className="xc-card documents-list">
        {items.map((d) => (
          <DocumentRow
            key={d.id}
            doc={d}
            language={language}
            onOpen={() => setOpenId(d.id)}
          />
        ))}
      </div>
    );

  const soon = summary?.soon ?? 0;
  const expired = summary?.expired ?? 0;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Documents")}
        subtitle={
          expired > 0
            ? `${expired} ${t("documents are expired")}`
            : summary
              ? t("Nothing expires soon")
              : undefined
        }
        aside={
          <button
            className="xc-btn primary"
            title={t("New document")}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} /> {t("New document")}
          </button>
        }
      />
      <StatStrip label={t("Documents")}>
        <StatCard
          label={t("All documents")}
          value={summary?.total ?? "–"}
          to="/documents"
        />
        <StatCard
          label={t("Expired")}
          value={summary?.expired ?? "–"}
          tone={expired > 0 ? "danger" : undefined}
          to="/documents?view=expired"
        />
        <StatCard
          label={t("Expiring soon")}
          value={summary?.soon ?? "–"}
          tone={soon > 0 ? "warn" : undefined}
          to="/documents?view=soon"
        />
        <StatCard
          label={t("Without expiry date")}
          value={summary?.none ?? "–"}
        />
      </StatStrip>
      <Toolbar
        start={
          <label className="xc-check">
            <input
              type="checkbox"
              checked={archived}
              onChange={(e) => setArchived(e.target.checked)}
            />
            <span>{t("Show archived")}</span>
          </label>
        }
        end={
          <SearchBox
            value={q}
            onChange={setQ}
            placeholder={t("Search documents")}
            clearLabel={t("Clear")}
          />
        }
      />
      {body}
      <DocumentDialog
        open={creating}
        onClose={() => setCreating(false)}
        defaultKind={kind ?? undefined}
      />
      <DocumentDetail document={open} onClose={() => setOpenId(null)} />
    </div>
  );
}

function DocumentRow({
  doc: d,
  language,
  onOpen,
}: {
  doc: DocumentItem;
  language: string;
  onOpen: () => void;
}) {
  const t = useT();
  const Icon = kindIcon(d.kind);
  return (
    <button className="documents-row" onClick={onOpen}>
      <span className="documents-row-icon">
        <Icon size={17} />
      </span>
      <span className="documents-row-main">
        <strong>{d.name}</strong>
        <small>
          {kindName(t, d.kind)}
          {d.holder && ` · ${d.holder}`}
        </small>
      </span>
      <span className="documents-row-side">
        {d.archived ? (
          <span className="xc-badge">{t("Archived")}</span>
        ) : (
          <span className={`xc-badge ${statusTone(d.status, d.daysLeft)}`}>
            {daysText(t, d.daysLeft)}
          </span>
        )}
        {d.expiresOn && (
          <small className="xc-muted">{shortDate(d.expiresOn, language)}</small>
        )}
      </span>
    </button>
  );
}
