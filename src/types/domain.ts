import type { Dispatch, SetStateAction } from "react";

export type Language = "zh" | "en";
export type ThemeMode = "system" | "dark" | "light";
export type Text = string | number | null | undefined;
export type Translate = (text: Text) => string;
export type Localize = (en: Text, zh?: Text) => string;
export type Navigate = (name: string | undefined, anchor?: string) => void;
export type Setter<T> = Dispatch<SetStateAction<T>>;
export type StringMap = Record<string, string>;
export type Bilingual = [en: string, zh: string];
export type AgentMode = "Suggest only" | "Ask first" | "Autopilot";
export type Stage = "Discovery" | "Evaluation" | "Proposal" | "Negotiation";
export type WorkStatus =
  "Queued" | "Running" | "Needs your call" | "Done today";
export type DecisionKind = "draft" | "health" | "stage";
export type DecisionAction = "approved" | "reassigned" | "dismissed" | "kept";
export type WorkStatuses = Record<string, WorkStatus>;
export type DecisionOutcomes = Record<number, DecisionAction>;

export interface AgentDefinition {
  name: string;
  color: string;
  mark: string;
  status: string;
}
export interface Decision {
  id: number;
  agent: string;
  verb: string;
  time: string;
  title: string;
  company: string;
  initials: string;
  companyColor: string;
  detail: string;
  subtitle?: string;
  receipts: string;
  confidence: string;
  kind: DecisionKind;
}
export interface DecisionDetails {
  assignee?: string;
}
export interface DecisionEntry extends DecisionDetails {
  key: string;
  item: Decision;
  action: DecisionAction;
}
export type DecisionHandler = (
  item: Decision,
  action: DecisionAction,
  details?: DecisionDetails,
) => void;
export type ActivityRow = [
  agent: string,
  action: string,
  company: string,
  receipt: string,
  time: string,
  actionKey?: string,
];
export interface DelegatedTask {
  title: string;
  assignee: string;
  company: string;
  sources: string[];
  askFirst: boolean;
}
export interface WorkItem {
  id: string;
  title: string;
  titleZh: string;
  assignee: string;
  company: string;
  status: WorkStatus;
  type: string;
  progress?: number;
  detail: string;
  detailZh: string;
  decision?: Decision;
  isExiting?: boolean;
  cost?: string;
  confidence?: string;
}
export interface RecordData {
  name: string;
  company?: string;
  initials?: string;
  color?: string;
  stage?: Stage;
  owner?: string;
  value?: number;
  health?: number;
  change?: number;
  agent?: string;
  touch: string;
  next?: string;
  nextZh?: string;
  signal?: string;
  signalZh?: string;
  segment?: string;
  role?: string;
  warmth?: string;
  knownBy?: string;
}
export interface CompanyRecord extends RecordData {
  initials: string;
  color: string;
  stage: Stage;
  owner: string;
  value: number;
  health: number;
  change: number;
  agent: string;
  next: string;
  nextZh: string;
  signal: string;
  signalZh: string;
  segment: string;
}
export interface PersonRecord extends RecordData {
  company: string;
  role: string;
  warmth: string;
  knownBy: string;
}
export interface DealRecord {
  id: number;
  company: string;
  title: string;
  titleZh: string;
  titleEn?: string;
  value: number;
  stage: Stage;
  forecast: string;
  close: string;
  closeZh: string;
  owner: string;
}
export type AccountTimelineEntry = [
  day: string,
  actor: string,
  title: string,
  detail: string,
  time: string,
  receipt: string,
  kind?: string,
];
export type AccountSignal = [
  en: string,
  zh: string,
  tone: string,
  agent: string,
  receipt: string,
  time: string,
];
export interface CompanyDetail {
  type: string;
  domain: string;
  industry: string;
  size: string;
  location: string;
  nextDue?: string;
  nextClose?: string;
  nextCloseValue?: number;
  bestCase?: string;
  briefAgent?: string;
  briefTime?: string;
  brief?: string[];
  briefZh?: string[];
  sources?: string[];
  signals?: AccountSignal[];
  peopleTitles?: StringMap;
  timeline?: AccountTimelineEntry[];
}
export interface AgentStats {
  runs: number;
  overnight: number;
  success: number;
  spend: number;
  budget: number;
  median: string;
  bars: number[];
  mode: AgentMode;
  current: string;
  currentZh: string;
  record: string;
}
export interface AgentChart {
  color: string;
  retried: number;
  range: number[];
}
export interface HireTemplate {
  id: string;
  name: string;
  job: Bilingual;
  duties: Bilingual[];
  sources: string[];
  runs: number;
  cost: number;
  calls: number;
}
export interface HiredAgent {
  id: string;
  template: string;
  name: string;
  sources: string[];
  mode: AgentMode;
  budget: number;
}
export interface CrewRun {
  id: string;
  time: string;
  agent: string;
  run: string;
  runZh?: string;
  record: string;
  receipt: string;
  receiptZh?: string;
  took: string;
  cost: number | null;
  decision?: number;
  offline: boolean;
}
export interface ToastMessage {
  message: string;
  subtitle?: string;
  agent?: string;
  undoKey?: string | null;
  onUndo?: () => void;
}
export interface ToastNotification extends ToastMessage {
  id: number;
  closing?: boolean;
}
export type Notify = (
  message: string | ToastMessage,
  undoKey?: string | null,
) => void;
export type OpenDelegate = (
  name?: string | React.MouseEvent<HTMLButtonElement>,
) => void;

export type CompanySeedRow = [
  name: string,
  initials: string,
  color: string,
  stage: Stage,
  owner: string,
  value: number,
  health: number,
  change: number,
  agent: string,
  touch: string,
  next: string,
  nextZh: string,
  signal: string,
  signalZh: string,
  segment: string,
];
export type PeopleSeedRow = [
  name: string,
  company: string,
  role: string,
  warmth: string,
];
export type DealSeedRow = [
  company: string,
  title: string,
  value: number,
  stage: Stage,
  forecast: string,
  close: string,
];
export type WorkSeedRow = [
  title: string,
  titleZh: string,
  assignee: string,
  company: string,
  status: WorkStatus,
  type: string,
  progress: number,
  detail: string,
  detailZh: string,
];
export type CrewRunSeedRow = [
  time: string,
  agent: string,
  run: string,
  runZh: string,
  record: string,
  receipt: string,
  receiptZh: string,
  took: string,
  cost: number,
  decision?: number,
];
