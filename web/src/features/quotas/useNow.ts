import { useEffect, useState } from "react";

/** 当前时间，每隔 interval 毫秒更新一次，倒计时用。 */
export function useNow(interval = 30_000) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), interval);
    return () => clearInterval(id);
  }, [interval]);
  return now;
}
