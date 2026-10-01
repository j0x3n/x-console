import { useMemo } from "react";
import { isNotLive } from "../../api/client";
import {
  useGitHubConfig,
  usePulls,
  useRuns,
  useWatchedRepos,
  type WatchedRepo,
} from "./api";
import { fallbackRepoRows, repoKey, sortRepos, type RepoRow } from "./logic";

export function toRow(r: WatchedRepo): RepoRow {
  return { ...r, key: repoKey(r.connectionId, r.repo) };
}

/**
 * 仓库页左边的列表（B70）。/github/repos 上线前，用配置、PR、运行记录拼出来，
 * 这时 live 为 false，页面上一些字段（说明、私有、Issue 数）没有。
 */
export function useRepoList(enabled = true) {
  const watched = useWatchedRepos(enabled);
  const fallback = enabled && watched.isError && isNotLive(watched.error);
  const config = useGitHubConfig();
  const pulls = usePulls(fallback);
  const runs = useRuns(fallback);
  const rows = useMemo<RepoRow[]>(() => {
    if (watched.data) return sortRepos(watched.data.map(toRow));
    if (!fallback || !config.data) return [];
    return sortRepos(
      fallbackRepoRows(
        config.data,
        pulls.data ?? [],
        runs.data ?? [],
        config.data.apiUrl,
      ),
    );
  }, [watched.data, fallback, config.data, pulls.data, runs.data]);
  return {
    rows,
    live: !!watched.data,
    isPending: watched.isPending || (fallback && config.isPending),
    isError: watched.isError && !fallback,
    error: watched.error,
    refetch: () => watched.refetch(),
  };
}
