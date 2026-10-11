import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/skills";

/* 技能练习记录（B138）。 */
export const skillsApi = createApi<paths>();

type S = components["schemas"];
export type Skill = S["Skill"];
export type SkillInput = S["SkillInput"];
export type SkillPatch = S["SkillPatch"];
export type SkillSession = S["SkillSession"];
export type SkillSessionInput = S["SkillSessionInput"];
export type SkillSessionPatch = S["SkillSessionPatch"];

export const skillKeys = {
  all: ["skills"] as const,
  list: (archived: boolean) => ["skills", "list", archived] as const,
  sessions: (id: number, stuck: boolean) =>
    ["skills", "sessions", id, stuck] as const,
};

export function useSkills(archived: boolean) {
  return useQuery({
    queryKey: skillKeys.list(archived),
    queryFn: () =>
      unwrap(skillsApi.GET("/skills", { params: { query: { archived } } })),
    retry: false,
  });
}

export function useSkillSessions(id: number, stuck: boolean, enabled: boolean) {
  return useQuery({
    queryKey: skillKeys.sessions(id, stuck),
    queryFn: () =>
      unwrap(
        skillsApi.GET("/skills/{skillId}/sessions", {
          params: { path: { skillId: id }, query: { stuck, limit: 200 } },
        }),
      ),
    enabled,
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: skillKeys.all });
}

export function useCreateSkill() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: SkillInput) =>
      unwrap(skillsApi.POST("/skills", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateSkill() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: SkillPatch }) =>
      unwrap(
        skillsApi.PATCH("/skills/{skillId}", {
          params: { path: { skillId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteSkill() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        skillsApi.DELETE("/skills/{skillId}", {
          params: { path: { skillId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

export function useLogSession() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { skillId: number; body: SkillSessionInput }) =>
      unwrap(
        skillsApi.POST("/skills/{skillId}/sessions", {
          params: { path: { skillId: v.skillId } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useUpdateSession() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: SkillSessionPatch }) =>
      unwrap(
        skillsApi.PATCH("/skill-sessions/{sessionId}", {
          params: { path: { sessionId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteSession() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        skillsApi.DELETE("/skill-sessions/{sessionId}", {
          params: { path: { sessionId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}
