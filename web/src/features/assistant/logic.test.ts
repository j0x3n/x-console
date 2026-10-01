import { describe, expect, it } from "vitest";
import type { Message, PendingAction, Tool } from "./api";
import {
  rememberedText,
  actionName,
  buildTimeline,
  foldLines,
  HOST_TOOLS,
  resultText,
  summarizeInput,
} from "./logic";
import { clampOffset } from "./store";

const tools: Tool[] = [
  {
    name: "notes__search",
    action: "notes.search",
    title: "搜索笔记",
    effect: "read",
  },
  {
    name: "notes__delete",
    action: "notes.delete",
    title: "删除笔记",
    effect: "write",
  },
];
const at = "2026-09-27T08:00:00Z";
const msg = (
  id: number,
  role: "user" | "assistant",
  content: Message["content"],
): Message => ({
  id,
  seq: id,
  role,
  content,
  createdAt: at,
});

describe("buildTimeline", () => {
  const messages = [
    msg(1, "user", [
      { type: "text", text: "当前页面 /notes", context: true },
      { type: "text", text: "删掉关于周报的笔记" },
    ]),
    msg(2, "assistant", [
      { type: "text", text: "我先找一下。" },
      {
        type: "tool_use",
        id: "tu1",
        name: "notes__search",
        input: { q: "周报" },
      },
    ]),
    msg(3, "user", [
      {
        type: "tool_result",
        tool_use_id: "tu1",
        content: [{ type: "text", text: "[{id: 7}]" }],
      },
    ]),
    msg(4, "assistant", [
      { type: "tool_use", id: "tu2", name: "notes__delete", input: { id: 7 } },
    ]),
  ];
  const pending: PendingAction[] = [
    {
      id: 9,
      conversationId: 1,
      toolUseId: "tu2",
      action: "notes.delete",
      input: { id: 7 },
      status: "pending",
    },
  ];

  it("hides page context and tool results, pairs results with calls", () => {
    const items = buildTimeline(messages, pending, tools, { running: true });
    expect(items.map((i) => i.kind)).toEqual([
      "user",
      "assistant",
      "action",
      "action",
    ]);
    expect(items[0]).toMatchObject({ text: "删掉关于周报的笔记" });
    expect(items[2]).toMatchObject({
      title: "搜索笔记",
      status: "ok",
      result: "[{id: 7}]",
    });
    expect(items[3]).toMatchObject({
      title: "删除笔记",
      status: "waiting",
      effect: "write",
    });
    expect(items[3].kind === "action" && items[3].pending?.id).toBe(9);
  });

  it("shows rejected and failed actions", () => {
    const rejected = buildTimeline(
      messages,
      [{ ...pending[0], status: "rejected" }],
      tools,
      { running: false },
    );
    expect(rejected[3]).toMatchObject({ status: "rejected" });
    const errored = buildTimeline(
      [
        ...messages,
        msg(5, "user", [
          {
            type: "tool_result",
            tool_use_id: "tu2",
            content: "没有这条笔记",
            is_error: true,
          },
        ]),
      ],
      [],
      tools,
      { running: false },
    );
    expect(errored[3]).toMatchObject({
      status: "error",
      result: "没有这条笔记",
    });
  });

  it("appends the streaming text and falls back to the action name", () => {
    const items = buildTimeline(
      [
        msg(1, "assistant", [
          { type: "tool_use", id: "x", name: "hosts__list", input: {} },
        ]),
      ],
      [],
      tools,
      { running: true, streaming: "正在" },
    );
    expect(items[0]).toMatchObject({
      title: "hosts.list",
      effect: "unknown",
      status: "running",
    });
    expect(items[1]).toMatchObject({
      kind: "assistant",
      text: "正在",
      streaming: true,
    });
  });
});

