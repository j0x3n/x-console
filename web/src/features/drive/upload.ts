import { create } from "zustand";
import { API_BASE } from "../../api/client";

/*
 * 上传队列。每个文件单独一个请求，用 XMLHttpRequest 拿到进度。
 * 同时最多传 3 个，其余排队。
 */
export type UploadState = "queued" | "uploading" | "done" | "failed";

export interface UploadTask {
  id: number;
  name: string;
  size: number;
  loaded: number;
  state: UploadState;
  error?: string;
}

type Target =
  | {
      parent: number | null;
      hidden: boolean;
    }
  | { remoteId: number; parent: string };

interface UploadStore {
  tasks: UploadTask[];
  add: (files: File[], target: Target, onDone: () => void) => void;
  clearFinished: () => void;
}

const MAX_PARALLEL = 3;
let nextId = 1;
const pending = new Map<
  number,
  { file: File; target: Target; onDone: () => void }
>();

export const useUploads = create<UploadStore>()((set, get) => {
  const patch = (id: number, change: Partial<UploadTask>) =>
    set({
      tasks: get().tasks.map((t) => (t.id === id ? { ...t, ...change } : t)),
    });

  const pump = () => {
    const running = get().tasks.filter((t) => t.state === "uploading").length;
    const next = get()
      .tasks.filter((t) => t.state === "queued")
      .slice(0, MAX_PARALLEL - running);
    for (const task of next) {
      const job = pending.get(task.id);
      if (!job) continue;
      pending.delete(task.id);
      patch(task.id, { state: "uploading" });
      send(job.file, job.target, (loaded) => patch(task.id, { loaded }))
        .then(() => {
          patch(task.id, { state: "done", loaded: task.size });
          job.onDone();
        })
        .catch((error: Error) =>
          patch(task.id, { state: "failed", error: error.message }),
        )
        .finally(pump);
    }
  };

  return {
    tasks: [],
    add: (files, target, onDone) => {
      const added = files.map((file) => {
        const id = nextId++;
        pending.set(id, { file, target, onDone });
        return {
          id,
          name: file.name,
          size: file.size,
          loaded: 0,
          state: "queued" as const,
        };
      });
      set({ tasks: [...get().tasks, ...added] });
      pump();
    },
    clearFinished: () =>
      set({
        tasks: get().tasks.filter(
          (t) => t.state === "queued" || t.state === "uploading",
        ),
      }),
  };
});

export function uploadUrl(target: Target, name = "") {
  if ("remoteId" in target) {
    const query = new URLSearchParams({ parent: target.parent, name });
    return `${API_BASE}/storage/remotes/${target.remoteId}/content?${query}`;
  }
  const q = new URLSearchParams();
  if (target.parent != null) q.set("parent", String(target.parent));
  if (target.hidden) q.set("hidden", "true");
  const s = q.toString();
  return `${API_BASE}/drive/upload${s ? `?${s}` : ""}`;
}

function send(
  file: File,
  target: Target,
  onProgress: (loaded: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const remote = "remoteId" in target;
    xhr.open(remote ? "PUT" : "POST", uploadUrl(target, file.name));
    xhr.withCredentials = true;
    xhr.setRequestHeader("X-Requested-With", "x-console");
    xhr.upload.onprogress = (e) => onProgress(Math.min(file.size, e.loaded));
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) return resolve();
      let message = xhr.statusText || `HTTP ${xhr.status}`;
      try {
        message = JSON.parse(xhr.responseText).message ?? message;
      } catch {
        /* 非 JSON 错误体 */
      }
      reject(new Error(message));
    };
    xhr.onerror = () => reject(new Error("网络错误，上传失败"));
    if (remote) {
      xhr.setRequestHeader("Content-Type", "application/octet-stream");
      xhr.send(file);
    } else {
      const form = new FormData();
      form.append("file", file, file.name);
      xhr.send(form);
    }
  });
}
