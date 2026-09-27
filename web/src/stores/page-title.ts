import { create } from "zustand";

/*
 * 页面没有大标题，左上角显示模块名（路由的 handle.title）。
 * 详情页（比如某个项目、某台服务器）用 PageHeading 的 title 传具体名称，
 * 这里记下来，左上角显示成“项目 / XC 项目”。
 */
interface PageTitleState {
  title: string;
  setTitle: (title: string) => void;
}

export const usePageTitle = create<PageTitleState>()((set) => ({
  title: "",
  setTitle: (title) => set({ title }),
}));
