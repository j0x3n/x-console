import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { createApi, errorMessage, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components } from "../../api/gen/projects";
import type { paths } from "../../api/gen/projects";
import { toast } from "../../hooks/useToast";
import { applyMove, type Issue, type IssueStatus, type MovePlan } from "./logic";

export const projectsApi = createApi<paths>();

export type Project = components["schemas"]["Project"];
export type Label = components["schemas"]["Label"];
export type Milestone = components["schemas"]["Milestone"];
export type Comment = components["schemas"]["Comment"];
export type IssueLink = components["schemas"]["IssueLink"];
export type IssueLinkKind = components["schemas"]["IssueLinkKind"];
export type CreateIssue = components["schemas"]["CreateIssue"];
export type UpdateIssue = components["schemas"]["UpdateIssue"];
export type UpdateProject = components["schemas"]["UpdateProject"];
export type { Issue, IssueStatus };

export const projectKeys = {
  all: ["projects"] as const,
  list: (archived = false) => ["projects", "list", archived] as const,
  issues: (projectId: number) => ["projects", "issues", projectId] as const,
  myIssues: ["projects", "my-issues"] as const,
  issue: (key: string) => ["projects", "issue", key.toUpperCase()] as const,
  labels: (projectId: number) => ["projects", "labels", projectId] as const,
  milestones: (projectId: number) =>
    ["projects", "milestones", projectId] as const,
  comments: (key: string) => ["projects", "comments", key.toUpperCase()] as const,
  links: (key: string) => ["projects", "links", key.toUpperCase()] as const,
};

// 其他窗口或其他模块改了数据时，服务端会推事件，这里整体刷新。
for (const prefix of [
  "project.",
  "issue.",
  "issue_comment.",
  "issue_link.",
  "label.",
  "milestone.",
])
  invalidateOn(prefix, projectKeys.all);

export function useProjects(archived = false) {
  return useQuery({
    queryKey: projectKeys.list(archived),
    queryFn: () =>
      unwrap(
        projectsApi.GET("/projects", {
          params: { query: archived ? { archived: true } : {} },
        }),
      ),
  });
}

/** 按 key 找项目（包括已归档的）。 */
export function useProjectByKey(key: string | undefined) {
  const active = useProjects(false);
  const archived = useProjects(true);
  const upper = key?.toUpperCase();
  const project =
    active.data?.find((p) => p.key === upper) ??
    archived.data?.find((p) => p.key === upper);
  return {
    project,
    isPending: active.isPending || archived.isPending,
    error: active.error ?? archived.error,
  };
}

/** 一次取完一个项目的全部 Issue，筛选在前端做。 */
async function fetchAllIssues(query: Record<string, unknown>): Promise<Issue[]> {
  const all: Issue[] = [];
  let cursor: string | undefined;
  do {
    const page = await unwrap(
      projectsApi.GET("/issues", {
        params: { query: { ...query, limit: 1000, cursor } },
      }),
    );
    all.push(...page.items);
    cursor = page.nextCursor;
  } while (cursor);
  return all;
}

export function useIssues(projectId: number | undefined) {
  return useQuery({
    queryKey: projectKeys.issues(projectId ?? 0),
    queryFn: () => fetchAllIssues({ projectId, sort: "manual" }),
    enabled: projectId !== undefined,
  });
}

/** 跨项目的未完成 Issue，按截止日期排序。 */
export function useMyIssues() {
  return useQuery({
    queryKey: projectKeys.myIssues,
    queryFn: async () =>
      (
        await unwrap(
          projectsApi.GET("/issues", {
            params: {
              query: {
                status: ["backlog", "todo", "in_progress", "in_review"],
                sort: "due",
                limit: 50,
              },
            },
          }),
        )
      ).items,
  });
}

export function useIssue(key: string | undefined) {
  return useQuery({
    queryKey: projectKeys.issue(key ?? ""),
    queryFn: () =>
      unwrap(
        projectsApi.GET("/issues/{key}", { params: { path: { key: key! } } }),
      ),
    enabled: !!key,
  });
}

