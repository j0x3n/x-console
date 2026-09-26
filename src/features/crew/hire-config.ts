import type { HireTemplate, StringMap } from "../../types/domain";
export const hireTemplates: HireTemplate[] = [
  {
    id: "relay",
    name: "Relay",
    job: ["Qualifies inbound requests and books demos", "评估新咨询并预约演示"],
    duties: [
      ["Reads every demo request within 5 minutes", "5 分钟内查看每条演示请求"],
      [
        "Checks fit: agents in production, team size, stack",
        "评估匹配度：已上线的智能体、团队规模与技术栈",
      ],
      [
        "Books a demo with Kofi, or sends a polite no",
        "预约与 Kofi 的演示，或礼貌婉拒",
      ],
    ],
    sources: ["Email", "Calendar", "Web"],
    runs: 30,
    cost: 2.4,
    calls: 5,
  },
  {
    id: "sentry",
    name: "Sentry",
    job: [
      "Tracks competitors in calls, email and the web",
      "跟踪通话、邮件与网页中的竞品信息",
    ],
    duties: [
      [
        "Flags competitor mentions in call transcripts",
        "标记通话记录中提及的竞品",
      ],
      [
        "Keeps one battlecard per competitor current",
        "持续更新每个竞品的竞争分析卡",
      ],
      [
        "Tells the owner when a deal names a competitor",
        "商机出现竞品时通知负责人",
      ],
    ],
    sources: ["Email", "Calls", "Web"],
    runs: 18,
    cost: 1.9,
    calls: 3,
  },
  {
    id: "tally",
    name: "Tally",
    job: [
      "Rolls up the forecast and flags slipping deals",
      "汇总销售预测并标记延期商机",
    ],
    duties: [
      [
        "Rolls up commit and best case every Friday",
        "每周五汇总确定收入与最佳预期",
      ],
      [
        "Flags deals whose close date slipped twice",
        "标记结单日期已延期两次的商机",
      ],
      ["Drafts your note for the pipeline review", "起草商机管线复盘笔记"],
    ],
    sources: ["Email", "Calendar", "Billing"],
    runs: 12,
    cost: 1.1,
    calls: 2,
  },
  {
    id: "bridge",
    name: "Bridge",
    job: ["Writes the handoff when a deal closes", "商机结单时编写交接记录"],
    duties: [
      [
        "Writes the sales-to-success handoff the day a deal closes",
        "结单当天编写销售到客户成功的交接记录",
      ],
      [
        "Lists every promise made in calls and email",
        "列出通话和邮件中作出的所有承诺",
      ],
      ["Books the kickoff with Ruth", "预约与 Ruth 的启动会议"],
    ],
    sources: ["Email", "Calls", "Billing"],
    runs: 4,
    cost: 0.6,
    calls: 1,
  },
];

export const sourceNames: StringMap = {
  Email: "邮件",
  Calls: "通话",
  Calendar: "日历",
  Web: "网页",
  Usage: "使用数据",
  Billing: "账单",
};

export const modeLabel = (
  mode: string,
  language: import("../../types/domain").Language,
) => (language === "zh" ? (mode === "Ask first" ? "先询问" : "仅建议") : mode);

export function radioKeys(
  event: import("react").KeyboardEvent<HTMLDivElement>,
) {
  if (
    ![
      "ArrowLeft",
      "ArrowRight",
      "ArrowUp",
      "ArrowDown",
      "Home",
      "End",
    ].includes(event.key)
  )
    return;
  const buttons = [
    ...event.currentTarget.querySelectorAll<HTMLButtonElement>(
      '[role="radio"]',
    ),
  ];
  const index = buttons.indexOf(
    (event.target as Element).closest<HTMLButtonElement>('[role="radio"]')!,
  );
  if (index < 0) return;
  event.preventDefault();
  const next =
    event.key === "Home"
      ? 0
      : event.key === "End"
        ? buttons.length - 1
        : (index +
            (["ArrowLeft", "ArrowUp"].includes(event.key) ? -1 : 1) +
            buttons.length) %
          buttons.length;
  buttons[next].focus();
  buttons[next].click();
}
