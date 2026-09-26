import { useState } from "react";
import { Check, ChevronDown, ChevronUp, Pencil, Star, X } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { EmptyState, ErrorState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useCallService,
  useHAFavorites,
  useSaveFavorites,
  type HAFavorite,
} from "../api";
import {
  displayName,
  isOn,
  isUnavailable,
  move,
  stateLabel,
  tapAction,
} from "../logic";
import EntityIcon from "./EntityIcon";

export default function FavoritesView({ onBrowse }: { onBrowse: () => void }) {
  const t = useT();
  const favorites = useHAFavorites();
  const save = useSaveFavorites();
  const [editing, setEditing] = useState(false);

  if (favorites.isPending) return <Loading />;
  if (favorites.isError)
    return (
      <ErrorState error={favorites.error} onRetry={() => favorites.refetch()} />
    );
  const list = favorites.data;
  if (list.length === 0)
    return (
      <EmptyState title={t("No favorites yet")} icon={<Star size={28} />}>
        <span>在“全部设备”里点星标，常用的设备就会出现在这里。</span>
        <button className="xc-btn small" onClick={onBrowse}>
          {t("Browse devices")}
        </button>
      </EmptyState>
    );

  const persist = (next: HAFavorite[]) =>
    save.mutate(
      next.map((f) => ({ entityId: f.entityId, alias: f.alias })),
      {
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );

  return (
    <>
      <div className="xc-row home-toolbar">
        <span className="xc-spacer" />
        <button
          className="xc-btn small ghost"
          onClick={() => setEditing(!editing)}
        >
          {editing ? <Check size={14} /> : <Pencil size={14} />}{" "}
          {editing ? t("Done editing") : t("Edit")}
        </button>
      </div>
      <div className="home-grid">
        {list.map((fav, index) => (
          <FavoriteCard
            key={fav.entityId}
            fav={fav}
            editing={editing}
            onRemove={() => persist(list.filter((_, i) => i !== index))}
            onMove={(delta) => persist(move(list, index, delta))}
          />
        ))}
      </div>
    </>
  );
}

function FavoriteCard({
  fav,
  editing,
  onRemove,
  onMove,
}: {
  fav: HAFavorite;
  editing: boolean;
  onRemove: () => void;
  onMove: (delta: -1 | 1) => void;
}) {
  const t = useT();
  const call = useCallService();
  const { state } = fav;
  const action = tapAction(fav.entityId, state);
  const name = displayName(fav.entityId, state, fav.alias);
  const unavailable = isUnavailable(state);
  const on = state ? isOn(state) : false;
  const className = [
    "home-card",
    on ? "on" : "",
    unavailable ? "unavailable" : "",
    action.kind !== "none" && !editing ? "tappable" : "",
  ]
    .filter(Boolean)
    .join(" ");

  const body = (
    <>
      <span className="home-card-icon">
        <EntityIcon entityId={fav.entityId} state={state} size={22} />
      </span>
      <span className="home-card-name">{name}</span>
      <span className="home-card-state">
        {state ? t(stateLabel(state)) : t("Unavailable")}
      </span>
    </>
  );

  if (editing)
    return (
      <div className={className}>
        {body}
        <div className="home-card-edit">
          <button
            className="xc-btn small ghost"
            aria-label={t("Move up")}
            onClick={() => onMove(-1)}
          >
            <ChevronUp size={14} />
          </button>
          <button
            className="xc-btn small ghost"
            aria-label={t("Move down")}
            onClick={() => onMove(1)}
          >
            <ChevronDown size={14} />
          </button>
          <button
            className="xc-btn small ghost"
            aria-label={t("Remove from favorites")}
            onClick={onRemove}
          >
            <X size={14} />
          </button>
        </div>
      </div>
    );

  if (action.kind === "none") return <div className={className}>{body}</div>;

  return (
    <button
      className={className}
      disabled={unavailable || call.isPending}
      aria-pressed={action.kind === "toggle" ? on : undefined}
      onClick={() =>
        call.mutate(
          {
            domain: action.domain,
            service: action.service,
            entityId: fav.entityId,
          },
          {
            onSuccess: () => {
              if (action.kind === "run") toast(`${t("Ran")} ${name}`);
            },
            onError: (error) =>
              toast({ message: errorMessage(error), tone: "error" }),
          },
        )
      }
    >
      {body}
    </button>
  );
}
