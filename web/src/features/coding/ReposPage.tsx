import { useEffect, useState } from "react";
import { Link } from "react-router";
import { FolderGit2, FolderSearch, Plus, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useAgents } from "../../api/core";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCodingSettings,
  useCreateRepo,
  useDeleteRepo,
  useDiscover,
  useRepos,
  useUpdateCodingSettings,
} from "./api";
import { confirmAction } from "../../components/ui/ConfirmDialog";

function RepoList() {
  const t = useT();
  const repos = useRepos();
  const remove = useDeleteRepo();
  if (repos.isPending) return <Loading />;
  if (repos.isError)
    return <ErrorState error={repos.error} onRetry={() => repos.refetch()} />;
  if (repos.data.length === 0) {
    return (
      <EmptyState
        title={t("No repositories yet")}
        icon={<FolderGit2 size={28} />}
      >
        <span>
          {t(
            "Scan a machine below and register the repositories you want to work on.",
          )}
        </span>
      </EmptyState>
    );
  }
  return (
    <div className="xc-table-wrap">
      <table className="xc-table">
        <thead>
          <tr>
            <th>{t("Name")}</th>
            <th>{t("Machine")}</th>
            <th>{t("Default branch")}</th>
            <th>GitHub</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {repos.data.map((r) => (
            <tr key={r.id}>
              <td>
                <strong className="coding-repo-name">{r.name}</strong>
                <div className="xc-mono xc-muted coding-path">{r.path}</div>
              </td>
              <td>
                <span className="xc-row">
                  <span className={`xc-dot ${r.agentOnline ? "ok" : ""}`} />
                  {r.agentName || r.agentId}
                </span>
              </td>
              <td className="xc-mono">{r.defaultBranch || "-"}</td>
              <td>{r.githubRepo || <span className="xc-muted">-</span>}</td>
              <td className="coding-cell-actions">
                <button
                  className="xc-btn ghost small"
                  aria-label={`${t("Remove")} ${r.name}`}
                  disabled={remove.isPending}
                  onClick={async () => {
                    if (
                      !(await confirmAction({
                        title: `${t("Remove")}“${r.name}”？`,
                        description: t(
                          "Its task history is deleted too. Files on disk stay.",
                        ),
                        confirmLabel: t("Remove"),
                      }))
                    )
                      return;
                    remove.mutate(r.id, {
                      onSuccess: () => toast(t("Removed")),
                      onError: (e) =>
                        toast({ message: errorMessage(e), tone: "error" }),
                    });
                  }}
                >
                  <Trash2 size={14} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Discover() {
  const t = useT();
  const agents = useAgents();
  const coding = (agents.data ?? []).filter((a) =>
    a.capabilities.includes("coding"),
  );
  const [agentId, setAgentId] = useState("");
  const [root, setRoot] = useState("");
  const [scan, setScan] = useState<{ agentId: string; root: string } | null>(
    null,
  );
  const [path, setPath] = useState("");
  const found = useDiscover(scan?.agentId ?? "", scan?.root ?? "", !!scan);
  const create = useCreateRepo();
  const agent = agentId || coding[0]?.id || "";

  useEffect(() => {
    setScan(null);
  }, [agentId]);

  const register = (p: string) =>
    create.mutate(
      { agentId: agent, path: p },
      {
        onSuccess: (repo) => {
          toast(`${t("Registered")} ${repo.name}`);
          setPath("");
        },
        onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
      },
    );

  if (agents.isSuccess && coding.length === 0) {
    return (
      <p className="xc-muted">
        {t(
          "No machine can run coding tasks yet. Pair the agent on your PC, then install Claude Code or Codex there.",
        )}{" "}
        <Link to="/settings/devices?add=desktop">{t("Add a computer")}</Link>
      </p>
    );
  }
  return (
    <div className="xc-stack">
      <div className="coding-form-row">
        <label className="xc-field">
          <span>{t("Machine")}</span>
          <select
            className="xc-select"
            value={agent}
            onChange={(e) => setAgentId(e.target.value)}
          >
            {coding.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
                {a.online ? "" : ` (${t("offline")})`}
              </option>
            ))}
          </select>
        </label>
        <label className="xc-field coding-grow">
          <span>{t("Folder to scan")}</span>
          <input
            className="xc-input"
            value={root}
            placeholder={t("Empty means the folders in the agent config")}
            onChange={(e) => setRoot(e.target.value)}
          />
        </label>
        <div className="xc-field coding-field-button">
          <span aria-hidden>&nbsp;</span>
          <button
            className="xc-btn"
            disabled={!agent}
            onClick={() => setScan({ agentId: agent, root: root.trim() })}
          >
            <FolderSearch size={14} /> {t("Scan")}
          </button>
        </div>
      </div>
      {scan &&
        (found.isFetching ? (
          <Loading />
        ) : found.isError ? (
          <ErrorState error={found.error} onRetry={() => found.refetch()} />
        ) : found.data ? (
          found.data.items.length === 0 ? (
            <p className="xc-muted">
              {t("No git repositories found in")} {found.data.roots.join(", ")}
            </p>
          ) : (
            <ul className="coding-found">
              {found.data.items.map((r) => (
                <li key={r.path}>
                  <div>
                    <strong>{r.name}</strong>
                    <div className="xc-mono xc-muted coding-path">{r.path}</div>
                  </div>
                  <span className="xc-mono xc-muted">{r.currentBranch}</span>
                  {r.repoId ? (
                    <span className="xc-badge ok">{t("Registered")}</span>
                  ) : (
                    <button
                      className="xc-btn small"
                      disabled={create.isPending}
                      onClick={() => register(r.path)}
                    >
                      <Plus size={13} /> {t("Register")}
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )
        ) : null)}
      <form
        className="coding-form-row"
        onSubmit={(e) => {
          e.preventDefault();
          if (path.trim()) register(path.trim());
        }}
      >
        <label className="xc-field coding-grow">
          <span>{t("Or register a path")}</span>
          <input
            className="xc-input xc-mono"
            value={path}
            placeholder={"C:\\Users\\me\\code\\project"}
            onChange={(e) => setPath(e.target.value)}
          />
        </label>
        <div className="xc-field coding-field-button">
          <span aria-hidden>&nbsp;</span>
          <button
            type="submit"
            className="xc-btn"
            disabled={!agent || !path.trim() || create.isPending}
          >
            <Plus size={14} /> {t("Register")}
          </button>
        </div>
      </form>
    </div>
  );
}

function RunnerSettings() {
  const t = useT();
  const settings = useCodingSettings();
  const update = useUpdateCodingSettings();
  const [max, setMax] = useState("");
  const [timeout, setTimeoutValue] = useState("");
  useEffect(() => {
    if (settings.data) {
      setMax(String(settings.data.maxConcurrent));
      setTimeoutValue(String(settings.data.defaultTimeoutMinutes));
    }
  }, [settings.data]);
  if (settings.isPending) return <Loading />;
  if (settings.isError)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  return (
    <form
      className="coding-form-row"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate(
          {
            maxConcurrent: Number(max),
            defaultTimeoutMinutes: Number(timeout),
          },
          {
            onSuccess: () => toast(t("Saved")),
            onError: (err) =>
              toast({ message: errorMessage(err), tone: "error" }),
          },
        );
      }}
    >
      <label className="xc-field">
        <span>{t("Tasks at the same time")}</span>
        <input
          className="xc-input"
          type="number"
          min={1}
          max={10}
          value={max}
          onChange={(e) => setMax(e.target.value)}
        />
        <small>{t("More tasks wait in the queue.")}</small>
      </label>
      <label className="xc-field">
        <span>{t("Time limit (minutes)")}</span>
        <input
          className="xc-input"
          type="number"
          min={1}
          max={1440}
          value={timeout}
          onChange={(e) => setTimeoutValue(e.target.value)}
        />
        <small>{t("A task is stopped after this.")}</small>
      </label>
      <div className="xc-field coding-field-button">
        <span aria-hidden>&nbsp;</span>
        <button
          type="submit"
          className="xc-btn primary"
          disabled={update.isPending}
        >
          {t("Save")}
        </button>
      </div>
    </form>
  );
}

export default function ReposPage() {
  const t = useT();
  return (
    <div className="xc-page coding-page">
      <PageHeading title={t("Repositories")} />
      <div className="xc-stack">
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Registered repositories")}</h2>
          </div>
          <RepoList />
        </section>
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Find repositories")}</h2>
          </div>
          <Discover />
        </section>
        <section className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Runner")}</h2>
          </div>
          <RunnerSettings />
        </section>
      </div>
    </div>
  );
}
