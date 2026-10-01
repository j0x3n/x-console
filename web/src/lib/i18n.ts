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
  "My day": "今日",
  Projects: "项目",
  Notes: "笔记",
  Drive: "云盘",
  Reminders: "提醒",
  Habits: "习惯",
  Calendar: "日历",
  Servers: "服务器",
  Computer: "电脑",
  "Coding tasks": "Agent 任务",
  Agents: "Agent",
  "Not found": "页面不存在",
  "Live updates disconnected. Reconnecting.": "实时连接断开了，正在重连",
  Monitoring: "监控",
  "Smart home": "智能家居",
  Automations: "自动化",
  Settings: "设置",
  Personal: "个人",
  Machines: "设备",
  Integrations: "集成",
  Assistant: "AI",
  GitHub: "GitHub",
  "Coming soon": "即将推出",
  "This module is planned in the roadmap.": "这个模块已经在开发计划里。",
  Search: "搜索",
  "Search or run a command...": "搜索或执行命令...",
  "Go to": "跳转",
  "No results found": "没有找到结果",
  "Open navigation": "打开导航",
  "Collapse sidebar": "折叠侧边栏",
  "Expand sidebar": "展开侧边栏",
  "Close navigation": "关闭导航",
  Main: "主导航",
  Notifications: "通知",
  "Mark all read": "全部已读",
  "No notifications": "没有通知",
  "Open command palette": "打开命令面板",
  Theme: "主题",
  "Follow system": "跟随系统",
  "Night mode": "夜间模式",
  On: "开",
  Off: "关",
  Auto: "自动",
  "Theme color": "主题色",
  "Theme colors apply in the daytime.": "主题色在白天生效。",
  Ember: "暖橙",
  Violet: "紫罗兰",
  Mint: "薄荷绿",
  Ocean: "海蓝",
  Rose: "玫瑰",
  Graphite: "石墨",
  Dark: "深色",
  Light: "浅色",
  Language: "语言",
  "Sign out": "退出登录",
  Cancel: "取消",
  Close: "关闭",
  Confirm: "确认",
  Save: "保存",
  Delete: "删除",
  "Type this to confirm:": "输入这几个字确认：",
  "Search the log": "搜索日志",
  Level: "级别",
  "All levels": "全部级别",
  "Errors only": "只看错误",
  "Warnings and above": "警告及以上",
  "Info and above": "信息及以上",
  "Debug and above": "调试及以上",
  Output: "输出",
  "All output": "全部输出",
  "Standard output": "标准输出",
  "Error output": "错误输出",
  lines: "行",
  "Loading earlier lines": "正在加载更早的日志",
  "No log lines": "没有日志",
  "new lines, back to bottom": "条新日志，回到底部",
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
  "Enter your login password.": "输入登录密码。",
  "Verification code": "验证码",
  Verify: "验证",
  Username: "用户名",
  Password: "密码",
  "Sign in": "登录",
  Online: "在线",
  Offline: "离线",
  "Just now": "刚刚",
  "type the text after the prefix": "在前缀后面输入内容",
  "See all": "全部",
  "Expand menu": "展开",
  "Collapse menu": "收起",
  "Open settings": "打开设置",
  // Markdown 编辑框贴图（B36）
  "Insert image": "插入图片",
  "Uploading…": "上传中…",
  "Only images can be added here": "这里只能加图片",
  "Image is larger than 20 MB": "图片超过 20 MB",
  "Image upload is not live yet": "图片上传还没上线",
});

// B41 报错提示
registerZh({
  "Request failed": "请求失败",
  Copy: "复制",
  Copied: "已复制",
  "Page error": "页面出错",
  "Show more": "展开",
  "Show less": "收起",
  "{n} more errors": "还有 {n} 条报错",
  "Dismiss all": "全部关闭",
  "Recent errors": "最近的报错",
  "Copy all": "全部复制",
  "No errors since this page was opened": "打开页面以来没有报错",
  "Something went wrong on this page": "这个页面出错了",
  Reload: "重新加载",
});
