import type { ComponentType } from "react";
import GeneralTab from "./GeneralTab";
import DevicesTab from "./DevicesTab";
import AuditTab from "./AuditTab";

export interface SettingsTab {
  id: string; // 路径段，/settings/<id>
  label: string; // 英文原文，中文在 i18n
  component: ComponentType;
}

// 设置页的标签。模块有自己的配置页时在这里加一行，
// 例如 { id: "notifications", label: "Notifications", component: NotificationSettings }。
export const settingsTabs: SettingsTab[] = [
  { id: "general", label: "General", component: GeneralTab },
  { id: "devices", label: "Devices & agents", component: DevicesTab },
  { id: "audit", label: "Audit log", component: AuditTab },
];
