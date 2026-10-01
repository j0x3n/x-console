import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, createApi, isNotLive, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import { withElevation } from "../../auth/elevation";
import type {
  components as ghComponents,
  paths as ghPaths,
} from "../../api/gen/github";
import type {
  components as lnComponents,
  paths as lnPaths,
} from "../../api/gen/linear";

export const githubApi = createApi<ghPaths>();
export const linearApi = createApi<lnPaths>();

export type GitHubConfig = ghComponents["schemas"]["GitHubConfig"];
export type GitHubConfigInput = ghComponents["schemas"]["GitHubConfigInput"];
export type GitHubTestInput = ghComponents["schemas"]["GitHubTestInput"];
export type GitHubTestResult = ghComponents["schemas"]["GitHubTestResult"];
export type GitHubStatus = ghComponents["schemas"]["GitHubStatus"];
export type GitHubPull = ghComponents["schemas"]["GitHubPull"];
export type GitHubRun = ghComponents["schemas"]["GitHubRun"];
export type GitHubIssue = ghComponents["schemas"]["GitHubIssue"];
export type CheckState = ghComponents["schemas"]["GitHubCheckState"];
export type Forge = ghComponents["schemas"]["Forge"];
export type RepoWatch = ghComponents["schemas"]["RepoWatch"];
export type WatchedRepo = ghComponents["schemas"]["WatchedRepo"];
export type GitHubCommit = ghComponents["schemas"]["GitHubCommit"];
export type GitHubJob = ghComponents["schemas"]["GitHubJob"];
export type GitHubStep = ghComponents["schemas"]["GitHubStep"];
export type RepoNotify = ghComponents["schemas"]["RepoNotify"];
export type GitHubNotifySettings =
  ghComponents["schemas"]["GitHubNotifySettings"];
export type ReviewState = ghComponents["schemas"]["GitHubReviewState"];

export type LinearConfig = lnComponents["schemas"]["LinearConfig"];
export type LinearConfigInput = lnComponents["schemas"]["LinearConfigInput"];
export type LinearMappingInput = lnComponents["schemas"]["LinearMappingInput"];
export type LinearTestInput = lnComponents["schemas"]["LinearTestInput"];
export type LinearTestResult = lnComponents["schemas"]["LinearTestResult"];
export type LinearStatus = lnComponents["schemas"]["LinearStatus"];
export type LinearTeam = lnComponents["schemas"]["LinearTeam"];

export const githubKeys = {
  all: ["github"] as const,
  config: ["github", "config"] as const,
  status: ["github", "status"] as const,
  pulls: ["github", "pulls"] as const,
  runs: ["github", "runs"] as const,
  issues: ["github", "issues"] as const,
  availableRepos: ["github", "available-repos"] as const,
  availableFor: (connectionId: number) =>
    ["github", "available-repos", connectionId] as const,
  repos: ["github", "repos"] as const,
  commits: (repo: string, connectionId: number) =>
    ["github", "commits", connectionId, repo] as const,
  jobs: (runId: number) => ["github", "jobs", runId] as const,
  notify: ["github", "notify"] as const,
};

export const linearKeys = {
  all: ["linear"] as const,
  config: ["linear", "config"] as const,
  status: ["linear", "status"] as const,
  teams: ["linear", "teams"] as const,
};

// 同步完成、建了 PR 时整体刷新。
invalidateOn("github.", githubKeys.all);
invalidateOn("linear.", linearKeys.all);

export function isNotConfigured(error: unknown) {
  return (
    error instanceof ApiError && error.code === "integration_not_configured"
  );
}

// ---- GitHub ----

export function useGitHubConfig() {
  return useQuery({
    queryKey: githubKeys.config,
    queryFn: () => unwrap(githubApi.GET("/github/config")),
  });
}

export function useGitHubStatus() {
  return useQuery({
    queryKey: githubKeys.status,
    queryFn: () => unwrap(githubApi.GET("/github/status")),
  });
}

export function usePulls(enabled = true) {
  return useQuery({
    queryKey: githubKeys.pulls,
    queryFn: () => unwrap(githubApi.GET("/github/pulls")),
    enabled,
  });
}

export function useRuns(enabled = true) {
  return useQuery({
    queryKey: githubKeys.runs,
    queryFn: () =>
      unwrap(
        githubApi.GET("/github/runs", { params: { query: { limit: 100 } } }),
      ),
    enabled,
  });
}

export function useGitHubIssues(enabled = true) {
  return useQuery({
    queryKey: githubKeys.issues,
    queryFn: () => unwrap(githubApi.GET("/github/issues")),
    enabled,
  });
}

/** 改令牌或 API 地址时服务端要求再次验证，这里会弹出验证码框。 */
export function useSaveGitHubConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: GitHubConfigInput) =>
      withElevation(() => unwrap(githubApi.PUT("/github/config", { body }))),
    onSuccess: (data) => {
      qc.setQueryData(githubKeys.config, data);
      qc.invalidateQueries({ queryKey: githubKeys.all });
    },
  });
}

export function useTestGitHub() {
  return useMutation({
    mutationFn: (body: GitHubTestInput | null) =>
      body
        ? withElevation(() => unwrap(githubApi.POST("/github/test", { body })))
        : unwrap(githubApi.POST("/github/test")),
  });
}

export function useSyncGitHub() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(githubApi.POST("/github/sync")),
    onSuccess: (data) => {
      qc.setQueryData(githubKeys.status, data);
      qc.invalidateQueries({ queryKey: githubKeys.all });
    },
  });
}

