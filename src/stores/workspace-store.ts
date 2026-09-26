import { create } from "zustand";
import { initialDecisions, activitySeed } from "../data/dashboard";
import { resolveUpdate } from "./update";
import type {
  ActivityRow,
  Decision,
  DecisionAction,
  DecisionDetails,
  DecisionEntry,
  DecisionOutcomes,
  DelegatedTask,
  HiredAgent,
  Setter,
  WorkStatuses,
} from "../types/domain";

export interface WorkspaceState {
  decisions: Decision[];
  decisionOutcomes: DecisionOutcomes;
  decisionHistory: DecisionEntry[];
  exitingDecisions: Decision[];
  activity: ActivityRow[];
  tasks: DelegatedTask[];
  workStatuses: WorkStatuses;
  hireRequest: number;
  hiredAgents: HiredAgent[];
  activeDecision: number;
  setTasks: Setter<DelegatedTask[]>;
  setWorkStatuses: Setter<WorkStatuses>;
  setHireRequest: Setter<number>;
  setHiredAgents: Setter<HiredAgent[]>;
  setActiveDecision: Setter<number>;
  applyDecision: (
    item: Decision,
    action: DecisionAction,
    details?: DecisionDetails,
  ) => DecisionEntry | null;
  undoDecision: (key: string) => DecisionEntry | null;
  reset: () => void;
}

let sequence = 0;
const exitTimers = new Map<number, ReturnType<typeof setTimeout>>();
const initialState = () => ({
  decisions: initialDecisions.map((item) => ({ ...item })),
  decisionOutcomes: {} as DecisionOutcomes,
  decisionHistory: [] as DecisionEntry[],
  exitingDecisions: [] as Decision[],
  activity: activitySeed.map((row) => [...row] as ActivityRow),
  tasks: [] as DelegatedTask[],
  workStatuses: {} as WorkStatuses,
  hireRequest: 0,
  hiredAgents: [] as HiredAgent[],
  activeDecision: 0,
});

export const useWorkspaceStore = create<WorkspaceState>()((set, get) => ({
  ...initialState(),
  setTasks: (update) =>
    set((state) => ({ tasks: resolveUpdate(update, state.tasks) })),
  setWorkStatuses: (update) =>
    set((state) => ({
      workStatuses: resolveUpdate(update, state.workStatuses),
    })),
  setHireRequest: (update) =>
    set((state) => ({ hireRequest: resolveUpdate(update, state.hireRequest) })),
  setHiredAgents: (update) =>
    set((state) => ({ hiredAgents: resolveUpdate(update, state.hiredAgents) })),
  setActiveDecision: (update) =>
    set((state) => ({
      activeDecision: resolveUpdate(update, state.activeDecision),
    })),
  applyDecision: (item, action, details = {}) => {
    if (!get().decisions.some((row) => row.id === item.id)) return null;
    const entry: DecisionEntry = {
      key: `${item.id}-${++sequence}`,
      item,
      action,
      ...details,
    };
    set((state) => ({
      decisions: state.decisions.filter((row) => row.id !== item.id),
      exitingDecisions: [...state.exitingDecisions, item],
      decisionOutcomes: { ...state.decisionOutcomes, [item.id]: action },
      decisionHistory: [...state.decisionHistory, entry],
      activity: [
        [
          item.agent,
          action === "approved"
            ? "completed your approved action"
            : "closed a decision",
          item.company,
          item.title,
          "now",
          entry.key,
        ],
        ...state.activity,
      ],
      activeDecision: 0,
    }));
    clearTimeout(exitTimers.get(item.id));
    exitTimers.set(
      item.id,
      setTimeout(() => {
        set((state) => ({
          exitingDecisions: state.exitingDecisions.filter(
            (row) => row.id !== item.id,
          ),
        }));
        exitTimers.delete(item.id);
      }, 260),
    );
    return entry;
  },
  undoDecision: (key) => {
    const entry = get().decisionHistory.find((row) => row.key === key);
    if (!entry) return null;
    clearTimeout(exitTimers.get(entry.item.id));
    exitTimers.delete(entry.item.id);
    set((state) => {
      const outcomes = { ...state.decisionOutcomes };
      delete outcomes[entry.item.id];
      return {
        decisionHistory: state.decisionHistory.filter((row) => row.key !== key),
        exitingDecisions: state.exitingDecisions.filter(
          (row) => row.id !== entry.item.id,
        ),
        decisions: state.decisions.some((item) => item.id === entry.item.id)
          ? state.decisions
          : [...state.decisions, entry.item].sort((a, b) => a.id - b.id),
        decisionOutcomes: outcomes,
        activity: state.activity.filter((row) => row[5] !== key),
        activeDecision: 0,
      };
    });
    return entry;
  },
  reset: () => {
    exitTimers.forEach((timer) => clearTimeout(timer));
    exitTimers.clear();
    set(initialState());
  },
}));