export function useLabels(projectId: number | undefined) {
  return useQuery({
    queryKey: projectKeys.labels(projectId ?? 0),
    queryFn: () =>
      unwrap(
        projectsApi.GET("/projects/{projectId}/labels", {
          params: { path: { projectId: projectId! } },
        }),
      ),
    enabled: projectId !== undefined,
  });
}

export function useMilestones(projectId: number | undefined) {
  return useQuery({
    queryKey: projectKeys.milestones(projectId ?? 0),
    queryFn: () =>
      unwrap(
        projectsApi.GET("/projects/{projectId}/milestones", {
          params: { path: { projectId: projectId! } },
        }),
      ),
    enabled: projectId !== undefined,
  });
}

export function useComments(key: string) {
  return useQuery({
    queryKey: projectKeys.comments(key),
    queryFn: () =>
      unwrap(
        projectsApi.GET("/issues/{key}/comments", { params: { path: { key } } }),
      ),
  });
}

export function useLinks(key: string) {
  return useQuery({
    queryKey: projectKeys.links(key),
    queryFn: () =>
      unwrap(
        projectsApi.GET("/issues/{key}/links", { params: { path: { key } } }),
      ),
  });
}

const fail = (error: unknown) =>
  toast({ message: errorMessage(error), tone: "error" });

/** 写入一个 Issue 的最新值到所有相关缓存。 */
function storeIssue(qc: QueryClient, issue: Issue) {
  qc.setQueryData(projectKeys.issue(issue.key), issue);
  qc.setQueryData<Issue[]>(projectKeys.issues(issue.projectId), (list) =>
    list?.map((i) => (i.id === issue.id ? issue : i)),
  );
}

export function useCreateProject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components["schemas"]["CreateProject"]) =>
      unwrap(projectsApi.POST("/projects", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.all }),
  });
}

export function useUpdateProject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateProject }) =>
      unwrap(
        projectsApi.PATCH("/projects/{projectId}", {
          params: { path: { projectId: id } },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.all }),
    onError: fail,
  });
}

export function useCreateIssue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, body }: { projectId: number; body: CreateIssue }) =>
      unwrap(
        projectsApi.POST("/projects/{projectId}/issues", {
          params: { path: { projectId } },
          body,
        }),
      ),
    onSuccess: (issue) => {
      qc.setQueryData<Issue[]>(projectKeys.issues(issue.projectId), (list) =>
        list ? [...list, issue] : list,
      );
      qc.invalidateQueries({ queryKey: projectKeys.all });
    },
  });
}

/** 改 Issue。先改缓存（乐观更新），失败时回滚。 */
export function useUpdateIssue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, body }: { issue: Issue; key: string; body: UpdateIssue }) =>
      unwrap(
        projectsApi.PATCH("/issues/{key}", { params: { path: { key } }, body }),
      ),
    onMutate: async ({ issue, body }) => {
      const listKey = projectKeys.issues(issue.projectId);
      await qc.cancelQueries({ queryKey: listKey });
      const prevList = qc.getQueryData<Issue[]>(listKey);
      const prevIssue = qc.getQueryData<Issue>(projectKeys.issue(issue.key));
      const patched: Issue = {
        ...issue,
        ...(body.title !== undefined && { title: body.title }),
        ...(body.description !== undefined && { description: body.description }),
        ...(body.status !== undefined && { status: body.status }),
        ...(body.priority !== undefined && { priority: body.priority }),
        ...(body.dueDate !== undefined && { dueDate: body.dueDate ?? undefined }),
        ...(body.milestoneId !== undefined && {
          milestoneId: body.milestoneId ?? undefined,
        }),
      };
      storeIssue(qc, patched);
      return { listKey, prevList, prevIssue };
    },
    onError: (error, { issue }, ctx) => {
      if (ctx) {
        qc.setQueryData(ctx.listKey, ctx.prevList);
        qc.setQueryData(projectKeys.issue(issue.key), ctx.prevIssue);
      }
      fail(error);
    },
    onSuccess: (issue) => storeIssue(qc, issue),
    onSettled: () => qc.invalidateQueries({ queryKey: projectKeys.all }),
  });
}

