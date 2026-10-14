import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invalidateOn } from "../../api/events";
import { ApiError, createApi, unwrap } from "../../api/client";
import type { components, paths } from "../../api/gen/recipes";

/* 菜谱和做菜计时（B209）。菜谱是结构化的数据，计时器存在服务端，到点会推送。 */
export const recipesApi = createApi<paths>();

type S = components["schemas"];
export type Recipe = S["Recipe"];
export type RecipeInput = S["RecipeInput"];
export type RecipePatch = S["RecipePatch"];
export type RecipeIngredient = S["RecipeIngredient"];
export type RecipeStep = S["RecipeStep"];
export type RecipeTimer = S["RecipeTimer"];
export type RecipeTagCount = S["RecipeTagCount"];
export type CookTimer = S["CookTimer"];
export type CookTimerInput = S["CookTimerInput"];
export type CookTimerPatch = S["CookTimerPatch"];

export const recipeKeys = {
  all: ["recipes"] as const,
  list: ["recipes", "list"] as const,
  recipe: (id: number) => ["recipes", "recipe", id] as const,
  timers: ["recipes", "timers"] as const,
};

invalidateOn("recipe.", recipeKeys.all);
invalidateOn("cook_timer.", recipeKeys.timers);

/** 全部菜谱。标签筛选和搜索在页面里做，数据量很小。 */
export function useRecipes() {
  return useQuery({
    queryKey: recipeKeys.list,
    queryFn: () => unwrap(recipesApi.GET("/recipes")),
    retry: false,
  });
}

export function useRecipe(id: number) {
  return useQuery({
    queryKey: recipeKeys.recipe(id),
    queryFn: () =>
      unwrap(
        recipesApi.GET("/recipes/{recipeId}", {
          params: { path: { recipeId: id } },
        }),
      ),
    enabled: id > 0,
    retry: false,
  });
}

function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: recipeKeys.all });
}

export function useCreateRecipe() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: RecipeInput) =>
      unwrap(recipesApi.POST("/recipes", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateRecipe() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: RecipePatch }) =>
      unwrap(
        recipesApi.PATCH("/recipes/{recipeId}", {
          params: { path: { recipeId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteRecipe() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        recipesApi.DELETE("/recipes/{recipeId}", {
          params: { path: { recipeId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

/** 用 AI 把教程原文整理成菜谱草稿，不保存。 */
export function useParseRecipe() {
  return useMutation({
    mutationFn: (body: { text: string; sourceNoteId?: number | null }) =>
      unwrap(recipesApi.POST("/recipes/parse", { body })),
  });
}

/**
 * 没关掉的计时器。全局浮条和做菜页共用这一个查询。
 * 后端没上线（404、501）时当作没有计时器，不报错。
 */
export function useCookTimers() {
  return useQuery({
    queryKey: recipeKeys.timers,
    queryFn: async (): Promise<CookTimer[]> => {
      try {
        const out = await unwrap(recipesApi.GET("/cook-timers"));
        return out.items;
      } catch (error) {
        if (
          error instanceof ApiError &&
          (error.status === 404 || error.status === 501)
        )
          return [];
        throw error;
      }
    },
    meta: { silentError: true },
  });
}

function useRefreshTimers() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: recipeKeys.timers });
}

export function useStartTimer() {
  const refresh = useRefreshTimers();
  return useMutation({
    mutationFn: (body: CookTimerInput) =>
      unwrap(recipesApi.POST("/cook-timers", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateTimer() {
  const refresh = useRefreshTimers();
  return useMutation({
    mutationFn: (v: { id: number; body: CookTimerPatch }) =>
      unwrap(
        recipesApi.PATCH("/cook-timers/{timerId}", {
          params: { path: { timerId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteTimer() {
  const refresh = useRefreshTimers();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        recipesApi.DELETE("/cook-timers/{timerId}", {
          params: { path: { timerId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}
