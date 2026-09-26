import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWorkspaceStore } from "./workspace-store";
import { activitySeed } from "../data/dashboard";

describe("workspace store", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useWorkspaceStore.getState().reset();
  });
  afterEach(() => {
    useWorkspaceStore.getState().reset();
    vi.useRealTimers();
  });

  it("updates the pending queue, outcome, history and activity in one notification", () => {
    const store = useWorkspaceStore.getState();
    const decision = store.decisions[2];
    const observer = vi.fn();
    const unsubscribe = useWorkspaceStore.subscribe(observer);
    const entry = store.applyDecision(decision, "approved");
    const state = useWorkspaceStore.getState();

    expect(observer).toHaveBeenCalledTimes(1);
    expect(state.decisions.map((item) => item.id)).toEqual([1, 2]);
    expect(state.decisionOutcomes[3]).toBe("approved");
    expect(state.decisionHistory).toEqual([entry]);
    expect(state.activity[0][5]).toBe(entry?.key);
    expect(state.exitingDecisions).toEqual([decision]);
    vi.advanceTimersByTime(260);
    expect(useWorkspaceStore.getState().exitingDecisions).toEqual([]);
    unsubscribe();
  });

  it("ignores duplicate approval without creating duplicate history or activity", () => {
    const decision = useWorkspaceStore.getState().decisions[0];
    useWorkspaceStore.getState().applyDecision(decision, "approved");
    expect(
      useWorkspaceStore.getState().applyDecision(decision, "approved"),
    ).toBeNull();
    expect(useWorkspaceStore.getState().decisionHistory).toHaveLength(1);
    expect(useWorkspaceStore.getState().activity).toHaveLength(
      activitySeed.length + 1,
    );
  });

  it("undoes only the selected decision and preserves other actions", () => {
    const [first, second] = useWorkspaceStore.getState().decisions;
    const entry = useWorkspaceStore.getState().applyDecision(first, "approved");
    useWorkspaceStore
      .getState()
      .applyDecision(second, "reassigned", { assignee: "Ruth Adler" });
    expect(entry).not.toBeNull();
    useWorkspaceStore.getState().undoDecision(entry!.key);
    const state = useWorkspaceStore.getState();
    expect(state.decisions.map((item) => item.id)).toEqual([1, 3]);
    expect(state.decisionOutcomes[1]).toBeUndefined();
    expect(state.decisionOutcomes[2]).toBe("reassigned");
    expect(state.decisionHistory[0].assignee).toBe("Ruth Adler");
    expect(state.activity.some((row) => row[5] === entry!.key)).toBe(false);
  });

  it("cancels an old exit timer when an undone decision is approved again", () => {
    const decision = useWorkspaceStore.getState().decisions[0];
    const entry = useWorkspaceStore
      .getState()
      .applyDecision(decision, "approved");
    vi.advanceTimersByTime(200);
    useWorkspaceStore.getState().undoDecision(entry!.key);
    useWorkspaceStore.getState().applyDecision(decision, "approved");
    vi.advanceTimersByTime(60);
    expect(useWorkspaceStore.getState().exitingDecisions).toHaveLength(1);
    vi.advanceTimersByTime(200);
    expect(useWorkspaceStore.getState().exitingDecisions).toHaveLength(0);
  });

  it("supports functional updates for delegated tasks, statuses and hires", () => {
    const store = useWorkspaceStore.getState();
    store.setTasks((tasks) => [
      ...tasks,
      {
        title: "Research account",
        assignee: "Scout",
        company: "Halcyon Robotics",
        sources: ["Web"],
        askFirst: true,
      },
    ]);
    store.setWorkStatuses((statuses) => ({ ...statuses, "work-0": "Running" }));
    store.setHireRequest((value) => value + 1);
    store.setHiredAgents((agents) => [
      ...agents,
      {
        id: "hire-1",
        template: "relay",
        name: "Relay",
        sources: ["Email"],
        mode: "Suggest only",
        budget: 4,
      },
    ]);
    const state = useWorkspaceStore.getState();
    expect(state.tasks[0].title).toBe("Research account");
    expect(state.workStatuses["work-0"]).toBe("Running");
    expect(state.hireRequest).toBe(1);
    expect(state.hiredAgents[0].name).toBe("Relay");
  });

  it("treats repeated or unknown undo as a no-op", () => {
    const entry = useWorkspaceStore
      .getState()
      .applyDecision(useWorkspaceStore.getState().decisions[0], "dismissed");
    useWorkspaceStore.getState().undoDecision(entry!.key);
    const snapshot = useWorkspaceStore.getState();
    expect(snapshot.undoDecision(entry!.key)).toBeNull();
    expect(snapshot.undoDecision("unknown")).toBeNull();
    expect(useWorkspaceStore.getState()).toBe(snapshot);
  });
});
