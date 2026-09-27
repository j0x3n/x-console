import { fileUrl, pdfFile, setFileUrl, svgImage, textFile } from "./files";
import { demoFull } from "./mode";
import { ago, fail, json, noContent, now, route as addRoute, type DemoHandler } from "./router";
import { vault } from "./vault";

function route(method: string, path: string, handler: DemoHandler) {
  addRoute(method, path, (req) => (demoFull ? handler(req) : undefined));
}

/* 云盘：整个目录树在内存里，刷新页面就回到初始状态。 */
interface Node {
  id: number;
  parentId?: number;
  name: string;
  isDir: boolean;
  size: number;
  mime?: string;
  hidden: boolean;
  /** 放进隐藏空间前所在的文件夹，0 是根目录。只有隐藏空间最上层的条目有。 */
  hiddenFrom?: number;
  trashedAt?: string;
  syncState: "synced" | "pending" | "failed" | "off";
  syncError?: string;
  createdAt: string;
  updatedAt: string;
}

let nextId = 100;
const nodes: Node[] = [];

function add(n: Partial<Node> & { name: string }, url?: string): Node {
  const node: Node = {
    id: nextId++,
    isDir: false,
    size: 0,
    hidden: false,
    syncState: "synced",
    createdAt: n.updatedAt ?? now(),
    updatedAt: now(),
    ...n,
  };
  nodes.push(node);
  if (url) setFileUrl(node.id, url);
  return node;
}

const docs = add({
  name: "项目资料",
  isDir: true,
  syncState: "off",
  updatedAt: ago(60 * 3),
});
const photos = add({
  name: "照片",
  isDir: true,
  syncState: "off",
  updatedAt: ago(60 * 26),
});
const contracts = add({
  name: "合同",
  isDir: true,
  hidden: true,
  syncState: "off",
  updatedAt: ago(60 * 24 * 9),
});
add(
  {
    name: "2026 年度计划.md",
    size: 4312,
    mime: "text/markdown",
    updatedAt: ago(40),
  },
  textFile(
    "# 2026 年度计划\n\n## 工作\n- 上线 X Console 2.0\n- 把家里的服务器迁到新机房\n\n## 生活\n- 每周跑步三次\n- 读完 12 本书\n",
  ),
);
add(
  {
    name: "发票-9月.pdf",
    size: 182_311,
    mime: "application/pdf",
    updatedAt: ago(60 * 20),
  },
  pdfFile("Invoice 2026-09"),
);
add({
  name: "服务器备份-0926.tar.gz",
  size: 1_283_457_024,
  mime: "application/gzip",
  syncState: "failed",
  syncError: "上传到 S3 超时",
  updatedAt: ago(60 * 30),
});
add(
  {
    name: "海边.jpg",
    size: 2_431_998,
    mime: "image/jpeg",
    syncState: "pending",
    updatedAt: ago(15),
  },
  svgImage("海边", "#2b8fd6", "#8fd3f4"),
);
add(
  {
    parentId: docs.id,
    name: "需求说明.txt",
    size: 1840,
    mime: "text/plain",
    updatedAt: ago(60 * 5),
  },
  textFile("云盘需求：\n1. 上传下载\n2. 隐藏文件\n3. 同步到 S3\n"),
);
add(
  {
    parentId: docs.id,
    name: "架构图.png",
    size: 512_004,
    mime: "image/png",
    updatedAt: ago(60 * 6),
  },
  svgImage("架构图", "#5b4bd6", "#b69cf5"),
);
add(
  {
    parentId: docs.id,
    name: "config.yaml",
    size: 623,
    mime: "application/yaml",
    updatedAt: ago(60 * 7),
  },
  textFile(
    "server:\n  addr: 0.0.0.0:8080\n  data: /data\ns3:\n  bucket: backup\n",
  ),
);
add(
  {
    parentId: photos.id,
    name: "山顶日出.jpg",
    size: 3_120_555,
    mime: "image/jpeg",
    updatedAt: ago(60 * 48),
  },
  svgImage("山顶日出", "#f08a4b", "#f7d060"),
);
add(
  {
    parentId: photos.id,
    name: "猫.jpg",
    size: 1_002_331,
    mime: "image/jpeg",
    updatedAt: ago(60 * 50),
  },
  svgImage("猫", "#6b7280", "#d1d5db"),
);
add(
  {
    parentId: photos.id,
    name: "樱花.jpg",
    size: 2_203_870,
    mime: "image/jpeg",
    updatedAt: ago(60 * 72),
  },
  svgImage("樱花", "#e86a9a", "#f9c6d8"),
);
add(
  {
    parentId: contracts.id,
    name: "租房合同.pdf",
    size: 402_113,
    mime: "application/pdf",
    syncState: "off",
    updatedAt: ago(60 * 24 * 9),
  },
  pdfFile("Lease 2026"),
);
add(
  {
    name: "旧的截图.png",
    size: 88_122,
    mime: "image/png",
    trashedAt: ago(60 * 24 * 3),
    updatedAt: ago(60 * 24 * 3),
  },
  svgImage("旧截图", "#374151", "#9ca3af"),
);

