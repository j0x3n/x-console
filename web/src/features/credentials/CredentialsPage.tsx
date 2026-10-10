import { useState } from "react";
import { useSearchParams } from "react-router";
import { KeyRound, Plus } from "lucide-react";
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
import { useT } from "../../contexts/LanguageContext";
import {
  useCredentials,
  type CredentialItem,
  type CredentialKind,
} from "./api";
import CredentialDetail from "./CredentialDetail";
import CredentialDialog from "./CredentialDialog";
import { KINDS, KIND_ICONS, KIND_LABELS, dueText, statusTone } from "./format";
import "./i18n";
import "./credentials.css";

type View = "all" | "expired" | "soon" | "stale";

function matchesView(c: CredentialItem, view: View) {
  return view === "all" || c.status === view;
}

/** 密钥（B120）：记 API 密钥、令牌和 SSH 密钥的用处和期限，内容可以加密存。 */
export default function CredentialsPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const [archived, setArchived] = useState(false);
  const [q, setQ] = useState("");
  const [kind, setKind] = useState<CredentialKind | "">("");
  const [openId, setOpenId] = useState<number | null>(null);
  const list = useCredentials(archived);

  const viewParam = params.get("view");
  const view: View =
    viewParam === "expired" || viewParam === "soon" || viewParam === "stale"
      ? viewParam
      : "all";
  const setView = (next: View) =>
    setParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        if (next === "all" || prev.get("view") === next) p.delete("view");
        else p.set("view", next);
        return p;
      },
      { replace: true },
    );
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
    (c) =>
      matchesView(c, view) &&
      (!kind || c.kind === kind) &&
      (!needle ||
        `${c.name}\n${c.platform}\n${c.account}\n${c.usedBy.join("\n")}\n${c.scopes}\n${c.notes}`
          .toLowerCase()
          .includes(needle)),
  );
  const open = all.find((c) => c.id === openId) ?? null;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Credentials")} icon={<KeyRound size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (all.length === 0)
    body = (
      <EmptyState title={t("No credentials yet")} icon={<KeyRound size={28} />}>
        <span>
          {t(
            "Keep your API keys, tokens and SSH keys here: where each one is used and when it expires. You can also save the key itself, encrypted.",
          )}
        </span>
        <button
          className="xc-btn primary small"
          onClick={() => setCreating(true)}
        >
          <Plus size={14} /> {t("New credential")}
        </button>
      </EmptyState>
    );
  else if (items.length === 0)
    body = (
      <EmptyState
        title={t("No credential matches")}
        icon={<KeyRound size={28} />}
      />
    );
  else
    body = (
      <div className="xc-card credentials-list">
        {items.map((c) => (
          <CredentialRow key={c.id} item={c} onOpen={() => setOpenId(c.id)} />
        ))}
      </div>
    );

  const expired = summary?.expired ?? 0;
  const soon = summary?.soon ?? 0;
  const stale = summary?.stale ?? 0;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Credentials")}
        subtitle={
          expired > 0
            ? `${expired} ${t("credentials are expired")}`
            : summary
              ? t("Nothing needs attention")
              : undefined
        }
        aside={
          <button
            className="xc-btn primary"
            title={t("New credential")}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} /> {t("New credential")}
          </button>
        }
      />
      <StatStrip label={t("Credentials")}>
        <StatCard
          label={t("All credentials")}
          value={summary?.total ?? "–"}
          onClick={() => setView("all")}
        />
        <StatCard
          label={t("Expired")}
          value={summary?.expired ?? "–"}
          tone={expired > 0 ? "danger" : undefined}
          onClick={() => setView("expired")}
        />
        <StatCard
          label={t("Expiring soon")}
          value={summary?.soon ?? "–"}
          tone={soon > 0 ? "warn" : undefined}
          onClick={() => setView("soon")}
        />
        <StatCard
          label={t("Not rotated in time")}
          value={summary?.stale ?? "–"}
          tone={stale > 0 ? "warn" : undefined}
          onClick={() => setView("stale")}
        />
      </StatStrip>
      <Toolbar
        start={
          <div className="credentials-filters">
            <select
              className="xc-select"
              aria-label={t("Type")}
              value={kind}
              onChange={(e) => setKind(e.target.value as CredentialKind | "")}
            >
              <option value="">{t("All types")}</option>
              {KINDS.map((k) => (
                <option key={k} value={k}>
                  {t(KIND_LABELS[k])}
                </option>
              ))}
            </select>
            <label className="xc-check">
              <input
                type="checkbox"
                checked={archived}
                onChange={(e) => setArchived(e.target.checked)}
              />
              <span>{t("Show archived")}</span>
            </label>
          </div>
        }
        end={
          <SearchBox
            value={q}
            onChange={setQ}
            placeholder={t("Search credentials")}
            clearLabel={t("Clear")}
          />
        }
      />
      {body}
      <CredentialDialog
        open={creating}
        onClose={() => setCreating(false)}
        defaultKind={kind || undefined}
      />
      <CredentialDetail
        credential={open}
        onClose={() => setOpenId(null)}
        onSearch={(text) => {
          setQ(text);
          setOpenId(null);
        }}
      />
    </div>
  );
}

function CredentialRow({
  item: c,
  onOpen,
}: {
  item: CredentialItem;
  onOpen: () => void;
}) {
  const t = useT();
  const Icon = KIND_ICONS[c.kind];
  const where = c.usedBy.slice(0, 2).join("、");
  return (
    <button className="credentials-row" onClick={onOpen}>
      <span className="credentials-row-icon">
        <Icon size={17} />
      </span>
      <span className="credentials-row-main">
        <strong>{c.name}</strong>
        <small>
          {[c.platform, c.account].filter(Boolean).join(" · ") ||
            t(KIND_LABELS[c.kind])}
        </small>
        {where && (
          <small>
            {t("Used on")} {where}
            {c.usedBy.length > 2 &&
              `，${t("{n} places in total").replace("{n}", String(c.usedBy.length))}`}
          </small>
        )}
      </span>
      <span className="credentials-row-side">
        {c.archived ? (
          <span className="xc-badge">{t("Archived")}</span>
        ) : (
          <span className={`xc-badge ${statusTone(c.status)}`}>
            {dueText(t, c)}
          </span>
        )}
        {c.hint && <small className="xc-muted">…{c.hint}</small>}
      </span>
    </button>
  );
}
