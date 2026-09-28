import type { Dispatch, SetStateAction } from "react";

// 只放跨模块共用的界面类型。接口数据的类型来自 api/gen/<模块>.ts。
export type Language = "zh" | "en";
export type ThemeMode = "system" | "dark" | "light";
/** 主题色（B22），只在白天生效。 */
export type Accent =
  | "ember"
  | "violet"
  | "mint"
  | "ocean"
  | "rose"
  | "graphite";
export type Text = string | number | null | undefined;
export type Translate = (text: Text) => string;
export type Setter<T> = Dispatch<SetStateAction<T>>;

export interface ToastMessage {
  message: string;
  subtitle?: string;
  tone?: "ok" | "error";
  onUndo?: () => void;
}
export interface ToastNotification extends ToastMessage {
  id: number;
  closing?: boolean;
}
export type Notify = (message: string | ToastMessage) => void;
