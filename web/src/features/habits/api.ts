import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import { invalidateOn } from "../../api/events";
import type { components, paths } from "../../api/gen/habits";

export const habitsApi = createApi<paths>();

type Schemas = components["schemas"];
export type Habit = Schemas["Habit"];
export type HabitInput = Schemas["HabitInput"];
export type HabitPatch = Schemas["HabitPatch"];
export type HabitToday = Schemas["HabitToday"];
export type HabitLog = Schemas["HabitLog"];
export type HabitStats = Schemas["HabitStats"];
export type HabitDay = Schemas["HabitDay"];
export type RemindMode = Schemas["RemindMode"];
export type HabitKind = Schemas["HabitKind"];
export type WorkoutItem = Schemas["WorkoutItem"];
export type WorkoutPlan = Schemas["WorkoutPlan"];
export type WorkoutLog = Schemas["WorkoutLog"];
export type WorkoutLogInput = Schemas["WorkoutLogInput"];
export type WorkoutSettings = Schemas["WorkoutSettings"];

export const habitKeys = {
  all: ["habits"] as const,
  today: ["habits", "today"] as const,
  list: ["habits", "list"] as const,
  stats: (id: number, days: number) => ["habits", "stats", id, days] as const,
  plans: ["habits", "workouts", "plans"] as const,
  logs: ["habits", "workouts", "logs"] as const,
  workoutSettings: ["habits", "workouts", "settings"] as const,
};

invalidateOn("habit.", habitKeys.all);
invalidateOn("workout.", ["habits", "workouts"]);

function useInvalidateAll() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: habitKeys.all });
}

export function useHabitsToday() {
  return useQuery({
    queryKey: habitKeys.today,
    queryFn: () => unwrap(habitsApi.GET("/habits/today")),
  });
}

export function useHabitList() {
  return useQuery({
    queryKey: habitKeys.list,
    queryFn: () =>
      unwrap(
        habitsApi.GET("/habits", { params: { query: { archived: true } } }),
      ),
  });
}

export function useHabitStats(id: number | null, days = 30) {
  return useQuery({
    queryKey: habitKeys.stats(id ?? 0, days),
    enabled: id !== null,
    queryFn: () =>
      unwrap(
        habitsApi.GET("/habits/{habitId}/stats", {
          params: { path: { habitId: id ?? 0 }, query: { days } },
        }),
      ),
  });
}

export function useCheckin() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: ({ id, amount }: { id: number; amount?: number }) =>
      unwrap(
        habitsApi.POST("/habits/{habitId}/checkin", {
          params: { path: { habitId: id } },
          body: amount === undefined ? {} : { amount },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useUndoCheckin() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (logId: number) =>
      unwrap(
        habitsApi.DELETE("/habits/logs/{logId}", {
          params: { path: { logId } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useCreateHabit() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (body: HabitInput) =>
      unwrap(habitsApi.POST("/habits", { body })),
    onSuccess: invalidate,
  });
}

export function useUpdateHabit() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: HabitPatch }) =>
      unwrap(
        habitsApi.PATCH("/habits/{habitId}", {
          params: { path: { habitId: id } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useDeleteHabit() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        habitsApi.DELETE("/habits/{habitId}", {
          params: { path: { habitId: id } },
        }),
      ),
    onSuccess: invalidate,
  });
}

// ---- 健身 ----

export function useWorkoutPlans() {
  return useQuery({
    queryKey: habitKeys.plans,
    queryFn: () => unwrap(habitsApi.GET("/workouts/plans")),
  });
}

export function useSavePlans() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (plans: WorkoutPlan[]) =>
      unwrap(habitsApi.PUT("/workouts/plans", { body: plans })),
    onSuccess: invalidate,
  });
}

export function useWorkoutLogs(days = 30) {
  return useQuery({
    queryKey: [...habitKeys.logs, days],
    queryFn: () =>
      unwrap(habitsApi.GET("/workouts/logs", { params: { query: { days } } })),
  });
}

export function useLogWorkout() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (body: WorkoutLogInput) =>
      unwrap(habitsApi.POST("/workouts/logs", { body })),
    onSuccess: invalidate,
  });
}

export function useDeleteWorkoutLog() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (logId: number) =>
      unwrap(
        habitsApi.DELETE("/workouts/logs/{logId}", {
          params: { path: { logId } },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useWorkoutSettings() {
  return useQuery({
    queryKey: habitKeys.workoutSettings,
    queryFn: () => unwrap(habitsApi.GET("/workouts/settings")),
  });
}

export function useSaveWorkoutSettings() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (body: WorkoutSettings) =>
      unwrap(habitsApi.PUT("/workouts/settings", { body })),
    onSuccess: invalidate,
  });
}
