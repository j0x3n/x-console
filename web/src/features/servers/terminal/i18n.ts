import { registerZh } from "../../../lib/i18n";

// B249 后台终端和顶栏的“后台”按钮
registerZh({
  "Background work": "后台",
  Terminal: "终端",
  Disconnect: "断开",
  Queued: "排队中",
  Running: "运行中",
  "Agent tasks": "Agent 任务",
  "The terminal is still connected": "终端还连着",
  "A terminal left in the background stays connected. Running commands keep running.":
    "留在后台的终端不会断开，正在跑的命令接着跑。",
  "Keep in background": "留在后台",
  "Always do this, don't ask again": "以后都这样，不再问",
  "Disconnected the terminal on {name}. Up to 4 stay in the background.":
    "已断开 {name} 的终端，后台最多留 4 个",
  "The terminal on {name} was idle in the background for 30 minutes and was disconnected.":
    "{name} 的终端在后台 30 分钟没看，已断开",
  "{n} in the background": "后台 {n} 个",
  "Disconnect the terminal on {name}?": "断开 {name} 的终端？",
  "Running commands stop.": "正在跑的命令会停止。",
  "Cancel task {name}?": "取消任务 {name}？",
  "A running task stops and its changes are thrown away.":
    "运行中的任务会停下，改动不保留。",
  "Cancel task": "取消任务",
  Canceled: "已取消",
  "Command running": "命令在跑",
  "Under a minute": "不到 1 分钟",
  "{n} min": "{n} 分钟",
  "{h} h {m} min": "{h} 小时 {m} 分钟",
  // 设置 → 通用 → 终端
  "When leaving a connected terminal": "离开连着的终端时",
  "Applies when you leave the server or computer page while a terminal is connected.":
    "终端连着的时候离开服务器页或电脑页，按这里处理。",
  "Ask every time": "每次都问",
  "Disconnect it": "断开",
});
