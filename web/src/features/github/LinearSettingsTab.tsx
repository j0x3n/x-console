import { useEffect, useState, type FormEvent } from "react";
import { Plus, PlugZap, RefreshCw, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { useProjects } from "../projects/api";
import {
  useLinearConfig,
  useLinearStatus,
  useLinearTeams,
  useSaveLinearConfig,
  useSyncLinear,
  useTestLinear,
  type LinearConfig,
  type LinearMappingInput,
  type LinearTestResult,
} from "./api";
import { mappingProblem } from "./logic";
import "./i18n";
import "./github.css";

const DEFAULT_URL = "https://api.linear.app/graphql";

export default function LinearSettingsTab() {
  const config = useLinearConfig();
  if (config.isPending) return <Loading />;
  if (config.isError)
    return <ErrorState error={config.error} onRetry={() => config.refetch()} />;
  return (
    <div className="github-settings">
      <ConfigForm initial={config.data} />
      <StatusCard />
    </div>
  );
}

interface Row {
  teamId: string;
  projectId: number;
}

function ConfigForm({ initial }: { initial: LinearConfig }) {
  const t = useT();
  const save = useSaveLinearConfig();
  const test = useTestLinear();
  const sync = useSyncLinear();
  const teams = useLinearTeams(initial.hasKey);
  const projects = useProjects();
  const [apiKey, setApiKey] = useState("");
  const [apiUrl, setApiUrl] = useState(initial.apiUrl === DEFAULT_URL ? "" : initial.apiUrl);
  const [rows, setRows] = useState<Row[]>(() => initial.mappings.map((m) => ({ teamId: m.teamId, projectId: m.projectId })));
  const [result, setResult] = useState<LinearTestResult | null>(null);

  useEffect(() => {
    setApiUrl(initial.apiUrl === DEFAULT_URL ? "" : initial.apiUrl);
    setRows(initial.mappings.map((m) => ({ teamId: m.teamId, projectId: m.projectId })));
  }, [initial]);

  const urlChanged = (apiUrl.trim() || DEFAULT_URL) !== initial.apiUrl;
  const problem = mappingProblem(rows);

  // 团队列表取不到时（比如 key 失效），用已保存的名字显示。
  const teamOptions = teams.data ?? initial.mappings.map((m) => ({ id: m.teamId, key: m.teamKey, name: m.teamName }));

  const mappings = (): LinearMappingInput[] =>
    rows.map((r) => {
      const team = teamOptions.find((x) => x.id === r.teamId);
      return { teamId: r.teamId, projectId: r.projectId, teamKey: team?.key, teamName: team?.name };
    });

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    if (problem) {
      toast({ message: t(problem), tone: "error" });
      return;
    }
    save.mutate(
      { apiKey: apiKey.trim() || undefined, apiUrl: urlChanged ? apiUrl.trim() : undefined, mappings: mappings() },
      {
        onSuccess: (cfg) => {
          setApiKey("");
          toast(t("Saved"));
          if (cfg.hasKey && cfg.mappings.length > 0) sync.mutate();
        },
        onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  const onTest = () => {
    setResult(null);
    const body = apiKey.trim() || urlChanged ? { apiKey: apiKey.trim() || undefined, apiUrl: apiUrl.trim() || undefined } : null;
    test.mutate(body, {
      onSuccess: setResult,
      onError: (error) => setResult({ ok: false, message: errorMessage(error) }),
    });
  };

  const onClear = () => {
    save.mutate(
      { clearKey: true, mappings: mappings() },
      {
        onSuccess: () => toast(t("Key removed")),
        onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  const update = (i: number, patch: Partial<Row>) =>
    setRows((list) => list.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>Linear</h2>
      </div>
      <label className="xc-field">
        <span>{t("API key")}</span>
        <input
          className="xc-input"
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={initial.hasKey ? `${initial.apiKey}（${t("leave empty to keep")}）` : "lin_api_..."}
          autoComplete="new-password"
        />
        <small>在 Linear 的 Settings → Security &amp; access 里创建个人 API key。</small>
      </label>
      <label className="xc-field">
        <span>{t("API address")}</span>
        <input
          className="xc-input"
          value={apiUrl}
          onChange={(e) => setApiUrl(e.target.value)}
          placeholder={DEFAULT_URL}
          inputMode="url"
          autoComplete="off"
        />
        <small>一般留空。</small>
      </label>

      <div className="xc-field">
        <span>{t("Teams and projects")}</span>
        {!initial.hasKey ? (
          <small>先保存 API key，再选团队。</small>
        ) : (
          <>
            {teams.isError && <small className="xc-error-text">{errorMessage(teams.error)}</small>}
            <div className="linear-rows">
              {rows.map((r, i) => (
                <div className="linear-row" key={i}>
                  <select
                    className="xc-select"
                    aria-label={t("Linear team")}
                    value={r.teamId}
                    onChange={(e) => update(i, { teamId: e.target.value })}
                  >
                    <option value="">{t("Choose a team")}</option>
                    {teamOptions.map((team) => (
                      <option key={team.id} value={team.id}>
                        {team.key} · {team.name}
                      </option>
                    ))}
                  </select>
                  <span className="xc-muted" aria-hidden>
                    ⇄
                  </span>
                  <select
                    className="xc-select"
                    aria-label={t("Local project")}
                    value={r.projectId || ""}
                    onChange={(e) => update(i, { projectId: Number(e.target.value) })}
                  >
                    <option value="">{t("Choose a project")}</option>
                    {(projects.data ?? []).map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.key} · {p.name}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    className="xc-btn ghost small"
                    aria-label={t("Remove")}
                    onClick={() => setRows((list) => list.filter((_, j) => j !== i))}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
            <div>
              <button
                type="button"
                className="xc-btn small"
                onClick={() => setRows((list) => [...list, { teamId: "", projectId: 0 }])}
              >
                <Plus size={14} /> {t("Add a pair")}
              </button>
            </div>
            <small>
              对应好的团队和项目会双向同步：标题、描述、状态、优先级和截止日期。两边都改了时，以后改的为准。
              Linear 里已完成或已取消的 Issue 不会导入。
            </small>
          </>
        )}
      </div>

      {result && (
        <p className={`github-test-result ${result.ok ? "ok" : "fail"}`} role="status">
          {result.ok ? `${t("Key works")}：${result.user ?? ""}` : result.message}
        </p>
      )}
      <div className="xc-dialog-actions github-actions">
        {initial.hasKey && (
          <button type="button" className="xc-btn ghost danger" onClick={onClear} disabled={save.isPending}>
            {t("Remove key")}
          </button>
        )}
        <span className="xc-spacer" />
        <button
          type="button"
          className="xc-btn"
          disabled={test.isPending || (!initial.hasKey && !apiKey.trim())}
          onClick={onTest}
        >
          <PlugZap size={14} /> {test.isPending ? t("Testing") : t("Test key")}
        </button>
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
    </form>
  );
}

function StatusCard() {
  const t = useT();
  const language = useLanguage();
  const status = useLinearStatus();
  const sync = useSyncLinear();
  if (!status.data) return null;
  const s = status.data;
  const last = s.lastSync;
  const busy = sync.isPending || s.syncing;
  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Sync status")}</h2>
        {s.configured && (
          <button
            className="xc-btn small"
            disabled={busy}
            onClick={() =>
              sync.mutate(undefined, {
                onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
              })
            }
          >
            <RefreshCw size={14} className={busy ? "github-spin" : undefined} />
            {busy ? t("Syncing") : t("Sync now")}
          </button>
        )}
      </div>
      {!s.configured ? (
        <p className="xc-muted">{t("Not configured")}</p>
      ) : (
        <dl className="github-status">
          <dt>{t("Pairs")}</dt>
          <dd>{s.mappingCount}</dd>
          <dt>{t("Last sync")}</dt>
          <dd>
            {last ? (
              <>
                {relativeTime(last.at, language)}{" "}
                {last.ok ? (
                  <span className="xc-badge ok">{t("OK")}</span>
                ) : (
                  <span className="xc-badge danger">{t("Had errors")}</span>
                )}
              </>
            ) : (
              t("Never")
            )}
          </dd>
          {last && (
            <>
              <dt>{t("Changes")}</dt>
              <dd>
                {t("Imported")} {last.created} · {t("Pulled")} {last.pulled} · {t("Pushed")} {last.pushed}
                {last.conflicts > 0 && ` · ${t("Conflicts")} ${last.conflicts}`}
              </dd>
            </>
          )}
          {last && last.errors.length > 0 && (
            <>
              <dt>{t("Errors")}</dt>
              <dd>
                <ul className="github-errors">
                  {last.errors.map((e, i) => (
                    <li key={i} className="xc-error-text">
                      {e}
                    </li>
                  ))}
                </ul>
              </dd>
            </>
          )}
        </dl>
      )}
    </div>
  );
}