describe("helpers", () => {
  it("maps tool names back to action names", () => {
    expect(actionName("projects__issues__create")).toBe(
      "projects.issues.create",
    );
  });
  it("summarizes input", () => {
    expect(
      summarizeInput({
        title: "周会",
        body: "a".repeat(40),
        tags: ["x"],
        pinned: true,
      }),
    ).toBe(`title: 周会 · body: ${"a".repeat(27)}… · tags: ["x"] · +1`);
    expect(summarizeInput({})).toBe("");
  });
  it("reads tool results in any shape", () => {
    expect(resultText("ok")).toBe("ok");
    expect(resultText({ a: 1 })).toContain('"a": 1');
  });
  it("keeps the panel inside the window", () => {
    const panel = { width: 400, height: 600 };
    const view = { width: 1360, height: 860 };
    expect(clampOffset({ x: -50, y: 5000 }, panel, view)).toEqual({
      x: 0,
      y: 220,
    });
    expect(clampOffset({ x: 100, y: 100 }, panel, view)).toEqual({
      x: 100,
      y: 100,
    });
  });
});

describe("host agent (B33)", () => {
  it("folds results over 200 lines", () => {
    const text = Array.from({ length: 250 }, (_, i) => `line ${i + 1}`).join(
      "\n",
    );
    const folded = foldLines(text);
    const lines = folded.text.split("\n");
    expect(folded.hidden).toBe(50);
    expect(lines).toHaveLength(201);
    expect(lines[0]).toBe("line 1");
    expect(lines[99]).toBe("line 100");
    expect(lines[100]).toBe("…… 省略 50 行 ……");
    expect(lines[200]).toBe("line 250");
    expect(foldLines("a\nb")).toEqual({ text: "a\nb", hidden: 0 });
  });

  it("uses the risk the server judged for this command", () => {
    const items = buildTimeline(
      [
        msg(1, "user", [{ type: "text", text: "清理一下" }]),
        msg(2, "assistant", [
          {
            type: "tool_use",
            id: "t1",
            name: "host__run_command",
            input: { command: "rm -rf /tmp/x", reason: "删掉临时文件" },
          },
        ]),
      ],
      [
        {
          id: 9,
          conversationId: 1,
          toolUseId: "t1",
          action: "host.run_command",
          input: { command: "rm -rf /tmp/x" },
          status: "pending",
          effect: "dangerous",
        },
      ],
      HOST_TOOLS,
      { running: true },
    );
    const action = items.find((i) => i.kind === "action");
    expect(action).toMatchObject({
      action: "host.run_command",
      title: "Run command",
      effect: "dangerous",
      status: "waiting",
    });
  });
});

describe("attachments", () => {
  it("shows images and files of a user message, even without text", () => {
    const items = buildTimeline(
      [
        {
          id: 1,
          seq: 1,
          role: "user",
          createdAt: "",
          content: [
            { type: "text", text: "" },
            {
              type: "image",
              attachmentId: 7,
              name: "图.png",
              mime: "image/png",
            },
            {
              type: "file",
              attachmentId: 8,
              name: "main.go",
              mime: "text/plain",
            },
            { type: "text", text: "当前时间：…", context: true },
          ],
        },
      ],
      [],
      [],
      { running: false },
    );
    expect(items).toEqual([
      {
        kind: "user",
        key: "m1",
        text: "",
        attachments: [
          { id: 7, name: "图.png", kind: "image" },
          { id: 8, name: "main.go", kind: "file" },
        ],
      },
    ]);
  });
});

describe("已记住（B61）", () => {
  const action = (action: string, status: "ok" | "error", text?: string) =>
    ({
      kind: "action",
      key: "a",
      action,
      title: action,
      effect: "write",
      input: text ? { text } : {},
      status,
    }) as const;
  it("shows saved and updated memories that succeeded", () => {
    expect(rememberedText(action("memory.save", "ok", " 周报周五写 "))).toBe(
      "周报周五写",
    );
    expect(rememberedText(action("memory.update", "ok", "改了"))).toBe("改了");
    expect(rememberedText(action("memory.save", "error", "失败的"))).toBeNull();
    expect(rememberedText(action("memory.delete", "ok"))).toBeNull();
    expect(rememberedText(action("notes.create", "ok", "笔记"))).toBeNull();
  });
});
