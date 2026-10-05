import type { HabitInput, HabitTemplate } from "./api";
import { HABIT_TEMPLATES } from "./presence";

/*
 * 推荐习惯和健身方案（用户 2026-10-05 要求）。
 * 推荐页的卡片有三种来源：
 *   1. 健康提醒模板（presence.ts 的 HABIT_TEMPLATES，服务端按 template 补默认值）
 *   2. 个人计划的打卡模板（服务端 /habits/library 的 habits，用 activate 接口加入）
 *   3. 这里写死的常见习惯（点了打开新建弹窗并填好，用户可以改提醒）
 * 健身方案按“身体问题”和“每天的基础动作”分组，动作 id 都来自个人计划的动作库。
 * 加入习惯时用方案的 habit.name 新建一个打卡习惯，今天页按名字找回方案，显示“看动作”。
 */

export type RecommendCategory =
  | "health"
  | "routine"
  | "food"
  | "learn"
  | "mind"
  | "life"
  | "body";

export const recommendCategories: { id: RecommendCategory; label: string }[] = [
  { id: "health", label: "Health reminders" },
  { id: "routine", label: "Routine" },
  { id: "food", label: "Food" },
  { id: "learn", label: "Learning" },
  { id: "mind", label: "Mind" },
  { id: "life", label: "Life" },
  { id: "body", label: "Posture & exercise" },
];

export interface Recommendation {
  id: string;
  category: RecommendCategory;
  name: string;
  icon: string;
  /** 一句话：为什么做、怎么做 */
  summary: string;
  input: Omit<HabitInput, "activeHostIds">;
}

const daily = (
  name: string,
  icon: string,
  extra: Partial<HabitInput> = {},
): Omit<HabitInput, "activeHostIds"> => ({
  name,
  icon,
  unit: "次",
  dailyTarget: 1,
  remindMode: "none",
  ...extra,
});

