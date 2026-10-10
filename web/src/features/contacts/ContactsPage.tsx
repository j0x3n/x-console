import { useState } from "react";
import { useSearchParams } from "react-router";
import { Phone, Plus, Users } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
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
import { toast } from "../../hooks/useToast";
import {
  useContacts,
  useTouchContact,
  type ContactGroup,
  type ContactItem,
} from "./api";
import ContactDetail from "./ContactDetail";
import ContactDialog from "./ContactDialog";
import {
  GROUPS,
  GROUP_ICONS,
  GROUP_LABELS,
  sinceText,
  statusText,
  statusTone,
} from "./format";
import "./i18n";
import "./contacts.css";

type View = "all" | "soon" | "overdue";

/** 联系人和重要日期（B122）：生日、纪念日和上次联系的日期，到期前提醒。 */
export default function ContactsPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const [archived, setArchived] = useState(false);
  const [q, setQ] = useState("");
  const [group, setGroup] = useState<ContactGroup | "">("");
  const [openId, setOpenId] = useState<number | null>(null);
  const list = useContacts(archived);

  const viewParam = params.get("view");
  const view: View =
    viewParam === "soon" || viewParam === "overdue" ? viewParam : "all";
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
      (view === "all" || c.status === view) &&
      (!group || c.group === group) &&
      (!needle ||
        `${c.name}\n${c.events.map((e) => e.label).join("\n")}\n${c.notes}`
          .toLowerCase()
          .includes(needle)),
  );
  const open = all.find((c) => c.id === openId) ?? null;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Contacts")} icon={<Users size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (all.length === 0)
    body = (
      <EmptyState title={t("No contacts yet")} icon={<Users size={28} />}>
        <span>
          {t(
            "Keep birthdays and other yearly dates, and when you were last in touch. You are reminded before a date, and when someone has not heard from you for too long.",
          )}
        </span>
        <button
          className="xc-btn primary small"
          onClick={() => setCreating(true)}
        >
          <Plus size={14} /> {t("New contact")}
        </button>
      </EmptyState>
    );
  else if (items.length === 0)
    body = (
      <EmptyState title={t("No contact matches")} icon={<Users size={28} />} />
    );
  else
    body = (
      <div className="xc-card contacts-list">
        {items.map((c) => (
          <ContactRow key={c.id} item={c} onOpen={() => setOpenId(c.id)} />
        ))}
      </div>
    );

  const soon = summary?.soon ?? 0;
  const overdue = summary?.overdue ?? 0;
  return (
    <div className="xc-page">
      <PageHeading
        title={t("Contacts")}
        subtitle={
          overdue > 0
            ? `${overdue} ${t("contacts have not been in touch for too long")}`
            : summary
              ? t("Nobody needs attention")
              : undefined
        }
        aside={
          <button
            className="xc-btn primary"
            title={t("New contact")}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} /> {t("New contact")}
          </button>
        }
      />
      <StatStrip label={t("Contacts")}>
        <StatCard
          label={t("All contacts")}
          value={summary?.total ?? "–"}
          onClick={() => setView("all")}
        />
        <StatCard
          label={t("Dates coming up")}
          value={summary?.soon ?? "–"}
          tone={soon > 0 ? "accent" : undefined}
          onClick={() => setView("soon")}
        />
        <StatCard
          label={t("Time to get in touch")}
          value={summary?.overdue ?? "–"}
          tone={overdue > 0 ? "warn" : undefined}
          onClick={() => setView("overdue")}
        />
      </StatStrip>
      <Toolbar
        start={
          <div className="contacts-filters">
            <select
              className="xc-select"
              aria-label={t("Group")}
              value={group}
              onChange={(e) => setGroup(e.target.value as ContactGroup | "")}
            >
              <option value="">{t("All groups")}</option>
              {GROUPS.map((g) => (
                <option key={g} value={g}>
                  {t(GROUP_LABELS[g])}
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
            placeholder={t("Search contacts")}
            clearLabel={t("Clear")}
          />
        }
      />
      {body}
      <ContactDialog
        open={creating}
        onClose={() => setCreating(false)}
        defaultGroup={group || undefined}
      />
      <ContactDetail contact={open} onClose={() => setOpenId(null)} />
    </div>
  );
}

function ContactRow({
  item: c,
  onOpen,
}: {
  item: ContactItem;
  onOpen: () => void;
}) {
  const t = useT();
  const touch = useTouchContact();
  const Icon = GROUP_ICONS[c.group];
  return (
    <div className="contacts-row">
      <button className="contacts-row-open" onClick={onOpen}>
        <span className="contacts-row-icon">
          <Icon size={17} />
        </span>
        <span className="contacts-row-main">
          <strong>{c.name}</strong>
          <small>
            {t("Last contact")} {sinceText(t, c.sinceContact)}
          </small>
        </span>
        <span className="contacts-row-side">
          {c.archived ? (
            <span className="xc-badge">{t("Archived")}</span>
          ) : (
            <span className={`xc-badge ${statusTone(c.status)}`}>
              {statusText(t, c)}
            </span>
          )}
        </span>
      </button>
      {!c.archived && (
        <button
          className="xc-btn small contacts-touch"
          title={t("Just contacted")}
          disabled={touch.isPending}
          onClick={() =>
            touch.mutate(
              { id: c.id },
              {
                onSuccess: () => toast(t("Contact recorded")),
                onError: (err) =>
                  toast({ message: errorMessage(err), tone: "error" }),
              },
            )
          }
        >
          <Phone size={13} /> {t("Just contacted")}
        </button>
      )}
    </div>
  );
}
