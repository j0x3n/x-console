import { registerZh } from "../../lib/i18n";

registerZh({
  // B162
  // 和别的模块相同的词，这个模块单独加载时也要有
  Save: "保存",
  Saved: "已保存",
  Close: "关闭",
  Status: "状态",
  Idle: "空闲",
  Offline: "离线",
  Manual: "手动",
  Yesterday: "昨天",
  On: "开",
  Off: "关",
  hr: "小时",
  min: "分钟",
  Copy: "复制",
  Copied: "已复制",
  Address: "地址",
  "In use": "正在使用",
  "At home": "在家",
  "Turn off": "关闭",
  "Home Assistant": "Home Assistant",
  "Home Assistant (optional)": "Home Assistant（可选）",
  "Devices & agents": "设备与代理",
  "Not turned on": "还没有开启",
  "Last report": "最近一次上报",
  "No report received yet": "还没有收到过上报",
  "Create a new token": "重新生成令牌",
  "Create a new token?": "重新生成令牌？",
  "Create report token": "生成上报令牌",
  "Report token created": "已生成上报令牌",
  // 今日页卡片
  "My status": "我的状态",
  "At work": "工作中",
  "Stepped away": "暂离",
  Asleep: "睡觉",
  "Out of home": "外出",
  "Not sure": "不确定",
  Guessed: "推测",
  until: "到",
  "Since {time}": "从 {time} 开始",
  "Change or see history": "改状态或看历史",
  "Judged from your computer": "根据电脑判断",
  "Reported by your phone": "手机上报",
  "Reported by Home Assistant": "Home Assistant 上报",
  "Set by hand": "手动设定",
  "Guessed from your daily schedule": "按作息推测",
  // 手动设定和历史
  "Set it by hand": "手动设定",
  "Use this when the guess is wrong, for example when you lie in bed with the sleep mode on.":
    "判断不对时用它。比如睡眠模式开着，但你醒着。",
  "For how long": "持续多久",
  "Set status": "设定",
  "Back to automatic": "改回自动",
  "1 hour": "1 小时",
  "2 hours": "2 小时",
  "Until the next wake-up time": "到下个起床时间",
  "Until I change it back": "一直到我改回来",
  "Last 7 days": "最近 7 天",
  "Could not load the history": "历史读不出来",
  "Only sleeps of 3 hours or more count as a night's sleep.":
    "睡了 3 小时以上才算一晚的睡眠。",
  // 设置页
  "Which signal to use": "用哪个来源",
  "Your computers tell when you are working. Your phone tells the rest. Two ways work together.":
    "工作中靠电脑判断。睡觉和在不在家靠手机上报。手机有两种方式，可以同时用。",
  "Home and out: use Home Assistant (person or device_tracker). The Home Assistant app finds your place in the background more reliably.":
    "在家和外出：用 Home Assistant 的 person 或 device_tracker。手机 App 在后台定位更稳。",
  "Sleep: use an iPhone shortcut with the Sleep focus mode or the bedtime. Home Assistant cannot tell whether a focus mode is the Sleep one.":
    "睡觉：用 iPhone 快捷指令，触发条件选睡眠专注模式或就寝时间。Home Assistant 的专注模式传感器分不出是不是睡眠模式。",
  "If both report the same thing within 2 minutes, it counts once. If they disagree, the last one received wins.":
    "两边在 2 分钟内报同一件事，只算一次。两边说法不一样时，以最后收到的为准。",
  "Work computers": "工作电脑",
  "Working means one of these is in use. If you pick none, every computer that reports idle time counts.":
    "其中任意一台在用，就算工作中。一台都不选时，所有能报空闲时间的电脑都算。",
  "No computer reports idle time yet. Pair a Windows agent in Devices first.":
    "还没有电脑报空闲时间。先在“设备与代理”里配对 Windows 代理。",
  "Screen locked": "已锁屏",
  "Guess sleep from my schedule": "按作息推测睡觉",
  "Guess that I am asleep": "推测我在睡觉",
  "From 1 hour before your bedtime to your wake-up time, when no work computer was used for 30 minutes and you are not out. It is marked Guessed.":
    "从睡觉时间前 1 小时到起床时间，工作电脑 30 分钟没人用，而且你没出门。界面上会标“推测”。",
  "Edit the daily schedule": "改作息",
  "Phone report": "手机上报",
  "An iPhone shortcut (or any script) posts one event with a token. It sends no location, only the event name.":
    "iPhone 快捷指令或脚本带着令牌报一件事。只报事件名，不报位置。",
  "Sleep focus mode on, or bedtime starts": "睡眠专注模式开启，或就寝时间开始",
  "Sleep focus mode off, wake up, or alarm stopped":
    "睡眠专注模式关闭，醒来，或闹钟停止",
  "Arrive home, or join the home Wi-Fi": "到达家，或连上家里的 Wi-Fi",
  "Leave home, or leave the home Wi-Fi": "离开家，或断开家里的 Wi-Fi",
  "The old token stops working at once. Update your shortcuts with the new one.":
    "旧令牌立即失效。用新令牌更新你的快捷指令。",
  "Turn off phone report?": "关闭手机上报？",
  "The token stops working. Your shortcuts will get an error until you make a new token.":
    "令牌立即失效。生成新令牌之前，快捷指令会报错。",
  "Phone report turned off": "手机上报已关闭",
  "The token is shown only once. Copy it now, or copy the command.":
    "令牌只显示这一次。现在复制令牌，或者复制下面的命令。",
  "iPhone shortcut steps": "iPhone 快捷指令的配置步骤",
  "Open the Shortcuts app, tap Automation, then the + at the top right.":
    "打开“快捷指令”App，点“自动化”，点右上角的“+”。",
  "Pick a trigger. For example Sleep > Bedtime Starts, or Focus > Sleep > When Turning On.":
    "选触发条件。比如“睡眠”里的“就寝时间开始”，或者“专注模式”里的“睡眠 · 开启时”。",
  "Choose Run Immediately and turn off Notify When Run.":
    "选“立即运行”，关掉“运行时通知”。",
  "Add the action Get Contents of URL. Put the address above in the URL, set the method to POST and add the header Authorization with the value Bearer plus your token. Set the request body to JSON and add one field event with the value sleep.start.":
    "动作选“获取 URL 内容”。地址填上面的上报地址，方法选 POST，头部加 Authorization，值是 Bearer 加空格再加令牌。请求体选 JSON，加一个字段 event，值是 sleep.start。",
  "Make more in the same way: sleep.end when the Sleep focus mode turns off or the alarm stops, home.arrive when you arrive home, home.leave when you leave home.":
    "按同样的方法再配：睡眠模式关闭或闹钟停止报 sleep.end，到达家报 home.arrive，离开家报 home.leave。",
  "Connect Home Assistant in Settings first.":
    "先在设置里连接 Home Assistant。",
  "Leave a field empty to skip it.": "不用的留空。",
  "At-home entity": "在家的实体",
  "person.xxx or device_tracker.xxx. The state home means arrived. Any other state means left.":
    "person.xxx 或 device_tracker.xxx。状态是 home 算到家，其他状态算离开家。",
  "Sleep focus entity": "睡眠专注的实体",
  "The focus sensor of the Home Assistant app. On means asleep. Off means awake.":
    "手机 App 的专注模式传感器。on 算开始睡觉，off 算醒来。",
  "This sensor is on for every focus mode, not only Sleep. If you also use other focus modes in the daytime, leave it empty and use a shortcut instead.":
    "这个传感器在任何专注模式开启时都是 on，不只是睡眠模式。白天也会开别的专注模式的话，不要填，改用快捷指令。",
});
