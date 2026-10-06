import { registerZh } from "../../lib/i18n";

registerZh({
  // B111
  Refresh: "刷新",
  More: "更多",
  Machine: "机器",
  "Move up": "上移",
  "Move down": "下移",
  "AI quotas": "AI 额度",
  "Add account": "添加账号",
  "Add quota account": "添加额度账号",
  "Edit quota account": "修改额度账号",
  "Refresh all": "全部刷新",
  "Delete account": "删除账号",
  "This only removes the record here. The sign-in on the machine is not touched.":
    "只删除这里的记录，不会影响机器上的登录。",
  "No quota accounts yet": "还没有额度账号",
  "Add your Claude, Codex, Grok or DeepSeek accounts to see what is left and when it resets.":
    "添加 Claude、Codex、Grok 或 DeepSeek 账号，就能看到还剩多少、什么时候重置。",
  Accounts: "账号",
  accounts: "个账号",
  "with problems": "个有问题",
  "All readings are fine": "读取都正常",
  "Least left": "剩余最少",
  "No allowance windows": "没有额度窗口",
  "Next reset": "最近重置",
  "No reset times known": "不知道重置时间",
  Left: "剩余",
  Extra: "额外",
  Balance: "余额",
  Credits: "积分",
  "Reading…": "正在读取…",
  Notifications: "通知",
  "Quota notifications": "额度通知",
  "Each window is told once per period. It is told again after the window resets.":
    "每个窗口每个周期只通知一次，重置后再通知。",
  "Running out": "快用完",
  "When a window has 10% or less left.": "窗口剩余 10% 以下时。",
  "Used up": "用完",
  "When a window has nothing left.": "窗口一点都不剩时。",
  "Low DeepSeek balance": "DeepSeek 余额不足",
  "When a balance falls under the limit set on the account.":
    "余额低于账号上设的数时。",
  "Reading keeps failing": "连续读取失败",
  "After 3 failed readings in a row (2 for Claude).":
    "连续 3 次读取失败后（Claude 是 2 次）。",
  "Notify when below": "余额低于这个数时通知",
  "Empty means no notification": "留空表示不通知",
  "Compared with the first currency of the balance.":
    "和余额里的第一个币种比。",
  "Last read": "上次读取",
  "These numbers are from the last successful reading.":
    "显示的是上一次读取成功的数字。",
  "Machine offline": "机器离线",
  "Sign-in expired": "登录失效",
  "Read failed": "读取失败",
  "Agent too old": "代理版本过旧",
  "Machine removed": "机器已移除",
  Service: "服务",
  "Account name": "备注名",
  "For example: personal, work": "比如：个人、公司",
  "DeepSeek API key": "DeepSeek API Key",
  "Leave empty to keep the current key": "留空表示不改",
  "The key is stored encrypted and never shown again.":
    "Key 加密保存，之后不会再显示。",
  "Choose a machine": "选择机器",
  "No machine can read quotas. Install Claude Code, Codex or Grok on a machine with an agent, and update the agent.":
    "没有能读额度的机器。请在装了代理的机器上安装 Claude Code、Codex 或 Grok，并升级代理。",
  "Sign-in directory": "登录目录",
  "Leave empty for the default directory. Use an absolute path, or one that starts with ~/.":
    "留空表示默认目录。要写绝对路径，或以 ~/ 开头。",
  "To sign in a second account in its own directory, run:":
    "要在单独的目录里登录第二个账号，运行：",
});
