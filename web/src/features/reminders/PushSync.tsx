import { useEffect } from "react";
import { syncPush } from "./push";

/** 登录后同步一次浏览器推送的订阅（B52）。失败不提示，下次打开再试。 */
export default function PushSync() {
  useEffect(() => {
    syncPush().catch(() => {});
  }, []);
  return null;
}
