import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { unwrap } from "../../api/client";
import { useInvalidate } from "../../api/useInvalidate";
import { withElevation } from "../../auth/elevation";
import type { components } from "../../api/gen/habits";
import { habitsApi, habitKeys } from "./api";

type Schemas = components["schemas"];
export type PersonalLibrary = Schemas["PersonalLibrary"];
export type PersonalProfile = Schemas["PersonalProfile"];
export type PersonalProfileInput = Schemas["PersonalProfileInput"];
export type PersonalDay = Schemas["PersonalDay"];
export type PersonalDayInput = Schemas["PersonalDayInput"];
export type LibraryExercise = Schemas["LibraryExercise"];
export type LibrarySession = Schemas["LibrarySession"];
export type LibrarySessionItem = Schemas["LibrarySessionItem"];
export type LibraryHabit = Schemas["LibraryHabit"];
export type PersonalBackup = Schemas["PersonalBackup"];

export function usePersonalLibrary() {
  return useQuery({
    queryKey: ["habits-library"],
    queryFn: () => unwrap(habitsApi.GET("/habits/library")),
    staleTime: Infinity,
  });
}
export function usePersonalProfile() {
  return useQuery({
    queryKey: ["habits", "personal", "profile"],
    queryFn: () => unwrap(habitsApi.GET("/habits/personal/profile")),
  });
}
export function useSavePersonalProfile() {
  const invalidate = useInvalidate(habitKeys.all);
  return useMutation({
    mutationFn: (body: PersonalProfileInput) =>
      unwrap(habitsApi.PUT("/habits/personal/profile", { body })),
    onSuccess: invalidate,
  });
}
export function useActivatePersonalHabits() {
  const invalidate = useInvalidate(habitKeys.all);
  return useMutation({
    mutationFn: (ids: string[]) =>
      unwrap(habitsApi.POST("/habits/library/activate", { body: { ids } })),
    onSuccess: invalidate,
  });
}
export function usePersonalDay(date: string) {
  return useQuery({
    queryKey: ["habits", "personal", "day", date],
    queryFn: () =>
      unwrap(
        habitsApi.GET("/habits/personal/days/{date}", {
          params: { path: { date } },
        }),
      ),
  });
}
export function usePersonalDays() {
  return useQuery({
    queryKey: ["habits", "personal", "days"],
    queryFn: () =>
      unwrap(
        habitsApi.GET("/habits/personal/days", {
          params: { query: { days: 366 } },
        }),
      ),
  });
}
export function useSavePersonalDay(date: string) {
  const invalidate = useInvalidate(habitKeys.all);
  const client = useQueryClient();
  const key = ["habits", "personal", "day", date];
  return useMutation({
    mutationFn: (body: PersonalDayInput) =>
      unwrap(
        habitsApi.PATCH("/habits/personal/days/{date}", {
          params: { path: { date } },
          body,
        }),
      ),
    onMutate: async (body) => {
      await client.cancelQueries({ queryKey: key });
      const previous = client.getQueryData<PersonalDay>(key);
      if (previous)
        client.setQueryData(key, {
          ...previous,
          ...body,
          sets: { ...previous.sets, ...body.sets },
        });
      return { previous };
    },
    onError: (_error, _body, context) => {
      if (context?.previous) client.setQueryData(key, context.previous);
    },
    onSuccess: (day) => {
      client.setQueryData(key, day);
      return invalidate();
    },
  });
}
export function useCheckPersonalHabit(date: string) {
  const invalidate = useInvalidate(habitKeys.all);
  const client = useQueryClient();
  const key = ["habits", "personal", "day", date];
  return useMutation({
    mutationFn: (body: { id: string; done: boolean }) =>
      unwrap(
        habitsApi.POST("/habits/personal/days/{date}/check", {
          params: { path: { date } },
          body,
        }),
      ),
    onMutate: async (body) => {
      await client.cancelQueries({ queryKey: key });
      const previous = client.getQueryData<PersonalDay>(key);
      if (previous)
        client.setQueryData(key, {
          ...previous,
          checks: { ...previous.checks, [body.id]: body.done },
        });
      return { previous };
    },
    onError: (_error, _body, context) => {
      if (context?.previous) client.setQueryData(key, context.previous);
    },
    onSuccess: (day) => {
      client.setQueryData(key, day);
      return invalidate();
    },
  });
}
export function useLogPersonalWorkout(date: string) {
  const invalidate = useInvalidate(habitKeys.all);
  return useMutation({
    mutationFn: (body: {
      items: Schemas["WorkoutItem"][];
      durationMinutes: number;
      note: string;
    }) =>
      unwrap(
        habitsApi.POST("/habits/personal/days/{date}/workout", {
          params: { path: { date } },
          body,
        }),
      ),
    onSuccess: invalidate,
  });
}
export function useImportPersonalBackup() {
  const invalidate = useInvalidate(habitKeys.all);
  return useMutation({
    mutationFn: (body: PersonalBackup) =>
      unwrap(habitsApi.POST("/habits/personal/backup", { body })),
    onSuccess: invalidate,
  });
}
export async function exportPersonalBackup() {
  return unwrap(habitsApi.GET("/habits/personal/backup"));
}

export type BodyPushStatus = Schemas["BodyPushStatus"];
export type BodyPushToken = Schemas["BodyPushToken"];
const bodyPushKey = ["habits", "personal", "body-push"];
export function useBodyPush() {
  return useQuery({
    queryKey: bodyPushKey,
    queryFn: () => unwrap(habitsApi.GET("/habits/body/push")),
  });
}
export function useCreateBodyPushToken() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(habitsApi.POST("/habits/body/push/token"))),
    onSuccess: () => client.invalidateQueries({ queryKey: bodyPushKey }),
  });
}
export function useDeleteBodyPush() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(habitsApi.DELETE("/habits/body/push")),
    onSuccess: () => client.invalidateQueries({ queryKey: bodyPushKey }),
  });
}