const byId = (id: number) => nodes.find((n) => n.id === id);
const isHidden = (n: Node): boolean => {
  let cur: Node | undefined = n;
  while (cur) {
    if (cur.hidden) return true;
    cur = cur.parentId ? byId(cur.parentId) : undefined;
  }
  return false;
};
const inTrash = (n: Node): boolean => {
  let cur: Node | undefined = n;
  while (cur) {
    if (cur.trashedAt) return true;
    cur = cur.parentId ? byId(cur.parentId) : undefined;
  }
  return false;
};
const visible = (n: Node) => !isHidden(n) || vault.unlocked;
const view = (n: Node) => {
  const { hiddenFrom, ...rest } = n;
  if (!n.hidden || n.parentId) return rest;
  const from = hiddenFrom ? byId(hiddenFrom) : undefined;
  const restoreTo = from
    ? "/" + [...path(from).map((p) => p.name), from.name].join("/")
    : "/";
  return { ...rest, restoreTo };
};
const path = (n: Node) => {
  const out: { id: number; name: string }[] = [];
  let cur = n.parentId ? byId(n.parentId) : undefined;
  while (cur) {
    out.unshift({ id: cur.id, name: cur.name });
    cur = cur.parentId ? byId(cur.parentId) : undefined;
  }
  return out;
};
const children = (id: number): Node[] =>
  nodes.filter((n) => n.parentId === id).flatMap((n) => [n, ...children(n.id)]);
const uniqueName = (
  parentId: number | undefined,
  name: string,
  skip?: number,
) => {
  const taken = new Set(
    nodes
      .filter((n) => n.parentId === parentId && !n.trashedAt && n.id !== skip)
      .map((n) => n.name),
  );
  if (!taken.has(name)) return name;
  const dot = name.lastIndexOf(".");
  const [base, ext] =
    dot > 0 ? [name.slice(0, dot), name.slice(dot)] : [name, ""];
  for (let i = 1; ; i++)
    if (!taken.has(`${base} (${i})${ext}`)) return `${base} (${i})${ext}`;
};

const s3 = {
  endpoint: "https://demo.r2.cloudflarestorage.com",
  region: "auto",
  bucket: "x-console-backup",
  prefix: "x-console",
  accessKeyId: "AKIADEMO",
  hasSecret: true,
  pathStyle: false,
  enabled: true,
  includeHidden: false,
};
let lastRunAt = ago(8);

