import { useCallback } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import {
  invalidateOn,
  useServerEvent,
  type ServerEvent,
} from "../../api/events";
import type { components, paths } from "../../api/gen/coding";
import {
  hasGap,
  mergeEvents,
  type Task,
  type TaskEvent,
  type TaskStatus,
} from "./logic";

export const codingApi = createApi<paths>();

type S = components["schemas"];
export type Repo = S["Repo"];
export type Executor = S["Executor"];
export type ExecutorName = S["ExecutorName"];
export type DiscoverResult = S["DiscoverResult"];
export type DiscoveredRepo = S["DiscoveredRepo"];
export type TaskDiff = S["TaskDiff"];
export type CreateTask = S["CreateTask"];
export type CodingSettings = S["CodingSettings"];
export type UpdateCodingSettings = S["UpdateCodingSettings"];
export type { Task, TaskEvent, TaskStatus };

export const codingKeys = {
  all: ["coding"] as const,
  tasks: ["coding", "tasks"] as const,
  taskList: (statuses?: TaskStatus[]) =>
    ["coding", "tasks", statuses ?? "all"] as const,
  tasksDetail: ["coding", "task"] as const,
  task: (id: number) => ["coding", "task", id] as const,
  events: (id: number) => ["coding", "events", id] as const,
  diff: (id: number, status: string) => ["coding", "diff", id, status] as const,
  repos: ["coding", "repos"] as const,
  executors: (agentId: string) => ["coding", "executors", agentId] as const,
  discover: (agentId: string, root: string) =>
    ["coding", "discover", agentId, root] as const,
  settings: ["coding", "settings"] as const,
};

// coding_task.output 每 200 毫秒一次，只直接写进输出缓存，不触发整体刷新。
invalidateOn("coding_task.created", codingKeys.tasks);
invalidateOn("coding_task.updated", codingKeys.tasks);
invalidateOn("coding_task.updated", codingKeys.tasksDetail);
invalidateOn("coding_repo.", codingKeys.repos);
invalidateOn("coding_repo.", ["coding", "discover"]);
invalidateOn("coding_settings.", codingKeys.settings);
invalidateOn("agent.", codingKeys.repos);

export function useTasks(statuses?: TaskStatus[]) {
  return useQuery({
    queryKey: codingKeys.taskList(statuses),
    queryFn: async () =>
      (
        await unwrap(
          codingApi.GET("/coding/tasks", {
            params: { query: { status: statuses, limit: 500 } },
          }),
        )
      ).items,
  });
}

export function useTask(id: number) {
  return useQuery({
    queryKey: codingKeys.task(id),
    queryFn: () =>
      unwrap(
        codingApi.GET("/coding/tasks/{taskId}", {
          params: { path: { taskId: id } },
        }),
      ),
    enabled: id > 0,
  });
}

/** 一次取完一个任务的全部输出。 */
async function fetchEvents(id: number): Promise<TaskEvent[]> {
  let all: TaskEvent[] = [];
  let after = 0;
  for (;;) {
    const page = await unwrap(
      codingApi.GET("/coding/tasks/{taskId}/events", {
        params: { path: { taskId: id }, query: { after, limit: 5000 } },
      }),
    );
    all = mergeEvents(all, page.items);
    if (page.items.length < 5000) return all;
    after = page.lastSeq;
  }
}

interface OutputPayload {
  taskId: number;
  events: TaskEvent[];
}

/**
 * 实时输出：先取历史，之后收到 coding_task.output 就追加到缓存。
 * 发现缺口（断线漏了批次）时重新取一次。
 */
export function useTaskEvents(id: number) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: codingKeys.events(id),
    queryFn: () => fetchEvents(id),
    enabled: id > 0,
    staleTime: Infinity,
  });
  const onOutput = useCallback(
    (event: ServerEvent) => {
      const payload = event.data as OutputPayload;
      if (!payload || payload.taskId !== id || !Array.isArray(payload.events))
        return;
      applyOutput(qc, id, payload.events);
    },
    [id, qc],
  );
  useServerEvent("coding_task.output", onOutput);
  return query;
}

export function applyOutput(qc: QueryClient, id: number, events: TaskEvent[]) {
  const key = codingKeys.events(id);
  const current = qc.getQueryData<TaskEvent[]>(key);
  if (!current) return; // 历史还没取到，取到时会包含这些事件
  if (hasGap(current, events)) {
    qc.invalidateQueries({ queryKey: key });
    return;
  }
  qc.setQueryData<TaskEvent[]>(key, mergeEvents(current, events));
}

