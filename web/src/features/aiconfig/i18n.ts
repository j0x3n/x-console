import { registerZh } from "../../lib/i18n";

registerZh({
  // B121
  // 和别的模块相同的词，这个模块单独加载时也要有
  Add: "添加",
  Address: "地址",
  Cancel: "取消",
  Command: "命令",
  Name: "名称",
  Offline: "离线",
  Online: "在线",
  Remove: "移除",
  Rules: "规则",
  Save: "保存",
  Saved: "已保存",
  "Config delivery": "配置下发",
  "Keep one Claude Code and Codex configuration here and write it to your machines. Only what you write here is managed. Your own settings on the machines are not read or changed.":
    "在这里维护一份 Claude Code 和 Codex 的配置，再写到你的机器上。只管这里写的内容，机器上你自己的设置不会被读取，也不会被改。",
  "Deliver to machines": "下发到哪些机器",
  "Check again": "重新检查",
  "Deliver to all": "全部下发",
  Deliver: "下发",
  "Deliver to {n} machines?": "下发到 {n} 台机器？",
  "Deliver to this machine?": "下发到这台机器？",
  "This changes the Claude Code and Codex files on the selected machines. Your own content in them stays. A copy of each file is kept the first time it is changed.":
    "会修改所选机器上 Claude Code 和 Codex 的配置文件。文件里你自己的内容保留，每个文件第一次修改前会留一份备份。",
  Delivered: "已下发",
  "Delivered, but some items failed": "已下发，但有几项没写成功",
  "Nothing to deliver": "没有要下发的",
  "Save the changes first": "先保存修改再下发",
  "You have unsaved changes": "有没保存的修改",
  "No machine is paired yet": "还没有配对的机器",
  "Pair a server or a computer in Settings, then come back here.":
    "先在设置里配对服务器或电脑，再回到这里。",
  "Pick the machines to deliver to.": "选一选要下发到哪些机器。",
  Checking: "检查中",
  "Check failed": "检查失败",
  "In sync": "一致",
  "Out of sync": "不一致",
  Conflict: "冲突",
  "Not installed": "没安装",
  "Not supported": "不支持",
  "Needs Claude Code or Codex on the machine, and an up-to-date agent.":
    "机器上要装 Claude Code 或 Codex，代理也要是新版。",
  "Tool permissions": "权限",
  "MCP servers": "MCP 服务器",
  "Global rules": "全局规则",
  "Written to CLAUDE.md in the Claude Code folder, between markers. Text outside the markers is not touched.":
    "写进 Claude Code 目录里的 CLAUDE.md，放在标记之间。标记以外的内容不动。",
  "Written to AGENTS.md in the Codex folder, between markers. Text outside the markers is not touched.":
    "写进 Codex 目录里的 AGENTS.md，放在标记之间。标记以外的内容不动。",
  Allow: "允许",
  "Ask first": "询问",
  Deny: "禁止",
  "One rule per line, for example Bash(git status). Added to the lists in settings.json. Rules you wrote there yourself are kept.":
    "每行一条，比如 Bash(git status)。会加进 settings.json 的对应列表，你自己写在里面的条目保留。",
  "Add server": "添加服务器",
  "No servers.": "没有服务器。",
  "Server name": "服务器名称",
  Transport: "传输方式",
  "Local process": "本机进程",
  "Remote (HTTP)": "远程（HTTP）",
  Arguments: "参数",
  "One argument per line": "每行一个参数",
  "Environment variables and headers are not supported. Set those on the machine.":
    "不支持环境变量和请求头，需要的话在机器上自己配。",
  "Remove server": "移除服务器",
  "Something is missing": "缺少内容",
  "Something was removed in the panel but is still there":
    "面板里已经删了，机器上还在",
  "The content differs": "内容和面板不同",
  "A server of this name is already there and was not written by the panel":
    "已有同名服务器，不是面板写的，不会覆盖",
  "The file can not be read": "文件读不了，格式不对",
  "The begin and end markers do not match": "开始和结束标记对不上",
  "Last checked": "上次检查",
  "All machines are in sync": "所有机器都一致",
  "{n} machines are out of sync": "{n} 台机器不一致",
  "{n} machines have a conflict": "{n} 台机器有冲突",
  "Config delivery commands": "配置下发",
  "Open config delivery": "打开配置下发",
});
