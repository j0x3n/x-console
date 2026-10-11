import { registerZh } from "../../lib/i18n";

registerZh({
  // B131
  // 和别的模块相同的词，这个模块单独加载时也要有
  Offline: "离线",
  Save: "保存",
  Saved: "已保存",
  "Devices & agents": "设备与代理",
  "Focus block": "专注屏蔽",
  "Block sites and programs on your computers while a pomodoro runs.":
    "番茄钟进行时，在你的电脑上屏蔽网站和结束程序。",
  "Turn on focus block": "开启专注屏蔽",
  "Computers to block": "要屏蔽的电脑",
  "No paired computer yet. Pair an agent in Devices first.":
    "还没有配对的电脑。先在“设备与代理”里配对。",
  "Needs a newer agent": "代理版本太旧",
  "Block is on": "屏蔽已生效",
  "No permission": "权限不足",
  "Block failed": "出错了",
  "Waiting for focus": "等专注开始",
  "Update the agent": "要更新代理",
  "Sites to block": "要屏蔽的网站",
  "One site per line. You can paste a whole address.":
    "一行一个。可以直接粘贴网址。",
  "Also block IPv6": "同时屏蔽 IPv6",
  "Programs to end": "要结束的程序",
  "One program name per line, like steam. Capital letters and .exe do not matter. Only the name is compared, never the path.":
    "一行一个程序名，比如 steam。大小写和 .exe 都不影响。只比对名字，不比对路径。",
  "Import entertainment programs": "导入娱乐类程序",
  "Programs that Computer time sorted as entertainment":
    "电脑时间里分到“娱乐”类的程序",
  "No new entertainment program found": "没有新的娱乐类程序",
  "programs added. Press Save lists to use them.":
    "个程序已加进名单。点“保存名单”后生效。",
  "Save lists": "保存名单",
  "Break time": "休息时间",
  "Keep blocking in the break": "休息时也屏蔽",
  "After the time is up and until you press Finish. Off by default, so the break is free.":
    "时间到了以后，到你点“完成”之前。默认不屏蔽，休息时放开。",
  "Home Assistant (optional)": "Home Assistant（可选）",
  "Turn on a scene or script when focus starts, and another when the block ends. Leave empty to skip.":
    "专注开始时打开一个场景或脚本，屏蔽结束时再打开另一个。留空就不用。",
  "Scene or script at the start": "开始时的场景或脚本",
  "Scene or script at the end": "结束时的场景或脚本",
  "Save Home Assistant": "保存 Home Assistant 设置",
  "Good to know": "使用前要知道",
  "Secure DNS (DNS over HTTPS) in the browser can skip the hosts file. Turn it off in the browser settings.":
    "浏览器开了“安全 DNS”（DoH）时，可能绕过 hosts 文件。请在浏览器设置里关掉。",
  "Only computers are covered. Phones and tablets are not.":
    "只管电脑，管不到手机和平板。",
  "Changing the hosts file needs administrator rights on Windows and root on Linux. Without them, only programs are ended.":
    "改 hosts 文件在 Windows 上要管理员权限，在 Linux 上要 root。没有权限时只会结束程序。",
  "If the panel or the agent stops, the agent restores everything by itself 5 minutes after the planned end.":
    "面板或代理中断时，代理会在预定结束时间的 5 分钟后自己恢复。",
  "Pages that are already open may need a reload.":
    "已经打开的网页可能要刷新一下才会被屏蔽。",
  "Blocking now": "屏蔽中",
  "Block has a problem": "屏蔽有问题",
  "Sites and programs are blocked on your computers.":
    "你的电脑上正在屏蔽网站和程序。",
});
