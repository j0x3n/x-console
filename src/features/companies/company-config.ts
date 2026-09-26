import type { Localize } from "../../types/domain";
export {
  ownerNames as names,
  agentMarks as marks,
  agentColors as colors,
  stages as stageNames,
  stageZh,
  rolesZh as roleZh,
  forecastZh,
} from "../../data/catalogs";

export const statusZh: Record<string, string> = {
  Running: "进行中",
  Queued: "排队中",
  "Needs your call": "待你决定",
  "Done today": "今日完成",
};

export const industryZh: Record<string, string> = {
  "Warehouse robotics": "仓储机器人",
  "Freight logistics": "货运物流",
  "Lab automation": "实验室自动化",
  Healthcare: "医疗健康",
  "Aerospace software": "航空航天软件",
  Payments: "支付",
  "Energy infrastructure": "能源基础设施",
  Agritech: "农业科技",
  "Industrial manufacturing": "工业制造",
  Banking: "银行业",
  "HR software": "人力资源软件",
  Biotech: "生物科技",
  Insurance: "保险业",
  "Asset management": "资产管理",
  Cybersecurity: "网络安全",
  "Renewable energy": "可再生能源",
  "Developer tools": "开发者工具",
  "Creative agency": "创意机构",
  "Publishing software": "出版软件",
  "Data platforms": "数据平台",
  "Climate software": "气候科技",
  "E-commerce tooling": "电商工具",
  "Maritime logistics": "海运物流",
  "Consumer fintech": "消费金融科技",
};

export const timelineTitlesZh: Record<string, string> = {
  "Echo prepared the pricing call brief": "Echo 准备了定价电话会议简报",
  "Scribe drafted a follow-up to Priya Raman":
    "Scribe 起草了给 Priya Raman 的跟进邮件",
  "Echo mapped the attendees for the pricing call": "Echo 梳理了定价会议参会者",
  "Echo filed notes from the demo": "Echo 归档了演示记录",
  "Theo Park met · Product demo · 47 min": "Theo Park 参加了产品演示 · 47 分钟",
  "Priya Raman emailed Re: Agenda for tomorrow’s demo":
    "Priya Raman 回复了演示议程邮件",
  "Theo Park sent Agenda for tomorrow’s demo": "Theo Park 发送了演示议程",
  "Scout found a buying signal: hiring 4 ML platform engineers":
    "Scout 发现购买信号：招聘 4 位机器学习平台工程师",
  "Scout spotted Priya’s post on agent reliability":
    "Scout 发现 Priya 关于智能体可靠性的动态",
  "Theo Park called · Technical discovery with Priya · 32 min":
    "Theo Park 与 Priya 进行了技术需求电话 · 32 分钟",
  "Pilot moved to Proposal": "Pilot 将阶段推进至方案",
  "Theo Park sent Proposal: X Console for the Halcyon fleet":
    "Theo Park 发送了 Halcyon 车队方案",
  "Theo Park met · Evaluation workshop · 60 min":
    "Theo Park 参加了评估研讨会 · 60 分钟",
  "Scribe answered the security questionnaire": "Scribe 回答了安全问卷",
  "Theo Park called · Intro call · 25 min":
    "Theo Park 进行了介绍电话 · 25 分钟",
  "Scout researched the account: 240 robots in production":
    "Scout 调研了客户：已有 240 台机器人投入使用",
  "Ledger flagged renewal risk: health 81 → 63 in 14 days":
    "Ledger 提醒续约风险：健康度 14 天内从 81 降至 63",
  "Scribe summarised the support thread on route sync":
    "Scribe 总结了路线同步支持工单",
  "Sam Okoye emailed Route sync still failing for the west depot":
    "Sam Okoye 发邮件反馈西部站点路线同步仍失败",
  "Ledger seat usage down 22% (118 → 92)":
    "Ledger 发现席位使用量下降 22%（118 → 92）",
  "Ruth Adler called · Check-in with Dana Whitcombe · 18 min":
    "Ruth Adler 与 Dana Whitcombe 沟通 · 18 分钟",
  "Scout found a signal: Oakline opened a Head of Data role":
    "Scout 发现 Oakline 新增数据负责人岗位",
  "Tomás Reyes emailed Escalation: route sync errors":
    "Tomás Reyes 发邮件升级路线同步问题",
  "Ledger invoice paid 12 days late": "Ledger 发现发票逾期 12 天支付",
  "Ruth Adler noted · Dana is out until Sep 18":
    "Ruth Adler 记录 Dana 将离岗至 9 月 18 日",
  "Ledger health dipped below 80 for the first time":
    "Ledger 发现健康度首次跌破 80",
  "Ruth Adler met · Q3 business review · 45 min":
    "Ruth Adler 参加第三季度业务回顾 · 45 分钟",
  "Scribe drafted the QBR recap": "Scribe 起草了业务回顾摘要",
  "Ruth Adler met · Quarterly business review · 50 min":
    "Ruth Adler 参加季度业务回顾 · 50 分钟",
  "Pilot opened the renewal: $184K, due Nov 30":
    "Pilot 创建了 $184K 续约商机，11 月 30 日到期",
  "Pilot proposed moving to Negotiation": "Pilot 建议进入谈判阶段",
  "Nikhil Rao replied by email": "Nikhil Rao 回复了邮件",
  "Pilot opened the second business unit deal": "Pilot 创建了第二业务部门商机",
  "Ines Duarte called · Intro call with Nikhil Rao · 30 min":
    "Ines Duarte 与 Nikhil Rao 进行了介绍电话 · 30 分钟",
  "Scout found a signal: security review passed": "Scout 发现安全审查已通过",
  "Pilot opened the platform deal": "Pilot 创建了平台商机",
  "Scout researched the account": "Scout 调研了该客户",
};

