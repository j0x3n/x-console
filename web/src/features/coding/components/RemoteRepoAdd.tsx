import { useState } from "react";
import { Link } from "react-router";
import { Lock, Plus } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { useAgents as useMachines } from "../../../api/core";
import { SearchBox } from "../../../components/ui/Toolbar";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useConnections, useRemoteRepos } from "../../aiagents/api";
import { useCreateRepo, useRepos } from "../api";

/** B47：从 Git 连接里选仓库，登记到一台机器上，代理会 clone。 */
export default function RemoteRepoAdd() {
  const t = useT();
  const conns = useConnections();
  const machines = useMachines();
  const repos = useRepos();
  const create = useCreateRepo();
  const [connId, setConnId] = useState<number | undefined>();
  const [machine, setMachine] = useState("");
  const [q, setQ] = useState("");
  const [adding, setAdding] = useState("");
  const conn = connId ?? conns.data?.[0]?.id;
  const remote = useRemoteRepos(conn, q.trim());
  const coding = (machines.data ?? []).filter((m) =>
    m.capabilities.includes("coding"),
  );
  const target = machine || coding[0]?.id || "";
  if (conns.isPending) return <Loading />;
  if (conns.isError)
    return <ErrorState error={conns.error} onRetry={() => conns.refetch()} />;
  if (!conns.data.length)
    return (
      <p className="xc-muted">
        {t("No Git connections yet.")}{" "}
        <Link to="/coding/connections">{t("Add one")}</Link>
      </p>
    );
  const registered = (full: string) =>
    (repos.data ?? []).some(
      (r) => r.remoteRepo === full && r.agentId === target,
    );
  return (
    <div className="coding-remote">
      <div className="coding-remote-bar">
        <select
          className="xc-select"
          aria-label={t("Git connection")}
          value={conn}
          onChange={(e) => setConnId(Number(e.target.value))}
        >
          {conns.data.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <select
          className="xc-select"
          aria-label={t("Machine")}
          value={target}
          onChange={(e) => setMachine(e.target.value)}
        >
          {coding.length === 0 && (
            <option value="">{t("No machine can run tasks")}</option>
          )}
          {coding.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
              {m.online ? "" : ` (${t("offline")})`}
            </option>
          ))}
        </select>
        <SearchBox
          value={q}
          onChange={setQ}
          placeholder={t("Search repositories")}
        />
      </div>
      {remote.isPending ? (
        <Loading />
      ) : remote.isError ? (
        <ErrorState error={remote.error} onRetry={() => remote.refetch()} />
      ) : remote.data.length === 0 ? (
        <p className="xc-muted">{t("Nothing here")}</p>
      ) : (
        <ul className="coding-remote-list">
          {remote.data.map((r) => (
            <li key={r.fullName}>
              <div>
                <strong>
                  {r.fullName}{" "}
                  {r.private && <Lock size={12} aria-label={t("Private")} />}
                </strong>
                {r.description && (
                  <small className="xc-muted">{r.description}</small>
                )}
              </div>
              {registered(r.fullName) ? (
                <span className="xc-badge ok">{t("Already added")}</span>
              ) : (
                <button
                  className="xc-btn small"
                  disabled={!target || adding !== ""}
                  onClick={async () => {
                    setAdding(r.fullName);
                    try {
                      await create.mutateAsync({
                        agentId: target,
                        connectionId: conn,
                        remoteRepo: r.fullName,
                        cloneUrl: r.cloneUrl,
                      });
                      toast(t("Repository added"));
                    } catch (err) {
                      toast({ message: errorMessage(err), tone: "error" });
                    } finally {
                      setAdding("");
                    }
                  }}
                >
                  <Plus size={14} />
                  {adding === r.fullName ? t("Cloning…") : t("Add")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
