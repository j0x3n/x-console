import { useState } from "react";
import { Link } from "react-router";
import { FolderGit2, Lock, RefreshCw, Unlink } from "lucide-react";
import { errorMessage, isNotLive } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { SearchBox } from "../../../components/ui/Toolbar";
import { Loading, NotLive } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import { useConnections, useRemoteRepos } from "../../aiagents/api";
import { useBoardRepo, type Board } from "../api";

/**
 * B84：看板绑定 Git 仓库。没绑定时选账号和仓库；绑定后看同步状态、马上同步、解除绑定。
 * 接口还没上线时显示“还没上线”。
 */
export default function BindRepoDialog({
  board,
  onClose,
}: {
  board: Board;
  onClose: () => void;
}) {
  const t = useT();
  const ops = useBoardRepo(board.id);
  const [notLive, setNotLive] = useState(false);
  const [error, setError] = useState("");
  const run = async (fn: () => Promise<unknown>, done: string) => {
    setError("");
    try {
      await fn();
      toast(t(done));
      return true;
    } catch (err) {
      if (isNotLive(err)) setNotLive(true);
      else setError(errorMessage(err));
      return false;
    }
  };
  return (
    <Dialog open onClose={onClose} title={t("Link a repository")} wide>
      {notLive ? (
        <NotLive name="绑定仓库" />
      ) : board.repo ? (
        <Bound
          board={board}
          busy={
            ops.sync.isPending || ops.unbind.isPending || ops.bind.isPending
          }
          onSync={() => run(() => ops.sync.mutateAsync(), "Synced")}
          onToggle={(syncIssues) =>
            run(
              () =>
                ops.bind.mutateAsync({
                  connectionId: board.repo!.connectionId,
                  fullName: board.repo!.fullName,
                  syncIssues,
                }),
              "Saved",
            )
          }
          onUnbind={async () => {
            if (
              await confirmAction({
                title: `${t("Unlink")} ${board.repo!.fullName}？`,
                description: t(
                  "Cards synced from it stay on the board as normal cards.",
                ),
                confirmLabel: t("Unlink"),
              })
            )
              if (await run(() => ops.unbind.mutateAsync(), "Unlinked"))
                onClose();
          }}
        />
      ) : (
        <Picker
          busy={ops.bind.isPending}
          onCancel={onClose}
          onBind={async (connectionId, fullName, syncIssues) => {
            if (
              await run(
                () =>
                  ops.bind.mutateAsync({ connectionId, fullName, syncIssues }),
                "Repository linked",
              )
            )
              onClose();
          }}
        />
      )}
      {error && <p className="xc-error-text">{error}</p>}
    </Dialog>
  );
}

function Bound({
  board,
  busy,
  onSync,
  onToggle,
  onUnbind,
}: {
  board: Board;
  busy: boolean;
  onSync: () => void;
  onToggle: (syncIssues: boolean) => void;
  onUnbind: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const r = board.repo!;
  return (
    <div className="projects-bindrepo">
      <div className="projects-bindrepo-current">
        <FolderGit2 size={18} />
        <span>
          <a href={r.htmlUrl} target="_blank" rel="noreferrer">
            <strong>{r.fullName}</strong>
          </a>
          <small>
            {r.connectionName}
            {r.syncedCount != null &&
              ` · ${r.syncedCount} ${t("cards synced")}`}
            {r.lastSyncedAt &&
              ` · ${t("Synced")} ${relativeTime(r.lastSyncedAt, language)}`}
          </small>
        </span>
      </div>
      {r.lastError && (
        <p className="xc-error-text">
          {t("Last sync failed")}：{r.lastError}
        </p>
      )}
      <label className="xc-check">
        <input
          type="checkbox"
          checked={r.syncIssues}
          disabled={busy}
          onChange={(e) => onToggle(e.target.checked)}
        />
        {t("Sync the repository's issues to this board")}
      </label>
      <small className="xc-check-hint">
        {t(
          "Open issues become cards. Closing a card closes the issue. Cards you create on the board stay on the board.",
        )}
      </small>
      <div className="xc-dialog-actions">
        <button className="xc-btn danger" disabled={busy} onClick={onUnbind}>
          <Unlink size={14} /> {t("Unlink")}
        </button>
        <span className="xc-spacer" />
        <button className="xc-btn primary" disabled={busy} onClick={onSync}>
          <RefreshCw size={14} /> {t("Sync now")}
        </button>
      </div>
    </div>
  );
}

function Picker({
  busy,
  onCancel,
  onBind,
}: {
  busy: boolean;
  onCancel: () => void;
  onBind: (connectionId: number, fullName: string, syncIssues: boolean) => void;
}) {
  const t = useT();
  const conns = useConnections();
  const [connId, setConnId] = useState<number>();
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState("");
  const [syncIssues, setSyncIssues] = useState(true);
  const conn = connId ?? conns.data?.[0]?.id;
  const remote = useRemoteRepos(conn, q.trim());
  if (conns.isPending) return <Loading />;
  if (!conns.data?.length)
    return (
      <div className="projects-bindrepo-empty">
        <span className="xc-muted">{t("No Git accounts yet.")}</span>
        <Link className="xc-btn small" to="/settings/git">
          {t("Add one")}
        </Link>
      </div>
    );
  return (
    <form
      className="projects-bindrepo"
      onSubmit={(e) => {
        e.preventDefault();
        if (conn && picked) onBind(conn, picked, syncIssues);
      }}
    >
      <div className="projects-bindrepo-row">
        <select
          className="xc-select"
          aria-label={t("Git account")}
          value={conn}
          onChange={(e) => {
            setConnId(Number(e.target.value));
            setPicked("");
          }}
        >
          {conns.data.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <SearchBox
          value={q}
          onChange={setQ}
          placeholder={t("Search repositories")}
        />
      </div>
      <div className="projects-bindrepo-list" role="listbox">
        {remote.isPending ? (
          <Loading />
        ) : remote.isError ? (
          <p className="xc-error-text">{errorMessage(remote.error)}</p>
        ) : remote.data.length === 0 ? (
          <p className="xc-muted">{t("No repositories found")}</p>
        ) : (
          remote.data.slice(0, 50).map((r) => (
            <button
              type="button"
              role="option"
              aria-selected={picked === r.fullName}
              key={r.fullName}
              className={`projects-bindrepo-item${picked === r.fullName ? " active" : ""}`}
              onClick={() => setPicked(r.fullName)}
            >
              <FolderGit2 size={14} />
              <span>{r.fullName}</span>
              {r.private && <Lock size={12} />}
            </button>
          ))
        )}
      </div>
      <label className="xc-check">
        <input
          type="checkbox"
          checked={syncIssues}
          onChange={(e) => setSyncIssues(e.target.checked)}
        />
        {t("Sync the repository's issues to this board")}
      </label>
      <small className="xc-check-hint">
        {t(
          "Open issues become cards. Closing a card closes the issue. Cards you create on the board stay on the board.",
        )}
      </small>
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onCancel}>
          {t("Cancel")}
        </button>
        <button
          type="submit"
          className="xc-btn primary"
          disabled={!picked || busy}
        >
          {t("Link repository")}
        </button>
      </div>
    </form>
  );
}
