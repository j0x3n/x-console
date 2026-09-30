import { useEffect, useState } from "react";
import Dialog from "../../../components/ui/Dialog";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatDate } from "../../../lib/time";
import {
  useBoardArchive,
  useCardActions,
  type Board,
  type BoardList,
  type UpdateBoardList,
} from "../api";
import { STATUSES, STATUS_LABELS, type IssueStatus } from "../logic";
import { LIST_COLORS } from "./ListBoard";

/** 看板图标可选的 emoji。也可以自己输入。 */
const BOARD_ICONS = ["", "🕹️", "💡", "🛠️", "📝", "🎯", "🐞", "🚀", "📦", "🎨"];

/** 新建或编辑看板：名字、图标，新建时选列表模板。 */
export function BoardDialog({
  open,
  onClose,
  board,
  onSubmit,
}: {
  open: boolean;
  onClose: () => void;
  board?: Board;
  onSubmit: (v: {
    name: string;
    icon: string;
    preset?: "statuses" | "simple" | "empty";
  }) => Promise<unknown>;
}) {
  const t = useT();
  const [name, setName] = useState("");
  const [icon, setIcon] = useState("");
  const [preset, setPreset] = useState<"statuses" | "simple" | "empty">(
    "simple",
  );
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) return;
    setName(board?.name ?? "");
    setIcon(board?.icon ?? "");
    setPreset("simple");
  }, [open, board]);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={board ? t("Edit board") : t("New board")}
    >
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            await onSubmit({
              name: name.trim(),
              icon,
              preset: board ? undefined : preset,
            });
            onClose();
          } finally {
            setBusy(false);
          }
        }}
      >
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={60}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />
        </label>
        <div className="xc-field">
          <span>{t("Icon")}</span>
          <div className="projects-icons">
            {BOARD_ICONS.map((i) => (
              <button
                type="button"
                key={i || "none"}
                className={i === icon ? "on" : ""}
                aria-pressed={i === icon}
                aria-label={i || t("No icon")}
                onClick={() => setIcon(i)}
              >
                {i || "–"}
              </button>
            ))}
            <input
              className="xc-input projects-icon-input"
              value={icon}
              maxLength={4}
              aria-label={t("Custom icon")}
              onChange={(e) => setIcon(e.target.value)}
            />
          </div>
        </div>
        {!board && (
          <label className="xc-field">
            <span>{t("Start with")}</span>
            <select
              className="xc-select"
              value={preset}
              onChange={(e) => setPreset(e.target.value as typeof preset)}
            >
              <option value="simple">{t("To do, In progress, Done")}</option>
              <option value="statuses">{t("One list per status")}</option>
              <option value="empty">{t("No lists")}</option>
            </select>
          </label>
        )}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!name.trim() || busy}
          >
            {board ? t("Save") : t("Create board")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** 列表设置：名字、对应状态、颜色、在制品上限。 */
export function ListSettingsDialog({
  list,
  onClose,
  onSubmit,
}: {
  list: BoardList | null;
  onClose: () => void;
  onSubmit: (id: number, body: UpdateBoardList) => Promise<unknown>;
}) {
  const t = useT();
  const [name, setName] = useState("");
  const [status, setStatus] = useState<IssueStatus | "">("");
  const [color, setColor] = useState("");
  const [wip, setWip] = useState(0);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!list) return;
    setName(list.name);
    setStatus(list.status ?? "");
    setColor(list.color);
    setWip(list.wipLimit);
  }, [list]);
  return (
    <Dialog open={!!list} onClose={onClose} title={t("List settings")}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (!list) return;
          setBusy(true);
          try {
            await onSubmit(list.id, {
              name: name.trim(),
              status: status === "" ? null : status,
              color,
              wipLimit: wip,
            });
            onClose();
          } finally {
            setBusy(false);
          }
        }}
      >
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={60}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />
        </label>
        <label className="xc-field">
          <span>{t("Status of cards here")}</span>
          <select
            className="xc-select"
            value={status}
            onChange={(e) => setStatus(e.target.value as IssueStatus | "")}
          >
            <option value="">{t("Keep their status")}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(STATUS_LABELS[s])}
              </option>
            ))}
          </select>
          <small>
            {t(
              "A card dragged here gets this status. When a card's status changes elsewhere, it moves to the first list with that status.",
            )}
          </small>
        </label>
        <div className="xc-field">
          <span>{t("Color")}</span>
          <div className="projects-list-colors">
            {LIST_COLORS.map((c) => (
              <button
                type="button"
                key={c || "none"}
                className={`list-color-${c || "none"}${c === color ? " on" : ""}`}
                aria-label={c || t("No color")}
                aria-pressed={c === color}
                onClick={() => setColor(c)}
              />
            ))}
          </div>
        </div>
        <label className="xc-field">
          <span>{t("Work in progress limit")}</span>
          <input
            className="xc-input"
            type="number"
            min={0}
            max={999}
            value={wip}
            onChange={(e) => setWip(Math.max(0, Number(e.target.value) || 0))}
          />
          <small>
            {t("0 means no limit. Over the limit the count turns yellow.")}
          </small>
        </label>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!name.trim() || busy}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** 把一个列表的全部卡片移到另一个列表（可以是别的看板）。 */
