import { lazy, type ComponentType, type LazyExoticComponent } from "react";
import type { HostDetail } from "./api";
import OverviewTab from "./components/OverviewTab";
import ProcessesTab from "./components/ProcessesTab";
import ServicesTab from "./components/ServicesTab";
import FilesTab from "./components/FilesTab";
import AlertsTab from "./components/AlertRules";

// xterm.js 比较大，打开终端标签时才加载。
const TerminalTab = lazy(() => import("./components/TerminalTab"));

type TabComponent = ComponentType<{ host: HostDetail }>;

export interface HostTab {
  id: string; // 路径段，/servers/<id>/<tab>
  label: string; // 英文原文，中文在 i18n
  /** 机器需要上报这个能力才显示 */
  cap?: string;
  component: TabComponent | LazyExoticComponent<TabComponent>;
}

// 机器详情页的标签。新功能在这里加一行，例如
// { id: "docker", label: "Containers", cap: "docker", component: DockerTab }。
export const hostTabs: HostTab[] = [
  { id: "overview", label: "Overview", component: OverviewTab },
  {
    id: "processes",
    label: "Processes",
    cap: "processes",
    component: ProcessesTab,
  },
  {
    id: "services",
    label: "Services",
    cap: "services",
    component: ServicesTab,
  },
  { id: "terminal", label: "Terminal", cap: "pty", component: TerminalTab },
  { id: "files", label: "Files", cap: "files", component: FilesTab },
  {
    id: "docker",
    label: "Containers",
    cap: "docker",
    component: lazy(() => import("../monitoring/DockerTab")),
  },
  { id: "alerts", label: "Alerts", component: AlertsTab },
];

/** 这台机器能用的标签。 */
export function tabsFor(host: Pick<HostDetail, "capabilities">): HostTab[] {
  return hostTabs.filter(
    (tab) => !tab.cap || host.capabilities.includes(tab.cap),
  );
}
