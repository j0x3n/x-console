import type { ComponentType } from "react";
import { useT } from "../../contexts/LanguageContext";
import {
  Bell,
  Bookmark,
  DatabaseBackup,
  GitBranch,
  HardDrive,
  House,
  KeyRound,
  Mail,
  MonitorSmartphone,
  Music,
  Newspaper,
  Plug,
  Router,
  ScrollText,
  Sparkles,
  Sunrise,
  UserRound,
  Wrench,
  type LucideIcon,
} from "lucide-react";
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
import MaintenanceTab from "../maintenance/MaintenanceTab";
import XSettingsTab from "../readlater/XSettingsTab";
import FeedsSettingsTab from "../feeds/FeedsSettingsTab";
import MusicSettingsTab from "../music/MusicSettingsTab";
import MeSettingsTab from "../me/MeSettingsTab";

export interface SettingsTab {
  id: string; // 地址里的 ?settings=<id>，旧地址 /settings/<id> 也能用
  label: string; // 英文原文，中文在 i18n
  icon: LucideIcon;
  group: string; // settingsGroups 里的 id
  component: ComponentType;
}

export interface SettingsGroup {
  id: string;
  label: string;
}

// 设置弹窗左边的分组（B180）。
export const settingsGroups: SettingsGroup[] = [
  { id: "general", label: "General" },
  { id: "security", label: "Security" },
  { id: "ai", label: "AI" },
  { id: "integrations", label: "Integrations" },
  { id: "data", label: "Data" },
  { id: "system", label: "System" },
];

// 几个小页并进相关的页，上下排着。
function AiWithUsage() {
  const t = useT();
  return (
    <div className="settings-stack">
      <AssistantSettingsTab />
      <div id="ai-usage">
        <h3 className="settings-subhead">{t("Usage")}</h3>
        <AiUsageTab />
      </div>
    </div>
  );
}

function GitWithLinear() {
  return (
    <div className="settings-stack">
      <GitSettingsTab />
      <LinearSettingsTab />
    </div>
  );
}

function MaintenanceWithErrors() {
  return (
    <div className="settings-stack">
      <MaintenanceTab />
      <ErrorsTab />
    </div>
  );
}

// 设置弹窗的每一项。模块有自己的配置时在这里加一行，放进合适的分组。
// 只和一个功能有关的设置放在那个功能的页面里，不放这里。
export const settingsTabs: SettingsTab[] = [
  {
    id: "me",
    label: "My status",
    icon: UserRound,
    group: "general",
    component: MeSettingsTab,
  },
  {
    id: "notifications",
    label: "Notifications",
    icon: Bell,
    group: "general",
    component: NotificationsTab,
  },
  {
    id: "brief",
    label: "Daily brief",
    icon: Sunrise,
    group: "general",
    component: BriefSettingsTab,
  },
  {
    id: "security",
    label: "Sign-in & verification",
    icon: KeyRound,
    group: "security",
    component: SecurityTab,
  },
  {
    id: "devices",
    label: "Devices & agents",
    icon: MonitorSmartphone,
    group: "security",
    component: DevicesTab,
  },
  {
    id: "remote",
    label: "Remote access",
    icon: Plug,
    group: "security",
    component: RemoteAccessTab,
  },
  {
    id: "audit",
    label: "Audit log",
    icon: ScrollText,
    group: "security",
    component: AuditTab,
  },
  {
    id: "assistant",
    label: "AI",
    icon: Sparkles,
    group: "ai",
    component: AiWithUsage,
  },
  {
    id: "git",
    label: "Git & repositories",
    icon: GitBranch,
    group: "integrations",
    component: GitWithLinear,
  },
  {
    id: "homeassistant",
    label: "Home Assistant",
    icon: House,
    group: "integrations",
    component: HASettingsTab,
  },
  {
    id: "mail",
    label: "Mail",
    icon: Mail,
    group: "integrations",
    component: MailSettingsTab,
  },
  {
    id: "router",
    label: "Router",
    icon: Router,
    group: "integrations",
    component: RouterSettingsTab,
  },
  {
    id: "readlater",
    label: "Read later",
    icon: Bookmark,
    group: "integrations",
    component: XSettingsTab,
  },
  {
    id: "feeds",
    label: "Feeds",
    icon: Newspaper,
    group: "integrations",
    component: FeedsSettingsTab,
  },
  {
    id: "music",
    label: "Music",
    icon: Music,
    group: "integrations",
    component: MusicSettingsTab,
  },
  {
    id: "storage",
    label: "Storage",
    icon: HardDrive,
    group: "data",
    component: StorageTab,
  },
  {
    id: "backup",
    label: "Backup",
    icon: DatabaseBackup,
    group: "data",
    component: BackupTab,
  },
  {
    id: "maintenance",
    label: "Maintenance",
    icon: Wrench,
    group: "system",
    component: MaintenanceWithErrors,
  },
];
