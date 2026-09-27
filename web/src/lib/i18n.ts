import type { Language, Text } from "../types/domain";

/*
 * 翻译用英文原文做键，中文词典按模块注册。
 * 每个模块在 features/<模块>/i18n.ts 里调用 registerZh，
 * 并在模块路由文件里 import 这个文件。
 */
const zh: Record<string, string> = {};
const conflicts: string[] = [];

/**
 * 注册中文词条。词典是全局共用的：同一个英文键在不同模块里
 * 必须对应同一个中文。意思不同就换一个更具体的英文键。
 */
export function registerZh(dict: Record<string, string>) {
  for (const [key, value] of Object.entries(dict)) {
    if (key in zh && zh[key] !== value) {
      conflicts.push(`${key}: "${zh[key]}" vs "${value}"`);
      if (import.meta.env?.DEV) console.warn(`i18n key conflict: ${key}`);
    }
    zh[key] = value;
  }
}

/** 测试用：注册过程中出现的冲突。 */
export function zhConflicts(): readonly string[] {
  return conflicts;
}

export function translate(language: Language, text: Text): string {
  const value = String(text ?? "");
  return language === "zh" ? (zh[value] ?? value) : value;
}

registerZh({
  Overview: "概览",
  Projects: "项目",
  Notes: "笔记",
  Reminders: "提醒",
  Habits: "习惯",
  Calendar: "日历",
  Servers: "服务器",
  "This PC": "本机",
  "Coding tasks": "编码任务",
  Monitoring: "监控",
  "Smart home": "智能家居",
  Automations: "自动化",
  Settings: "设置",
  Personal: "个人",
  Machines: "设备",
  Integrations: "集成",
  Assistant: "AI 助手",
  GitHub: "GitHub",
  "Coming soon": "即将推出",
  "This module is planned in the roadmap.": "这个模块已经在开发计划里。",
  Search: "搜索",
  "Search or run a command...": "搜索或执行命令...",
  "Go to": "跳转",
  "No results found": "没有找到结果",
  "Open navigation": "打开导航",
  "Close navigation": "关闭导航",
  Main: "主导航",
  Notifications: "通知",
  "Mark all read": "全部已读",
  "No notifications": "没有通知",
  "Open command palette": "打开命令面板",
  Theme: "主题",
  "Follow system": "跟随系统",
  Dark: "深色",
  Light: "浅色",
  Language: "语言",
  "Sign out": "退出登录",
  Cancel: "取消",
  Close: "关闭",
  Confirm: "确认",
  Save: "保存",
  Delete: "删除",
  Edit: "编辑",
  Create: "新建",
  Loading: "加载中",
  Retry: "重试",
  "Something went wrong": "出错了",
  Undo: "撤销",
  Dismiss: "关闭",
  "Verify it's you": "再次验证",
  "Enter the 6-digit code from your authenticator app.":
    "输入验证器 App 里的 6 位验证码。",
  "Verification code": "验证码",
  Verify: "验证",
  Username: "用户名",
  Password: "密码",
  "Sign in": "登录",
  Online: "在线",
  Offline: "离线",
  "Just now": "刚刚",
});