export function MoveCardsDialog({
  list,
  boards,
  onClose,
  onSubmit,
}: {
  list: BoardList | null;
  boards: Board[];
  onClose: () => void;
  onSubmit: (from: number, to: number) => Promise<unknown>;
}) {
  const t = useT();
  const [to, setTo] = useState<number | "">("");
  useEffect(() => setTo(""), [list]);
  return (
    <Dialog
      open={!!list}
      onClose={onClose}
      title={`${t("Move all cards")}：${list?.name ?? ""}`}
    >
      <label className="xc-field">
        <span>{t("To list")}</span>
        <ListSelect
          boards={boards}
          value={to}
          exclude={list?.id}
          onChange={setTo}
        />
      </label>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          className="xc-btn primary"
          disabled={to === "" || !list}
          onClick={async () => {
            if (!list || to === "") return;
            await onSubmit(list.id, to);
            onClose();
          }}
        >
          {t("Move")}
        </button>
      </div>
    </Dialog>
  );
}

/** 按看板分组的列表下拉。 */
export function ListSelect({
  boards,
  value,
  exclude,
  onChange,
  label,
}: {
  boards: Board[];
  value: number | "";
  exclude?: number;
  onChange: (id: number) => void;
  label?: string;
}) {
  const t = useT();
  return (
    <select
      className="xc-select"
      value={value}
      aria-label={label}
      onChange={(e) => onChange(Number(e.target.value))}
    >
      <option value="" disabled>
        {t("Pick a list")}
      </option>
      {boards.map((b) => (
        <optgroup key={b.id} label={`${b.icon ? b.icon + " " : ""}${b.name}`}>
          {b.lists
            .filter((l) => l.id !== exclude)
            .map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
        </optgroup>
      ))}
    </select>
  );
}

/** 归档内容：归档的卡片可以恢复，归档的列表可以放回看板。 */
export function ArchiveDialog({
  board,
  open,
  onClose,
  onRestoreList,
  onOpenCard,
}: {
  board?: Board;
  open: boolean;
  onClose: () => void;
  onRestoreList: (list: BoardList) => Promise<unknown>;
  onOpenCard: (key: string) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const archive = useBoardArchive(board?.id, open);
  const cards = useCardActions();
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={`${t("Archived")}：${board?.name ?? ""}`}
      wide
    >
      {archive.isPending ? (
        <p className="xc-muted">{t("Loading")}…</p>
      ) : !archive.data ||
        (archive.data.issues.length === 0 &&
          archive.data.lists.length === 0) ? (
        <p className="xc-muted">{t("Nothing archived on this board.")}</p>
      ) : (
        <div className="projects-archive">
          {archive.data.lists.length > 0 && (
            <>
              <h3>{t("Lists")}</h3>
              {archive.data.lists.map((l) => (
                <div key={l.id} className="projects-archive-row">
                  <span>{l.name}</span>
                  <button
                    className="xc-btn small"
                    onClick={() => onRestoreList(l)}
                  >
                    {t("Restore")}
                  </button>
                </div>
              ))}
            </>
          )}
          {archive.data.issues.length > 0 && (
            <>
              <h3>{t("Cards")}</h3>
              {archive.data.issues.map((i) => (
                <div key={i.id} className="projects-archive-row">
                  <button
                    className="projects-archive-title"
                    onClick={() => onOpenCard(i.key)}
                  >
                    <span className="xc-mono">{i.key}</span> {i.title}
                  </button>
                  <small className="xc-muted">
                    {i.archivedAt ? formatDate(i.archivedAt, language) : ""}
                  </small>
                  <button
                    className="xc-btn small"
                    disabled={cards.restore.isPending}
                    onClick={() => cards.restore.mutate(i.key)}
                  >
                    {t("Restore")}
                  </button>
                </div>
              ))}
            </>
          )}
        </div>
      )}
    </Dialog>
  );
}