route("GET", "/drive/items", ({ query }) => {
  const hiddenView = query.get("hidden") === "true";
  if (hiddenView && !vault.unlocked) return json({ items: [] });
  const q = (query.get("q") ?? "").trim().toLowerCase();
  let items: Node[];
  if (query.get("trashed") === "true") {
    items = nodes.filter(
      (n) =>
        n.trashedAt &&
        visible(n) &&
        !(n.parentId && inTrash(byId(n.parentId)!)),
    );
  } else if (q) {
    items = nodes.filter(
      (n) =>
        !inTrash(n) &&
        n.name.toLowerCase().includes(q) &&
        (hiddenView ? isHidden(n) : !isHidden(n)),
    );
  } else {
    const parent = query.get("parent")
      ? Number(query.get("parent"))
      : undefined;
    items = nodes.filter(
      (n) =>
        n.parentId === parent &&
        !n.trashedAt &&
        (hiddenView ? isHidden(n) : !isHidden(n)),
    );
  }
  return json({ items: items.map(view) });
});
route("GET", "/drive/items/:id", ({ params }) => {
  const n = byId(Number(params.id));
  if (!n || !visible(n)) return fail(404, "not_found", "资源不存在");
  return json({ ...view(n), path: path(n) });
});
route("POST", "/drive/folders", ({ body }) => {
  const parentId = body?.parentId || undefined;
  if (
    nodes.some(
      (n) => n.parentId === parentId && !n.trashedAt && n.name === body.name,
    )
  )
    return fail(409, "conflict", "这里已经有同名的文件夹");
  const n = add({
    parentId,
    name: body.name,
    isDir: true,
    hidden: !!body.hidden,
    syncState: "off",
  });
  return json(view(n), 201);
});
route("PATCH", "/drive/items/:id", ({ params, body }) => {
  const n = byId(Number(params.id));
  if (!n || !visible(n)) return fail(404, "not_found", "资源不存在");
  if (body?.name !== undefined) {
    if (
      nodes.some(
        (x) =>
          x.parentId === n.parentId &&
          x.id !== n.id &&
          !x.trashedAt &&
          x.name === body.name,
      )
    )
      return fail(409, "conflict", "这里已经有同名的文件");
    n.name = body.name;
  }
  if (body?.parentId !== undefined) {
    const target = body.parentId || undefined;
    if (
      target === n.id ||
      (target && children(n.id).some((c) => c.id === target))
    )
      return fail(400, "bad_request", "不能移到自己里面");
    n.parentId = target;
    n.name = uniqueName(target, n.name, n.id);
  }
  if (body?.hidden !== undefined && body.hidden !== isHidden(n)) {
    if (!vault.unlocked) return fail(403, "vault_locked", "先解锁隐藏内容");
    if (body.hidden) {
      // 放进隐藏空间最上层，记住原来的文件夹
      n.hiddenFrom = n.parentId ?? 0;
      n.parentId = undefined;
      n.hidden = true;
    } else {
      // 放回原来的文件夹；原文件夹没了、进了回收站或也被隐藏了，就放到根目录
      const from = n.hiddenFrom ? byId(n.hiddenFrom) : undefined;
      const ok = from && !inTrash(from) && !isHidden(from);
      n.hidden = false;
      n.parentId = ok ? from.id : undefined;
      n.hiddenFrom = undefined;
      for (const c of children(n.id)) c.hidden = false;
    }
    n.name = uniqueName(n.parentId, n.name, n.id);
  }
  n.updatedAt = now();
  if (!n.isDir && s3.enabled) n.syncState = "pending";
  return json(view(n));
});
route("DELETE", "/drive/items/:id", ({ params, query }) => {
  const n = byId(Number(params.id));
  if (!n) return fail(404, "not_found", "资源不存在");
  if (query.get("permanent") === "true") {
    for (const x of [n, ...children(n.id)]) nodes.splice(nodes.indexOf(x), 1);
  } else n.trashedAt = now();
  return noContent();
});
route("POST", "/drive/items/:id/restore", ({ params }) => {
  const n = byId(Number(params.id));
  if (!n) return fail(404, "not_found", "资源不存在");
  n.trashedAt = undefined;
  if (n.parentId && !byId(n.parentId)) n.parentId = undefined;
  n.name = uniqueName(n.parentId, n.name, n.id);
  return json(view(n));
});
route("GET", "/drive/items/:id/content", ({ params }) => {
  const url = fileUrl(Number(params.id));
  if (!url)
    return new Response("这是演示文件，没有内容。", {
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  return fetch(url);
});
route("GET", "/drive/usage", () => {
  const live = nodes.filter((n) => !n.isDir && !isHidden(n));
  return json({
    files: live.filter((n) => !inTrash(n)).length,
    bytes: live.filter((n) => !inTrash(n)).reduce((s, n) => s + n.size, 0),
    trashBytes: live.filter((n) => inTrash(n)).reduce((s, n) => s + n.size, 0),
  });
});
route("GET", "/drive/s3", () => json(s3));
route("PUT", "/drive/s3", ({ body }) => {
  const { secretAccessKey, ...rest } = body ?? {};
  Object.assign(s3, rest);
  if (secretAccessKey) s3.hasSecret = true;
  return json(s3);
});
route("POST", "/drive/s3/test", () =>
  json({ ok: true, message: "连接成功：写入并删除了测试文件（演示）" }),
);
route("POST", "/drive/s3/sync", () => {
  for (const n of nodes)
    if (!n.isDir && n.syncState === "pending") n.syncState = "synced";
  lastRunAt = now();
  return noContent(202);
});
route("GET", "/drive/s3/status", () => {
  const files = nodes.filter((n) => !n.isDir);
  const failed = files.filter((n) => n.syncState === "failed");
  return json({
    state: !s3.enabled ? "off" : failed.length ? "failed" : "idle",
    lastRunAt,
    lastError: failed[0]?.syncError,
    pending: files.filter((n) => n.syncState === "pending").length,
    synced: files.filter((n) => n.syncState === "synced").length,
    failed: failed.length,
  });
});

/** 上传：由 index.ts 里的假 XMLHttpRequest 调用。 */
export function demoUpload(query: URLSearchParams, file: File) {
  const parentId = query.get("parent")
    ? Number(query.get("parent"))
    : undefined;
  const n = add(
    {
      parentId,
      name: uniqueName(parentId, file.name),
      size: file.size,
      mime: file.type || "application/octet-stream",
      hidden: query.get("hidden") === "true" && !parentId,
      syncState: s3.enabled ? "pending" : "off",
    },
    URL.createObjectURL(file),
  );
  return view(n);
}