/** 看板拖动。乐观更新，失败回滚。 */
export function useMoveIssue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, plan }: { projectId: number; key: string; plan: MovePlan }) =>
      unwrap(
        projectsApi.POST("/issues/{key}/move", {
          params: { path: { key } },
          body: {
            status: plan.status,
            afterKey: plan.afterKey,
            beforeKey: plan.beforeKey,
          },
        }),
      ),
    onMutate: async ({ projectId, key, plan }) => {
      const listKey = projectKeys.issues(projectId);
      await qc.cancelQueries({ queryKey: listKey });
      const prev = qc.getQueryData<Issue[]>(listKey);
      if (prev) qc.setQueryData(listKey, applyMove(prev, key, plan));
      return { listKey, prev };
    },
    onError: (error, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(ctx.listKey, ctx.prev);
      fail(error);
    },
    onSuccess: (issue) => storeIssue(qc, issue),
    onSettled: () => qc.invalidateQueries({ queryKey: projectKeys.all }),
  });
}

export function useDeleteIssue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (key: string) =>
      unwrap(projectsApi.DELETE("/issues/{key}", { params: { path: { key } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.all }),
    onError: fail,
  });
}

export function useAddComment(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      unwrap(
        projectsApi.POST("/issues/{key}/comments", {
          params: { path: { key } },
          body: { body },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.comments(key) }),
    onError: fail,
  });
}

export function useDeleteComment(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (commentId: number) =>
      unwrap(
        projectsApi.DELETE("/issues/{key}/comments/{commentId}", {
          params: { path: { key, commentId } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.comments(key) }),
    onError: fail,
  });
}

export function useAddLink(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components["schemas"]["CreateIssueLink"]) =>
      unwrap(
        projectsApi.POST("/issues/{key}/links", { params: { path: { key } }, body }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.links(key) }),
    onError: fail,
  });
}

export function useDeleteLink(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (linkId: number) =>
      unwrap(
        projectsApi.DELETE("/issues/{key}/links/{linkId}", {
          params: { path: { key, linkId } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: projectKeys.links(key) }),
    onError: fail,
  });
}

export function useLabelMutations(projectId: number) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: projectKeys.all });
  const create = useMutation({
    mutationFn: (body: components["schemas"]["CreateLabel"]) =>
      unwrap(
        projectsApi.POST("/projects/{projectId}/labels", {
          params: { path: { projectId } },
          body,
        }),
      ),
    onSuccess: done,
    onError: fail,
  });
  const update = useMutation({
    mutationFn: ({ id, body }: { id: number; body: components["schemas"]["UpdateLabel"] }) =>
      unwrap(
        projectsApi.PATCH("/projects/{projectId}/labels/{labelId}", {
          params: { path: { projectId, labelId: id } },
          body,
        }),
      ),
    onSuccess: done,
    onError: fail,
  });
  const remove = useMutation({
    mutationFn: (id: number) =>
      unwrap(
        projectsApi.DELETE("/projects/{projectId}/labels/{labelId}", {
          params: { path: { projectId, labelId: id } },
        }),
      ),
    onSuccess: done,
    onError: fail,
  });
  return { create, update, remove };
}

export function useMilestoneMutations(projectId: number) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: projectKeys.all });
  const create = useMutation({
    mutationFn: (body: components["schemas"]["CreateMilestone"]) =>
      unwrap(
        projectsApi.POST("/projects/{projectId}/milestones", {
          params: { path: { projectId } },
          body,
        }),
      ),
    onSuccess: done,
    onError: fail,
  });
  const update = useMutation({
    mutationFn: ({ id, body }: { id: number; body: components["schemas"]["UpdateMilestone"] }) =>
      unwrap(
        projectsApi.PATCH("/projects/{projectId}/milestones/{milestoneId}", {
          params: { path: { projectId, milestoneId: id } },
          body,
        }),
      ),
    onSuccess: done,
    onError: fail,
  });
  const remove = useMutation({
    mutationFn: (id: number) =>
      unwrap(
        projectsApi.DELETE("/projects/{projectId}/milestones/{milestoneId}", {
          params: { path: { projectId, milestoneId: id } },
        }),
      ),
    onSuccess: done,
    onError: fail,
  });
  return { create, update, remove };
}