/** 写死的常见习惯。名字就是新建出来的习惯名，判断“已添加”也按名字。 */
export const EXTRA_RECOMMENDATIONS: Recommendation[] = [
  {
    id: "sun",
    category: "routine",
    name: "上午晒太阳 10 分钟",
    icon: "☀️",
    summary: "起床后两小时内到户外或窗边，晚上更容易困。",
    input: daily("上午晒太阳 10 分钟", "☀️", {
      remindMode: "times",
      remindTimes: ["09:30"],
    }),
  },
  {
    id: "no-phone-bed",
    category: "routine",
    name: "睡前 30 分钟不看手机",
    icon: "📵",
    summary: "手机放到床够不着的地方，换成看纸书或拉伸。",
    input: daily("睡前 30 分钟不看手机", "📵", {
      remindMode: "times",
      remindTimes: ["22:30"],
    }),
  },
  {
    id: "nap",
    category: "routine",
    name: "午休不超过 30 分钟",
    icon: "😴",
    summary: "定个闹钟，睡太久下午反而更困。",
    input: daily("午休不超过 30 分钟", "😴"),
  },
  {
    id: "teeth",
    category: "routine",
    name: "睡前用牙线",
    icon: "🦷",
    summary: "刷牙刷不到牙缝，每天一次就够。",
    input: daily("睡前用牙线", "🦷"),
  },
  {
    id: "veg",
    category: "food",
    name: "每天吃够蔬菜",
    icon: "🥗",
    summary: "一天两拳头的蔬菜，一餐一份最省事。",
    input: daily("每天吃够蔬菜", "🥗", { unit: "份", dailyTarget: 2 }),
  },
  {
    id: "fruit",
    category: "food",
    name: "吃一份水果",
    icon: "🍎",
    summary: "一个拳头大小，代替饭后甜点。",
    input: daily("吃一份水果", "🍎", { unit: "份" }),
  },
  {
    id: "no-sugar",
    category: "food",
    name: "不喝含糖饮料",
    icon: "🚫",
    summary: "奶茶、果汁、可乐都算。想喝时换成茶或气泡水。",
    input: daily("不喝含糖饮料", "🚫"),
  },
  {
    id: "dinner-early",
    category: "food",
    name: "睡前 3 小时不吃东西",
    icon: "🍽️",
    summary: "晚饭早点吃完，睡眠和胃都会舒服些。",
    input: daily("睡前 3 小时不吃东西", "🍽️"),
  },
  {
    id: "read",
    category: "learn",
    name: "读书 20 分钟",
    icon: "📖",
    summary: "固定一个时间，比如午饭后或睡前，读几页也算。",
    input: daily("读书 20 分钟", "📖", { unit: "分钟", dailyTarget: 20 }),
  },
  {
    id: "write",
    category: "learn",
    name: "写三行日记",
    icon: "✍️",
    summary: "今天做了什么、学到什么、明天先做什么。",
    input: daily("写三行日记", "✍️", {
      remindMode: "times",
      remindTimes: ["22:00"],
    }),
  },
  {
    id: "listen",
    category: "learn",
    name: "通勤听播客或网课",
    icon: "🎧",
    summary: "把路上的时间用起来，每天一集。",
    input: daily("通勤听播客或网课", "🎧"),
  },
  {
    id: "meditate",
    category: "mind",
    name: "冥想 10 分钟",
    icon: "🧘",
    summary: "坐着数呼吸，走神了就拉回来。刚开始 3 分钟也行。",
    input: daily("冥想 10 分钟", "🧘", { unit: "分钟", dailyTarget: 10 }),
  },
  {
    id: "breath",
    category: "mind",
    name: "紧张时深呼吸 1 分钟",
    icon: "🌬️",
    summary: "吸 4 秒、停 4 秒、呼 6 秒，做 5 轮。",
    input: daily("紧张时深呼吸 1 分钟", "🌬️"),
  },
  {
    id: "gratitude",
    category: "mind",
    name: "记一件好事",
    icon: "🙏",
    summary: "睡前写下今天一件顺利或开心的小事。",
    input: daily("记一件好事", "🙏"),
  },
  {
    id: "tidy",
    category: "life",
    name: "整理桌面 5 分钟",
    icon: "🧹",
    summary: "下班前收好桌面和电脑桌面，第二天开工更快。",
    input: daily("整理桌面 5 分钟", "🧹", {
      remindMode: "times",
      remindTimes: ["18:00"],
    }),
  },
  {
    id: "money",
    category: "life",
    name: "记账",
    icon: "💰",
    summary: "当天的花销当天记，周末看一眼钱花在哪了。",
    input: daily("记账", "💰", {
      remindMode: "times",
      remindTimes: ["21:30"],
    }),
  },
  {
    id: "plan-tomorrow",
    category: "life",
    name: "睡前列好明天三件事",
    icon: "📝",
    summary: "只写最重要的三件，早上起来先做第一件。",
    input: daily("睡前列好明天三件事", "📝"),
  },
  {
    id: "call-family",
    category: "life",
    name: "给家人打个电话",
    icon: "📞",
    summary: "一周两三次，每次几分钟就好。",
    input: daily("给家人打个电话", "📞"),
  },
];

const templateSummary: Record<HabitTemplate, string> = {
  water: "每小时提醒一次，醒着的时候才提醒。",
  eyes: "在用电脑时每 20 分钟提醒看远处。",
  move: "在用电脑时每 45 分钟提醒起来走走。",
  medicine: "按固定时间提醒，早晚各一次。",
};

/** 健康提醒模板转成推荐卡片。 */
export const TEMPLATE_RECOMMENDATIONS: (Recommendation & {
  template: HabitTemplate;
})[] = HABIT_TEMPLATES.map((tpl) => ({
  id: `tpl-${tpl.id}`,
  category: "health",
  name: tpl.input.name,
  icon: tpl.input.icon ?? "",
  summary: templateSummary[tpl.id],
  input: tpl.input,
  template: tpl.id,
}));

/** 个人计划的打卡模板分到哪个推荐分类。 */
export function libraryCategory(h: {
  category: string;
  exerciseId?: string;
}): RecommendCategory {
  if (h.exerciseId) return "body";
  if (h.category === "food") return "food";
  if (h.category === "english") return "learn";
  return "routine";
}

/* ---------------- 健身方案 ---------------- */

export type ProgramKind = "problem" | "daily";

