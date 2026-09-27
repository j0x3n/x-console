import type { RouteObject } from "react-router";
import { Sparkles } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import "./i18n";
import { useAssistant } from "./store";

// 助手是全局浮窗（见 AssistantPanel），没有自己的页面。
registerCommands([
  {
    id: "assistant.open",
    title: "打开 AI 助手",
    group: "AI 助手",
    keywords: "ai assistant claude chat ⌘J",
    icon: Sparkles,
    run: () => useAssistant.getState().setOpen(true),
  },
]);

export const routes: RouteObject[] = [];
