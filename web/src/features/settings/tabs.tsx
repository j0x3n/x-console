import type { ComponentType } from "react";
import DevicesTab from "./DevicesTab";
import AuditTab from "./AuditTab";
import SecurityTab from "./SecurityTab";
import NotificationsTab from "../reminders/NotificationsTab";
import HASettingsTab from "../home/HASettingsTab";
import GitSettingsTab from "../github/GitSettingsTab";
import LinearSettingsTab from "../github/LinearSettingsTab";
import BriefSettingsTab from "../calendar/BriefSettingsTab";
import StorageTab from "../storage/StorageTab";
import BackupTab from "../backup/BackupTab";
import AssistantSettingsTab from "../assistant/AssistantSettingsTab";
import ErrorsTab from "./ErrorsTab";
import AiUsageTab from "../assistant/AiUsageTab";
import RemoteAccessTab from "../mcp/RemoteAccessTab";
import MailSettingsTab from "../mail/MailSettingsTab";
import RouterSettingsTab from "../router/RouterSettingsTab";

export interface SettingsTab {
  id: string; // 路径段，/settings/<id>
  label: string; // 英文原文，中文在 i18n
  component: ComponentType;
}

// 设置页的标签。模块有自己的配置页时在这里加一行，
// 例如 { id: "notifications", label: "Notifications", component: NotificationSettings }。
export const settingsTabs: SettingsTab[] = [
  { id: "security", label: "Security", component: SecurityTab },
  { id: "devices", label: "Devices & agents", component: DevicesTab },
  { id: "audit", label: "Audit log", component: AuditTab },
  { id: "notifications", label: "Notifications", component: NotificationsTab },
  { id: "homeassistant", label: "Home Assistant", component: HASettingsTab },
  { id: "git", label: "Git & repositories", component: GitSettingsTab },
  { id: "linear", label: "Linear", component: LinearSettingsTab },
  { id: "brief", label: "Daily brief", component: BriefSettingsTab },
  { id: "storage", label: "Storage", component: StorageTab },
  { id: "backup", label: "Backup", component: BackupTab },
  { id: "assistant", label: "AI", component: AssistantSettingsTab },
  { id: "ai-usage", label: "AI usage", component: AiUsageTab },
  { id: "errors", label: "Recent errors", component: ErrorsTab },
  { id: "remote", label: "Remote access", component: RemoteAccessTab },
  { id: "mail", label: "Mail", component: MailSettingsTab },
  { id: "router", label: "Router", component: RouterSettingsTab },
];