export const timelineDetailsZh: Record<string, string> = {
  "Owen will push on the 120-robot price. Priya wants decision replay in the base tier.":
    "Owen 会关注 120 台机器人的价格。Priya 希望在基础档加入决策回放。",
  "Waiting for your review on Today.": "正在今日页面等待你审阅。",
  "Jun asked how we trace handoffs between picking agents. Priya asked for an on-prem collector.":
    "Jun 询问如何追踪分拣智能体间的交接；Priya 希望增加本地采集器。",
  "Fleet telemetry, decision replay, cost per robot-hour.":
    "车队遥测、决策回放、每台机器人每小时成本。",
  "Can we spend ten minutes on fleet telemetry? Jun will join.":
    "能否花十分钟讨论车队遥测？Jun 也会参加。",
  "Three parts: tracing, replay, and what it costs per robot-hour.":
    "分三部分：追踪、回放、每台机器人每小时成本。",
  "Proposed a check-in with Ruth and Dana this week. Waiting for your call on Today.":
    "建议本周安排与 Ruth 和 Dana 沟通，正在今日页面等待你决定。",
  "West depot sync fails after the carrier API change. Engineering has a fix in review.":
    "承运商 API 变更后，西部站点同步失败。工程团队的修复方案正在审核。",
  "Drivers are back on paper manifests at the west depot. Any update?":
    "西部站点司机已恢复使用纸质清单。现在有进展吗？",
};

export const monthZh: Record<string, string> = {
  Jan: "1",
  Feb: "2",
  Mar: "3",
  Apr: "4",
  May: "5",
  Jun: "6",
  Jul: "7",
  Aug: "8",
  Sep: "9",
  Oct: "10",
  Nov: "11",
  Dec: "12",
};

export const shortDate = (value: string | undefined, L: Localize) =>
  L(
    value,
    value?.replace(
      /^(\w+) (\d+)$/,
      (_, m, d) => `${monthZh[m] || m} 月 ${d} 日`,
    ),
  );

export const localizedTime = (value: string | undefined, L: Localize) =>
  L(
    value,
    value?.replace(
      /^(\d+)([mh]) ago$/,
      (_, n, unit) => `${n} ${unit === "m" ? "分钟前" : "小时前"}`,
    ),
  );
