import { useMemo, useState, type KeyboardEvent } from "react";
import { Lock, Plus, RefreshCw, X } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { SearchBox } from "../../components/ui/Toolbar";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { useAvailableRepos, useRefreshAvailableRepos } from "./api";
import { filterRepos, isRepoName, MAX_REPOS, parseRepos } from "./logic";

/*
 * 关注的仓库（B35）：从令牌能访问的仓库里多选，可以搜索。
 * 后端还没上线、没有令牌或者取列表失败时，退回手动填写（一行一个）。
 */
export default function RepoPicker({
  value,
  onChange,
  hasToken,
}: {
  value: string[];
  onChange: (repos: string[]) => void;
  hasToken: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const available = useAvailableRepos(hasToken);
  const refresh = useRefreshAvailableRepos();
  const [q, setQ] = useState("");
  const [text, setText] = useState(value.join("\n"));

  const selected = useMemo(
    () => new Set(value.map((r) => r.toLowerCase())),
    [value],
  );
  const repos = available.data?.repos ?? [];
  const matches = useMemo(
    () =>
      filterRepos(repos, q)
        .filter((r) => !selected.has(r.fullName.toLowerCase()))
        .slice(0, 50),
    [repos, q, selected],
  );

  if (!available.data) {
    const reason =
      !hasToken || available.isPending
        ? null
        : isNotLive(available.error)
          ? null
          : errorMessage(available.error);
    return (
      <>
        <textarea
          className="xc-textarea xc-mono"
          rows={4}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            onChange(parseRepos(e.target.value));
          }}
          placeholder={"owner/name\nowner/another"}
          spellCheck={false}
          aria-label={t("Watched repositories")}
        />
        <small>
          {hasToken && available.isPending
            ? t("Loading repositories…")
            : t("One per line, as owner/name.")}
          {reason && <span className="xc-error-text"> {reason}</span>}
        </small>
      </>
    );
  }

  const full = value.length >= MAX_REPOS;
  const add = (name: string) => {
    if (full || selected.has(name.toLowerCase())) return;
    onChange([...value, name]);
    setQ("");
  };
  const typed = q.trim();
  const canAddTyped =
    isRepoName(typed) &&
    !selected.has(typed.toLowerCase()) &&
    !repos.some((r) => r.fullName.toLowerCase() === typed.toLowerCase());
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    if (matches[0]) add(matches[0].fullName);
    else if (canAddTyped) add(typed);
  };

  return (
    <div className="github-picker">
      {value.length > 0 ? (
        <ul className="github-picked" aria-label={t("Watched repositories")}>
          {value.map((name) => (
            <li key={name} className="xc-badge">
              <span className="xc-mono">{name}</span>
              <button
                type="button"
                aria-label={`${t("Remove")} ${name}`}
                onClick={() => onChange(value.filter((r) => r !== name))}
              >
                <X size={12} />
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="xc-muted github-picked-none">
          {t("No repositories yet. Pick some below.")}
        </p>
      )}
      <div className="github-picker-bar" onKeyDown={onKeyDown}>
        <SearchBox
          value={q}
          onChange={setQ}
          placeholder={t("Search repositories")}
          clearLabel={t("Clear")}
        />
        <button
          type="button"
          className="xc-btn small"
          disabled={refresh.isPending}
          title={t("Refresh repository list")}
          aria-label={t("Refresh repository list")}
          onClick={() =>
            refresh.mutate(undefined, {
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            })
          }
        >
          <RefreshCw
            size={14}
            className={refresh.isPending ? "github-spin" : undefined}
          />
        </button>
      </div>
      <ul
        className="github-options"
        role="listbox"
        aria-label={t("Repositories")}
      >
        {canAddTyped && (
          <li>
            <button type="button" disabled={full} onClick={() => add(typed)}>
              <Plus size={13} />
              <span className="xc-mono">{typed}</span>
              <small className="xc-muted">{t("Add by name")}</small>
            </button>
          </li>
        )}
        {matches.map((r) => (
          <li key={r.fullName}>
            <button
              type="button"
              role="option"
              aria-selected={false}
              disabled={full}
              onClick={() => add(r.fullName)}
            >
              <span className="xc-mono github-option-name">{r.fullName}</span>
              {r.private && (
                <Lock
                  size={12}
                  aria-label={t("Private")}
                  className="xc-muted"
                />
              )}
              {r.description && (
                <small className="xc-muted github-option-desc">
                  {r.description}
                </small>
              )}
            </button>
          </li>
        ))}
        {matches.length === 0 && !canAddTyped && (
          <li className="xc-muted github-options-empty">
            {repos.length === 0
              ? t("This token cannot see any repository.")
              : t("No matching repositories")}
          </li>
        )}
      </ul>
      <small className="xc-muted">
        {full
          ? t("You can watch up to 50 repositories.")
          : `${repos.length} ${t("repositories")} · ${t("list from")} ${relativeTime(available.data.fetchedAt, language)}`}
      </small>
    </div>
  );
}
