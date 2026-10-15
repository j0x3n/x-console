import { useEffect } from "react";
import { meApi } from "./api";
import { everyMinuteOnPhone } from "./phone";

/** B164：手机上的面板打开着时告诉服务端，2 分钟内算“在用手机”。 */
export default function PhoneSeen() {
  useEffect(
    () =>
      everyMinuteOnPhone(() => {
        void meApi
          .POST("/me/seen", { body: { device: "phone" } })
          .catch(() => {});
      }),
    [],
  );
  return null;
}
