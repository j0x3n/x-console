import { useMemo, useState } from "react";
import { Search, Star } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useCallService,
  useHAFavorites,
  useHAStates,
  useSaveFavorites,
  type HAState,
} from "../api";
import {
  displayName,
  domainLabel,
  groupByDomain,
  isOn,
  isUnavailable,
  matches,
  rowActions,
  stateLabel,
  type RowAction,
} from "../logic";
import EntityIcon from "./EntityIcon";

export default function EntitiesView() {
  const t = useT();
  const states = useHAStates();
  const favorites = useHAFavorites();
  const save = useSaveFavorites();
  const [query, setQuery] = useState("");
  const [domain, setDomain] = useState("");

  const groups = useMemo(
    () => groupByDomain(states.data ?? []),
    [states.data],
  );
  const visible = useMemo(
    () =>
      groups
        .filter((g) => !domain || g.domain === domain)
        .map((g) => ({ ...g, items: g.items.filter((s) => matches(s, query)) }))
        .filter((g) => g.items.length > 0),
    [groups, domain, query],
  );

  if (states.isPending) return <Loading />;
  if (states.isError)
    return <ErrorState error={states.error} onRetry={() => states.refetch()} />;

  const favIds = new Set((favorites.data ?? []).map((f) => f.entityId));
  const toggleFavorite = (entityId: string) => {
    const current = favorites.data ?? [];
    const items = favIds.has(entityId)
      ? current.filter((f) => f.entityId !== entityId)
      : [...current, { entityId, alias: "" }];
    save.mutate(
      items.map((f) => ({ entityId: f.entityId, alias: f.alias })),
      {
        onSuccess: () =>
          toast(
            favIds.has(entityId)
              ? t("Removed from favorites")
              : t("Added to favorites"),
          ),
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  return (
    <div className="xc-stack">
      <div className="home-filters">
        <label className="home-search">
          <Search size={15} aria-hidden />
          <input
            className="xc-input"
            type="search"
            placeholder={t("Search devices")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <select
          className="xc-select"
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
          aria-label={t("Type")}
        >
          <option value="">{t("All types")}</option>
          {groups.map((g) => (
            <option key={g.domain} value={g.domain}>
              {t(domainLabel(g.domain))} ({g.items.length})
            </option>
          ))}
        </select>
      </div>
      {visible.length === 0 ? (
        <EmptyState title={t("No matching devices")} />
      ) : (
        visible.map((group) => (
          <section className="xc-card home-group" key={group.domain}>
            <div className="xc-card-head">
              <h2>
                {t(domainLabel(group.domain))}{" "}
                <span className="xc-muted">{group.items.length}</span>
              </h2>
            </div>
            <ul className="home-rows">
              {group.items.map((state) => (
                <EntityRow
                  key={state.entityId}
                  state={state}
                  favorite={favIds.has(state.entityId)}
                  onFavorite={() => toggleFavorite(state.entityId)}
                  favoriteBusy={save.isPending}
                />
              ))}
            </ul>
          </section>
        ))
      )}
    </div>
  );
}

function EntityRow({
  state,
  favorite,
  onFavorite,
  favoriteBusy,
}: {
  state: HAState;
  favorite: boolean;
  onFavorite: () => void;
  favoriteBusy: boolean;
}) {
  const t = useT();
  const call = useCallService();
  const run = (a: RowAction) =>
    call.mutate(
      { domain: a.domain, service: a.service, entityId: state.entityId },
      {
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  const unavailable = isUnavailable(state);
  return (
    <li className={`home-row${unavailable ? " unavailable" : ""}`}>
      <span className={`home-row-icon${isOn(state) ? " on" : ""}`}>
        <EntityIcon entityId={state.entityId} state={state} />
      </span>
      <span className="home-row-name">
        <strong>{displayName(state.entityId, state)}</strong>
        <span className="xc-mono xc-muted">{state.entityId}</span>
      </span>
      <span className="home-row-state">{t(stateLabel(state))}</span>
      <span className="home-row-actions">
        {rowActions(state.entityId, state).map((a) => (
          <button
            key={a.service}
            className="xc-btn small"
            disabled={unavailable || call.isPending}
            onClick={() => run(a)}
          >
            {t(a.label)}
          </button>
        ))}
        <button
          className={`xc-btn small ghost home-star${favorite ? " active" : ""}`}
          aria-label={favorite ? t("Remove from favorites") : t("Add to favorites")}
          aria-pressed={favorite}
          title={favorite ? t("Remove from favorites") : t("Add to favorites")}
          disabled={favoriteBusy}
          onClick={onFavorite}
        >
          <Star size={15} fill={favorite ? "currentColor" : "none"} />
        </button>
      </span>
    </li>
  );
}
