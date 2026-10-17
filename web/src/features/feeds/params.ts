import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";

/** 输入停下来 250 毫秒后才去搜。 */
export function useDebounced(value: string, ms = 250): string {
  const [out, setOut] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setOut(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return out;
}

/** 改地址栏里的几个参数，空值删掉。筛选用 replace，打开详情记一条历史，手机上能按返回关掉。 */
export function useParamSetter() {
  const [, setParams] = useSearchParams();
  return (changes: Record<string, string | null>, replace = true) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [key, value] of Object.entries(changes)) {
          if (value) next.set(key, value);
          else next.delete(key);
        }
        return next;
      },
      { replace },
    );
}