export interface ProgramItem {
  /** 动作库的 id */
  exerciseId: string;
  /** 这个方案里的量，比动作库的参考量轻 */
  dose: string;
}

export interface FitnessProgram {
  id: string;
  kind: ProgramKind;
  name: string;
  icon: string;
  /** 适合谁 */
  summary: string;
  /** 大约几分钟 */
  minutes: number;
  /** 出现这些情况先看医生 */
  caution?: string;
  items: ProgramItem[];
  /** 加入习惯时用的名字和默认提醒时间 */
  habitName: string;
  remindAt: string;
}

export const FITNESS_PROGRAMS: FitnessProgram[] = [
  {
    id: "low-back",
    kind: "problem",
    name: "久坐腰背酸",
    icon: "🪑",
    summary: "坐一天腰发酸、发紧。练躯干耐力，比只拉伸更管用。",
    minutes: 10,
    caution: "腿麻、腿痛往下窜，或者夜里痛醒，先去看医生。",
    items: [
      { exerciseId: "curlup", dose: "3 次 × 8 秒" },
      { exerciseId: "sideplank", dose: "每侧 3 次 × 8 秒" },
      { exerciseId: "birddog", dose: "每侧 5 次 × 8 秒" },
      { exerciseId: "bridge", dose: "2 组 × 12 次" },
      { exerciseId: "hipstretch", dose: "每侧 30 秒" },
    ],
    habitName: "护腰练习",
    remindAt: "12:30",
  },
  {
    id: "neck",
    kind: "problem",
    name: "脖子前伸、低头多",
    icon: "📱",
    summary: "看屏幕时头往前探，脖子后面发紧。",
    minutes: 6,
    caution: "手麻、头晕或者转头时剧痛，先去看医生。",
    items: [
      { exerciseId: "chin", dose: "8 次 × 5 秒" },
      { exerciseId: "wallslide", dose: "2 组 × 8 次" },
      { exerciseId: "thoracic", dose: "2 个位置 × 6 次" },
      { exerciseId: "shoulderCircle", dose: "前后各 10 圈" },
    ],
    habitName: "颈部放松",
    remindAt: "15:00",
  },
  {
    id: "rounded",
    kind: "problem",
    name: "圆肩驼背",
    icon: "🧍",
    summary: "肩膀往前扣，后背拱起来。练上背和肩后侧。",
    minutes: 10,
    items: [
      { exerciseId: "wallslide", dose: "2 组 × 10 次" },
      { exerciseId: "facepull", dose: "2 组 × 15 次" },
      { exerciseId: "rotation", dose: "每侧 2 组 × 12 次" },
      { exerciseId: "thoracic", dose: "2 个位置 × 6 次" },
      { exerciseId: "hang", dose: "脚辅助 3 次 × 10 秒" },
    ],
    habitName: "体态练习",
    remindAt: "19:30",
  },
  {
    id: "hip",
    kind: "problem",
    name: "髋前侧紧、臀部无力",
    icon: "🍑",
    summary: "坐久了站起来大腿根发紧，走路屁股不发力。",
    minutes: 8,
    items: [
      { exerciseId: "hipstretch", dose: "每侧 2 × 30 秒" },
      { exerciseId: "bridge", dose: "2 组 × 15 次" },
      { exerciseId: "singleBridge", dose: "每侧 1 组 × 8 次" },
      { exerciseId: "hipCircle", dose: "每侧每方向 6 圈" },
    ],
    habitName: "髋部激活",
    remindAt: "18:30",
  },
  {
    id: "knee",
    kind: "problem",
    name: "膝盖发软、下楼不稳",
    icon: "🦵",
    summary: "上下楼膝盖没劲，蹲起时晃。练大腿和臀部，膝盖更稳。",
    minutes: 10,
    caution: "膝盖肿、卡住或者打软腿，先去看医生。",
    items: [
      { exerciseId: "bridge", dose: "2 组 × 12 次" },
      { exerciseId: "split", dose: "扶稳，每侧 2 组 × 8 次" },
      { exerciseId: "calf", dose: "2 组 × 15 次" },
      { exerciseId: "quadStretch", dose: "每侧 30 秒" },
    ],
    habitName: "护膝练习",
    remindAt: "19:00",
  },
  {
    id: "shoulder",
    kind: "problem",
    name: "肩膀僵硬",
    icon: "💆",
    summary: "肩膀抬不高、发沉，鼠标用久了更明显。",
    minutes: 6,
    caution: "肩膀夜里痛，或者手臂抬不过头，先去看医生。",
    items: [
      { exerciseId: "shoulderCircle", dose: "前后各 10 圈" },
      { exerciseId: "wallslide", dose: "2 组 × 8 次" },
      { exerciseId: "rotation", dose: "每侧 2 组 × 12 次" },
    ],
    habitName: "肩部放松",
    remindAt: "16:00",
  },
  {
    id: "hamstring",
    kind: "problem",
    name: "腿后侧紧、弯腰够不到脚",
    icon: "🙇",
    summary: "站着弯腰手离地面很远，腿后侧一拉就紧。",
    minutes: 8,
    items: [
      { exerciseId: "hamstretch", dose: "每侧 2 × 30 秒" },
      { exerciseId: "hinge", dose: "2 组 × 8 次" },
      { exerciseId: "legSwing", dose: "每侧 10 次" },
      { exerciseId: "calfStretch", dose: "每侧 30 秒" },
    ],
    habitName: "腿后侧拉伸",
    remindAt: "21:30",
  },
  {
    id: "balance",
    kind: "problem",
    name: "站不稳、平衡差",
    icon: "⚖️",
    summary: "单脚站一会儿就晃，走路容易崴脚。",
    minutes: 8,
    items: [
      { exerciseId: "split", dose: "扶稳，每侧 2 组 × 8 次" },
      { exerciseId: "calf", dose: "2 组 × 12 次" },
      { exerciseId: "lateralSwing", dose: "每侧 10 次" },
      { exerciseId: "birddog", dose: "每侧 5 次 × 8 秒" },
    ],
    habitName: "平衡练习",
    remindAt: "19:30",
  },
  {
    id: "stamina",
    kind: "problem",
    name: "体力差、爬楼喘",
    icon: "🫁",
    summary: "爬两层楼就喘。从走路开始，慢慢加时间和坡度。",
    minutes: 25,
    caution: "运动时胸口痛、心慌或者头晕，马上停下并去看医生。",
    items: [
      { exerciseId: "highMarch", dose: "热身 60 秒" },
      { exerciseId: "walk", dose: "快走 20 分钟" },
      { exerciseId: "inclineWalk", dose: "能做时换成低坡 15 分钟" },
    ],
    habitName: "有氧快走",
    remindAt: "19:00",
  },
  {
    id: "morning",
    kind: "daily",
    name: "起床活动 5 分钟",
    icon: "🌅",
    summary: "起床后把关节都动一遍，人会精神一些。",
    minutes: 5,
    items: [
      { exerciseId: "shoulderCircle", dose: "前后各 10 圈" },
      { exerciseId: "hipCircle", dose: "每侧每方向 5 圈" },
      { exerciseId: "legSwing", dose: "每侧 8 次" },
      { exerciseId: "chin", dose: "5 次 × 3 秒" },
      { exerciseId: "highMarch", dose: "30 秒" },
    ],
    habitName: "起床活动",
    remindAt: "08:00",
  },
  {
    id: "desk-break",
    kind: "daily",
    name: "久坐间歇 2 分钟",
    icon: "💻",
    summary: "每坐一小时站起来做一轮，不用换衣服。",
    minutes: 2,
    items: [
      { exerciseId: "chin", dose: "5 次 × 3 秒" },
      { exerciseId: "wallslide", dose: "1 组 × 8 次" },
      { exerciseId: "calf", dose: "1 组 × 15 次" },
      { exerciseId: "highMarch", dose: "30 秒" },
    ],
    habitName: "工间操",
    remindAt: "15:30",
  },
  {
    id: "bodyweight",
    kind: "daily",
    name: "徒手基础力量 15 分钟",
    icon: "💪",
    summary: "不用器械，在家就能练全身。一周 2 到 3 次。",
    minutes: 15,
    items: [
      { exerciseId: "split", dose: "每侧 2 组 × 10 次" },
      { exerciseId: "pushup", dose: "2 组 × 8 次，可以扶高台" },
      { exerciseId: "bridge", dose: "2 组 × 15 次" },
      { exerciseId: "deadbug", dose: "每侧 2 组 × 8 次" },
      { exerciseId: "sideplank", dose: "每侧 3 次 × 10 秒" },
    ],
    habitName: "徒手力量",
    remindAt: "19:30",
  },
  {
    id: "core",
    kind: "daily",
    name: "核心 8 分钟",
    icon: "🎯",
    summary: "卷腹、侧撑、鸟狗三个动作，练腰腹的耐力。",
    minutes: 8,
    items: [
      { exerciseId: "curlup", dose: "5 次 × 8 秒" },
      { exerciseId: "sideplank", dose: "每侧 4 次 × 8 秒" },
      { exerciseId: "birddog", dose: "每侧 5 次 × 8 秒" },
      { exerciseId: "deadbug", dose: "每侧 8 次" },
    ],
    habitName: "核心练习",
    remindAt: "20:00",
  },
  {
    id: "warmup",
    kind: "daily",
    name: "运动前热身",
    icon: "🔥",
    summary: "跑步或力量训练前做一遍，关节先活动开。",
    minutes: 5,
    items: [
      { exerciseId: "highMarch", dose: "60 秒" },
      { exerciseId: "legSwing", dose: "每侧 10 次" },
      { exerciseId: "lateralSwing", dose: "每侧 10 次" },
      { exerciseId: "hipCircle", dose: "每侧每方向 5 圈" },
      { exerciseId: "shoulderCircle", dose: "前后各 10 圈" },
    ],
    habitName: "运动前热身",
    remindAt: "18:30",
  },
  {
    id: "bedtime",
    kind: "daily",
    name: "睡前拉伸 10 分钟",
    icon: "🌙",
    summary: "放慢呼吸，拉开一天坐紧的地方，帮助入睡。",
    minutes: 10,
    items: [
      { exerciseId: "hipstretch", dose: "每侧 30 秒" },
      { exerciseId: "hamstretch", dose: "每侧 30 秒" },
      { exerciseId: "quadStretch", dose: "每侧 30 秒" },
      { exerciseId: "calfStretch", dose: "每侧 30 秒" },
      { exerciseId: "thoracic", dose: "1 个位置 × 6 次" },
    ],
    habitName: "睡前拉伸",
    remindAt: "22:00",
  },
  {
    id: "walking",
    kind: "daily",
    name: "每天走路",
    icon: "🚶",
    summary: "饭后走 10 分钟，或者通勤提前一站下车。",
    minutes: 20,
    items: [{ exerciseId: "walk", dose: "累计 20 分钟" }],
    habitName: "每天快走",
    remindAt: "19:00",
  },
];

export function programHabitInput(
  p: FitnessProgram,
): Omit<HabitInput, "activeHostIds"> {
  return {
    name: p.habitName,
    icon: p.icon,
    unit: "次",
    dailyTarget: 1,
    remindMode: "times",
    remindTimes: [p.remindAt],
  };
}

/** 单个动作加入习惯时的默认值。 */
export function exerciseHabitInput(e: {
  name: string;
  group: string;
}): Omit<HabitInput, "activeHostIds"> {
  return {
    name: e.name,
    icon: groupIcons[e.group] ?? "💪",
    unit: "次",
    dailyTarget: 1,
    remindMode: "none",
  };
}

const groupIcons: Record<string, string> = {
  下肢: "🦵",
  上肢: "💪",
  核心: "🎯",
  体态: "🧘",
  有氧: "🏃",
};

/** 习惯名对应的健身方案，今天页用来给卡片加“看动作”。 */
export function programForHabit(name: string): FitnessProgram | undefined {
  return FITNESS_PROGRAMS.find((p) => p.habitName === name);
}

/** 名字相同算已添加。去掉空格比较，避免手动多打一个空格就重复建。 */
export function sameName(a: string, b: string) {
  return a.replace(/\s+/g, "") === b.replace(/\s+/g, "");
}