// ---- Linear ----

export function useLinearConfig() {
  return useQuery({
    queryKey: linearKeys.config,
    queryFn: () => unwrap(linearApi.GET("/linear/config")),
  });
}

export function useLinearStatus() {
  return useQuery({
    queryKey: linearKeys.status,
    queryFn: () => unwrap(linearApi.GET("/linear/status")),
  });
}

export function useLinearTeams(enabled: boolean) {
  return useQuery({
    queryKey: linearKeys.teams,
    queryFn: () => unwrap(linearApi.GET("/linear/teams")),
    enabled,
    retry: false,
  });
}

export function useSaveLinearConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: LinearConfigInput) =>
      withElevation(() => unwrap(linearApi.PUT("/linear/config", { body }))),
    onSuccess: (data) => {
      qc.setQueryData(linearKeys.config, data);
      qc.invalidateQueries({ queryKey: linearKeys.all });
    },
  });
}

export function useTestLinear() {
  return useMutation({
    mutationFn: (body: LinearTestInput | null) =>
      body
        ? withElevation(() => unwrap(linearApi.POST("/linear/test", { body })))
        : unwrap(linearApi.POST("/linear/test")),
  });
}

export function useSyncLinear() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(linearApi.POST("/linear/sync")),
    onSuccess: (data) => {
      qc.setQueryData(linearKeys.status, data);
      qc.invalidateQueries({ queryKey: linearKeys.all });
      // 拉回来的 Issue 在项目页里显示。
      qc.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

/* ---- B35：令牌能访问的仓库 ---- */

export type GitHubAvailableRepos =
  ghComponents["schemas"]["GitHubAvailableRepos"];
export type GitHubAvailableRepo = GitHubAvailableRepos["repos"][number];

/**
 * 回 404 或 501 表示后端还没做，设置页退回手动填写。
 * B70：传了 connectionId 时列这个 Git 账号的仓库（GitHub 或 Forgejo）。
 */
export function useAvailableRepos(enabled: boolean, connectionId?: number) {
  return useQuery({
    queryKey: connectionId
      ? githubKeys.availableFor(connectionId)
      : githubKeys.availableRepos,
    queryFn: () =>
      unwrap(
        githubApi.GET("/github/available-repos", {
          params: { query: connectionId ? { connectionId } : {} },
        }),
      ),
    retry: (count, error) =>
      !isNotLive(error) && !isNotConfigured(error) && count < 2,
    staleTime: 5 * 60_000,
    enabled,
  });
}

export function useRefreshAvailableRepos(connectionId?: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      unwrap(
        githubApi.GET("/github/available-repos", {
          params: {
            query: connectionId
              ? { refresh: true, connectionId }
              : { refresh: true },
          },
        }),
      ),
    onSuccess: (data) =>
      qc.setQueryData(
        connectionId
          ? githubKeys.availableFor(connectionId)
          : githubKeys.availableRepos,
        data,
      ),
  });
}

/* ---- B70：关注的仓库、提交、CI 步骤 ---- */

/** 关注的仓库和概况。回 404 或 501 时页面用旧接口自己拼（见 useRepoList）。 */
export function useWatchedRepos(enabled = true) {
  return useQuery({
    queryKey: githubKeys.repos,
    queryFn: () => unwrap(githubApi.GET("/github/repos")),
    retry: (count, error) =>
      !isNotLive(error) && !isNotConfigured(error) && count < 2,
    meta: { silentError: true },
    enabled,
  });
}

/** 某个仓库（或全部）默认分支最近的提交。 */
export function useCommits(repo: string, connectionId: number) {
  return useQuery({
    queryKey: githubKeys.commits(repo, connectionId),
    queryFn: () =>
      unwrap(
        githubApi.GET("/github/commits", {
          params: {
            query: {
              ...(repo ? { repo } : {}),
              ...(connectionId ? { connectionId } : {}),
              limit: repo ? 30 : 60,
            },
          },
        }),
      ),
    retry: (count, error) => !isNotLive(error) && count < 2,
  });
}

/**
 * 一条运行的 job 和步骤。运行还没结束时每 5 秒取一次（GitHub 不推步骤进度，
 * 这是规格里唯一允许轮询的地方），结束后停止。
 */
export function useRunJobs(
  run: Pick<GitHubRun, "id" | "repo" | "status" | "connectionId">,
  enabled: boolean,
) {
  return useQuery({
    queryKey: githubKeys.jobs(run.id),
    queryFn: () =>
      unwrap(
        githubApi.GET("/github/runs/{runId}/jobs", {
          params: {
            path: { runId: run.id },
            query: {
              repo: run.repo,
              ...(run.connectionId ? { connectionId: run.connectionId } : {}),
            },
          },
        }),
      ),
    enabled,
    retry: (count, error) => !isNotLive(error) && count < 2,
    meta: { silentError: true },
    refetchInterval: run.status === "completed" ? false : 5000,
  });
}

/* ---- B71：仓库事件通知 ---- */

export function useGitHubNotify() {
  return useQuery({
    queryKey: githubKeys.notify,
    queryFn: () => unwrap(githubApi.GET("/github/notify")),
    retry: (count, error) => !isNotLive(error) && count < 2,
  });
}

export function useSaveGitHubNotify() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: GitHubNotifySettings) =>
      unwrap(githubApi.PUT("/github/notify", { body })),
    onSuccess: (data) => {
      qc.setQueryData(githubKeys.notify, data);
      qc.invalidateQueries({ queryKey: githubKeys.repos });
    },
  });
}
