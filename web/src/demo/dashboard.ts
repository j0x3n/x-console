import { json, route } from "./router";

/* 今日页布局：存在 localStorage，刷新后还在。 */
const KEY = "xc.demo.layout";

route("GET", "/dashboard/layout", () => {
  try {
    return json(JSON.parse(localStorage.getItem(KEY) ?? '{"cards":[]}'));
  } catch {
    return json({ cards: [] });
  }
});
route("PUT", "/dashboard/layout", ({ body }) => {
  try {
    localStorage.setItem(KEY, JSON.stringify(body));
  } catch {
    /* 存不了就算了 */
  }
  return json(body);
});
