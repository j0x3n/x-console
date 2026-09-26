import { useCallback } from "react";
import { useSearchParams } from "react-router";
import { errorMessage } from "../../../api/client";
import { toast } from "../../../hooks/useToast";

/** 地址栏里的一个查询参数，比如 ?monitor=3 打开详情，?new=1 打开新建弹窗。 */
export function useParam(key: string): [string | null, (value: string | null) => void] {
  const [params, setParams] = useSearchParams();
  const set = useCallback(
    (value: string | null) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (value === null) next.delete(key);
          else next.set(key, value);
          return next;
        },
        { replace: true },
      );
    },
    [key, setParams],
  );
  return [params.get(key), set];
}

/** 数字 id 参数。 */
export function useIdParam(key: string): [number | null, (id: number | null) => void] {
  const [value, set] = useParam(key);
  const id = value && /^\d+$/.test(value) ? Number(value) : null;
  return [id, (next) => set(next === null ? null : String(next))];
}

export function showError(err: unknown) {
  toast({ message: errorMessage(err), tone: "error" });
}

/** 浏览器 datetime 转 “9月27日 14:05” 这样的短时间。 */
export function shortDateTime(value: string, language: string): string {
  return new Date(value).toLocaleString(language === "zh" ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/** 日期，比如 2026-10-11 显示成 “2026年10月11日”。 */
export function longDate(value: string, language: string): string {
  const d = value.length === 10 ? new Date(value + "T00:00:00") : new Date(value);
  return d.toLocaleDateString(language === "zh" ? "zh-CN" : "en", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}
