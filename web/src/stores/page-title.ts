import { useEffect, type ReactNode } from "react";
import { create } from "zustand";

/*
 * 页面没有大标题，左上角显示模块名（路由的 handle.title），点它回到模块首页。
 * 详情页（比如某个项目、某台服务器）用 PageHeading 的 title 传具体名称，
 * 这里记下来，左上角显示成“项目 / XC 项目”。中间还有一层时用 parents，
 * 比如 Issue 页是“项目 / XC / XC-1”。
 * status 只给有设备状态的页面用（服务器详情、本机），左上角显示一个状态点。
 * subtitle 是这一页的概况灰字，显示在左上角标题后面（B20）。
 */
export interface Crumb {
  label: string;
  to: string;
}
export type PageStatus = "ok" | "warn" | "danger";

interface PageTitleState {
  title: string;
  parents: Crumb[];
  status: PageStatus | null;
  statusLabel: string;
  subtitle: ReactNode;
  setTitle: (title: string, parents?: Crumb[]) => void;
  setStatus: (status: PageStatus | null, label?: string) => void;
  setSubtitle: (subtitle: ReactNode) => void;
}

export const usePageTitle = create<PageTitleState>()((set) => ({
  title: "",
  parents: [],
  status: null,
  statusLabel: "",
  subtitle: null,
  setTitle: (title, parents = []) => set({ title, parents }),
  setStatus: (status, statusLabel = "") => set({ status, statusLabel }),
  setSubtitle: (subtitle) => set({ subtitle }),
}));

/** 不用 PageHeading 的详情页直接调它。离开页面时清空。 */
export function usePageCrumb(title: string, parents?: Crumb[]) {
  const setTitle = usePageTitle((s) => s.setTitle);
  const key = JSON.stringify(parents ?? []);
  useEffect(() => {
    setTitle(title, parents);
    return () => setTitle("");
    // parents 按内容比较
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title, key, setTitle]);
}

/** 左上角的设备状态点。离开页面时去掉。 */
export function usePageStatus(status: PageStatus | null, label = "") {
  const setStatus = usePageTitle((s) => s.setStatus);
  useEffect(() => {
    setStatus(status, label);
    return () => setStatus(null);
  }, [status, label, setStatus]);
}
