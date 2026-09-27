/*
 * 演示开关。打开时（默认），所有页面都用内存里的假数据，页面上的改动只改假数据，
 * 不碰服务器上的真实数据。关掉后，已经有后端的模块回到真实数据；
 * 还没有后端的功能里，AI 助手和自动化仍然用假数据。云盘只在开关打开时用假数据。
 */
const KEY = "xc.demo.full";

function read() {
  try {
    return localStorage.getItem(KEY) !== "off";
  } catch {
    return true;
  }
}

export const demoFull = read();

export function setDemoFull(on: boolean) {
  try {
    localStorage.setItem(KEY, on ? "on" : "off");
  } catch {
    /* 存不了就算了 */
  }
  location.reload();
}

/** 演示打开时，这些路径开头的请求都不发到服务器。 */
export const FULL_PREFIXES = [
  "/projects",
  "/issues",
  "/notes",
  "/reminders",
  "/habits",
  "/workouts",
  "/calendars",
  "/calendar",
  "/briefs",
  "/weather",
  "/focus",
  "/hosts",
  "/ssh-hosts",
  "/alert-rules",
  "/alerts",
  "/scripts",
  "/script-runs",
  "/monitors",
  "/subscriptions",
  "/coding",
  "/ha",
  "/github",
  "/linear",
  "/notifications",
  "/audit",
  "/agents",
];

/** 只读、不涉及个人数据的接口，演示模式下也走真实服务器。 */
const PASS_THROUGH = ["/weather/places"];

export function isFullPath(path: string) {
  if (PASS_THROUGH.some((p) => path === p || path.startsWith(`${p}?`)))
    return false;
  return FULL_PREFIXES.some(
    (p) => path === p || path.startsWith(`${p}/`) || path.startsWith(`${p}?`),
  );
}
