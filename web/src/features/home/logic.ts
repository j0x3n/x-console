/*
 * 智能家居页面的纯逻辑：按域分组、搜索、每种实体点一下做什么、状态怎么显示。
 * 这里不依赖 React，方便单元测试。
 */

export interface EntityState {
  entityId: string;
  state: string;
  attributes: Record<string, unknown>;
  lastChanged: string;
}

export function domainOf(entityId: string): string {
  const i = entityId.indexOf(".");
  return i < 0 ? entityId : entityId.slice(0, i);
}

export function displayName(
  entityId: string,
  state?: EntityState,
  alias?: string,
): string {
  if (alias) return alias;
  const name = state?.attributes.friendly_name;
  if (typeof name === "string" && name) return name;
  return entityId;
}

/** 点一下能开关的域。 */
const toggleDomains = new Set([
  "light",
  "switch",
  "fan",
  "input_boolean",
  "automation",
  "humidifier",
  "media_player",
  "siren",
]);

export type TapAction =
  | { kind: "toggle"; domain: string; service: "turn_on" | "turn_off" }
  | { kind: "run"; domain: string; service: string }
  | { kind: "none" };

/**
 * 卡片被点一下时调用的服务。门锁、窗帘这类有风险的设备不做一键操作，
 * 只显示状态，要控制时去“全部实体”里点按钮。
 */
export function tapAction(entityId: string, state?: EntityState): TapAction {
  const domain = domainOf(entityId);
  if (toggleDomains.has(domain)) {
    const on = state ? isOn(state) : false;
    return { kind: "toggle", domain, service: on ? "turn_off" : "turn_on" };
  }
  switch (domain) {
    case "scene":
    case "script":
      return { kind: "run", domain, service: "turn_on" };
    case "button":
    case "input_button":
      return { kind: "run", domain, service: "press" };
    default:
      return { kind: "none" };
  }
}

export interface RowAction {
  label: string; // 英文原文，界面里用 t()
  domain: string;
  service: string;
}

/**
 * “全部实体”列表里每行的按钮。门锁、窗帘也在这里，
 * 开锁和开门时服务端会要求再次验证。
 */
export function rowActions(entityId: string, state?: EntityState): RowAction[] {
  const domain = domainOf(entityId);
  const tap = tapAction(entityId, state);
  if (tap.kind === "toggle")
    return [
      {
        label: tap.service === "turn_on" ? "Turn on" : "Turn off",
        domain,
        service: tap.service,
      },
    ];
  if (tap.kind === "run")
    return [
      { label: tap.service === "press" ? "Press" : "Run", domain, service: tap.service },
    ];
  if (domain === "lock")
    return state?.state === "locked"
      ? [{ label: "Unlock", domain, service: "unlock" }]
      : [{ label: "Lock", domain, service: "lock" }];
  if (domain === "cover")
    return [
      { label: "Open", domain, service: "open_cover" },
      { label: "Close", domain, service: "close_cover" },
    ];
  return [];
}

const stateLabels: Record<string, string> = {
  on: "On",
  off: "Off",
  locked: "Locked",
  unlocked: "Unlocked",
  locking: "Locking",
  unlocking: "Unlocking",
  open: "Opened",
  opening: "Opening",
  closed: "Closed",
  closing: "Closing",
  unavailable: "Unavailable",
  unknown: "Unknown",
  home: "At home",
  not_home: "Away",
  playing: "Playing",
  paused: "Paused",
  idle: "Idle",
  heat: "Heating",
  cool: "Cooling",
  auto: "Auto",
  armed_away: "Armed away",
  armed_home: "Armed home",
  disarmed: "Disarmed",
  triggered: "Triggered",
};

/** 常见状态的英文原文，交给 t() 翻译；其余原样返回（数值会带单位）。 */
export function stateLabel(state: EntityState): string {
  // 场景和按钮的状态是上次运行的时间，对用户没有意义。
  const domain = state.entityId.split(".")[0];
  if ((domain === "scene" || domain === "button") && !isUnavailable(state)) return "Tap to run";
  return stateLabels[state.state] ?? formatState(state);
}

const domainLabels: Record<string, string> = {
  light: "Lights",
  switch: "Switches",
  fan: "Fans",
  climate: "Climate",
  cover: "Covers",
  lock: "Locks",
  sensor: "Sensors",
  binary_sensor: "Binary sensors",
  scene: "Scenes",
  script: "Scripts",
  automation: "Automations",
  media_player: "Media players",
  button: "Buttons",
  person: "People",
  input_boolean: "Toggles",
};

export function domainLabel(domain: string): string {
  return domainLabels[domain] ?? domain;
}

export function isOn(state: EntityState): boolean {
  return ["on", "open", "playing", "unlocked", "home", "heat", "cool"].includes(
    state.state,
  );
}

export function isUnavailable(state?: EntityState): boolean {
  return !state || state.state === "unavailable" || state.state === "unknown";
}

/** 传感器读数带单位，例如 "21.5 °C"。 */
export function formatState(state: EntityState): string {
  const unit = state.attributes.unit_of_measurement;
  if (typeof unit === "string" && unit) return `${state.state} ${unit}`;
  return state.state;
}

export function matches(state: EntityState, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    state.entityId.toLowerCase().includes(q) ||
    displayName(state.entityId, state).toLowerCase().includes(q)
  );
}

export interface DomainGroup<T extends EntityState> {
  domain: string;
  items: T[];
}

/** 常用的域排在前面，其余按字母。 */
const domainOrder = [
  "light",
  "switch",
  "fan",
  "climate",
  "cover",
  "lock",
  "sensor",
  "binary_sensor",
  "scene",
  "script",
];

export function groupByDomain<T extends EntityState>(
  states: T[],
): DomainGroup<T>[] {
  const map = new Map<string, T[]>();
  for (const s of states) {
    const d = domainOf(s.entityId);
    const list = map.get(d);
    if (list) list.push(s);
    else map.set(d, [s]);
  }
  const rank = (d: string) => {
    const i = domainOrder.indexOf(d);
    return i < 0 ? domainOrder.length : i;
  };
  return [...map.entries()]
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
    .map(([domain, items]) => ({
      domain,
      items: [...items].sort((a, b) =>
        displayName(a.entityId, a).localeCompare(displayName(b.entityId, b)),
      ),
    }));
}

/** 用新状态替换列表里的旧状态，没有就追加。 */
export function applyStateChange<T extends EntityState>(
  list: T[],
  state: T,
): T[] {
  let found = false;
  const next = list.map((s) => {
    if (s.entityId !== state.entityId) return s;
    found = true;
    return state;
  });
  return found ? next : [...next, state];
}

/** 收藏列表里把第 index 项往前或往后挪一格。 */
export function move<T>(list: T[], index: number, delta: -1 | 1): T[] {
  const target = index + delta;
  if (target < 0 || target >= list.length) return list;
  const next = [...list];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}