export function useTaskDiff(
  id: number,
  status: TaskStatus | undefined,
  enabled: boolean,
) {
  return useQuery({
    queryKey: codingKeys.diff(id, status ?? ""),
    queryFn: () =>
      unwrap(
        codingApi.GET("/coding/tasks/{taskId}/diff", {
          params: { path: { taskId: id } },
        }),
      ),
    enabled,
    retry: false,
    staleTime: 60_000,
  });
}

export function useRepos() {
  return useQuery({
    queryKey: codingKeys.repos,
    queryFn: () => unwrap(codingApi.GET("/coding/repos")),
  });
}

export function useExecutors(agentId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: codingKeys.executors(agentId ?? ""),
    queryFn: () =>
      unwrap(
        codingApi.GET("/coding/executors", {
          params: { query: { agentId: agentId! } },
        }),
      ),
    enabled: !!agentId && enabled,
    retry: false,
    staleTime: 5 * 60_000,
  });
}

export function useDiscover(agentId: string, root: string, enabled: boolean) {
  return useQuery({
    queryKey: codingKeys.discover(agentId, root),
    queryFn: () =>
      unwrap(
        codingApi.GET("/coding/repos/discover", {
          params: { query: { agentId, root: root || undefined } },
        }),
      ),
    enabled: enabled && !!agentId,
    retry: false,
    staleTime: 60_000,
  });
}

export function useCodingSettings() {
  return useQuery({
    queryKey: codingKeys.settings,
    queryFn: () => unwrap(codingApi.GET("/coding/settings")),
  });
}

/* ---- 修改 ---- */

function useTaskUpdate() {
  const qc = useQueryClient();
  return (task: Task) => {
    qc.setQueryData(codingKeys.task(task.id), task);
    qc.invalidateQueries({ queryKey: codingKeys.tasks });
  };
}

export function useCreateTask() {
  const update = useTaskUpdate();
  return useMutation({
    mutationFn: (body: CreateTask) =>
      unwrap(codingApi.POST("/coding/tasks", { body })),
    onSuccess: update,
  });
}

export function useCancelTask() {
  const update = useTaskUpdate();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        codingApi.POST("/coding/tasks/{taskId}/cancel", {
          params: { path: { taskId: id } },
        }),
      ),
    onSuccess: update,
  });
}

export function useCommitTask() {
  const update = useTaskUpdate();
  return useMutation({
    mutationFn: ({
      id,
      message,
      push,
    }: {
      id: number;
      message?: string;
      push?: boolean;
    }) =>
      unwrap(
        codingApi.POST("/coding/tasks/{taskId}/commit", {
          params: { path: { taskId: id } },
          body: { message: message || undefined, push },
        }),
      ),
    onSuccess: update,
  });
}

export function usePushTask() {
  const update = useTaskUpdate();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        codingApi.POST("/coding/tasks/{taskId}/push", {
          params: { path: { taskId: id } },
        }),
      ),
    onSuccess: update,
  });
}

export function useOpenPR() {
  const update = useTaskUpdate();
  return useMutation({
    mutationFn: ({
      id,
      title,
      draft,
    }: {
      id: number;
      title?: string;
      draft?: boolean;
    }) =>
      unwrap(
        codingApi.POST("/coding/tasks/{taskId}/pr", {
          params: { path: { taskId: id } },
          body: { title: title || undefined, draft },
        }),
      ),
    onSuccess: update,
  });
}

export function useDiscardTask() {
  const update = useTaskUpdate();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        codingApi.POST("/coding/tasks/{taskId}/discard", {
          params: { path: { taskId: id } },
        }),
      ),
    onSuccess: update,
  });
}

export function useCreateRepo() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { agentId: string; path: string }) =>
      unwrap(codingApi.POST("/coding/repos", { body })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: codingKeys.repos });
      qc.invalidateQueries({ queryKey: ["coding", "discover"] });
    },
  });
}

export function useDeleteRepo() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        codingApi.DELETE("/coding/repos/{repoId}", {
          params: { path: { repoId: id } },
        }),
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: codingKeys.repos });
      qc.invalidateQueries({ queryKey: ["coding", "discover"] });
    },
  });
}

export function useUpdateCodingSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: UpdateCodingSettings) =>
      unwrap(codingApi.PUT("/coding/settings", { body })),
    onSuccess: (data) => qc.setQueryData(codingKeys.settings, data),
  });
}
