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
export type RecipeNutrition = S["RecipeNutrition"];
export type DietGoal = S["DietGoal"];
export type RecipeSettings = S["RecipeSettings"];
export type RecipeSettingsInput = S["RecipeSettingsInput"];
export type RecipeRecommendation = S["RecipeRecommendation"];
export type RecipeRecommendations = S["RecipeRecommendations"];
export type Meal = S["Meal"];
export type MealInput = S["MealInput"];
export type MealPatch = S["MealPatch"];
export type MealSlot = S["MealSlot"];
export type DayWeight = S["DayWeight"];
export type RecipeCandidate = S["RecipeCandidate"];
export type RecipeTasteProfile = S["RecipeTasteProfile"];

export const recipeKeys = {
  all: ["recipes"] as const,
  list: ["recipes", "list"] as const,
  recipe: (id: number) => ["recipes", "recipe", id] as const,
  timers: ["recipes", "timers"] as const,
  recommend: ["recipes", "recommend"] as const,
  settings: ["recipes", "settings"] as const,
  meals: (from: string, to: string) => ["recipes", "meals", from, to] as const,
  candidates: ["recipes", "candidates"] as const,
};

invalidateOn("recipe.", recipeKeys.all);
invalidateOn("cook_timer.", recipeKeys.timers);
// B210：饮食记录变了，菜谱的统计和推荐也跟着变
invalidateOn("meal.", recipeKeys.all);
invalidateOn("recipe_candidate.", recipeKeys.candidates);

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

// ---- B210：营养分类、推荐、设置、饮食记录 ----

/** 推荐。后端没上线时当作没有推荐，不报错。 */
export function useRecommend() {
  return useQuery({
    queryKey: recipeKeys.recommend,
    queryFn: async (): Promise<RecipeRecommendations | null> => {
      try {
        return await unwrap(recipesApi.GET("/recipes/recommend"));
      } catch (error) {
        if (
          error instanceof ApiError &&
          (error.status === 404 || error.status === 501)
        )
          return null;
        throw error;
      }
    },
    meta: { silentError: true },
  });
}

export function useRecipeSettings() {
  return useQuery({
    queryKey: recipeKeys.settings,
    queryFn: () => unwrap(recipesApi.GET("/recipes/settings")),
    retry: false,
  });
}

export function useSaveRecipeSettings() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: RecipeSettingsInput) =>
      unwrap(recipesApi.PUT("/recipes/settings", { body })),
    onSuccess: refresh,
  });
}

/** 用 AI 估一份菜谱的营养分类，不保存。 */
export function useEstimateNutrition() {
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        recipesApi.POST("/recipes/{recipeId}/nutrition", {
          params: { path: { recipeId: id } },
        }),
      ),
  });
}

/** 用 AI 给还没有营养分类的菜谱补上，直接保存。 */
export function useFillNutrition() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: () => unwrap(recipesApi.POST("/recipes/nutrition/fill")),
    onSuccess: refresh,
  });
}

/** 饮食记录。不传日期时是服务器时区的最近 30 天（含今天）。 */
export function useMeals(from = "", to = "") {
  return useQuery({
    queryKey: recipeKeys.meals(from, to),
    queryFn: () =>
      unwrap(
        recipesApi.GET("/meals", {
          params: { query: { from: from || undefined, to: to || undefined } },
        }),
      ),
    retry: false,
  });
}

export function useCreateMeal() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (body: MealInput) =>
      unwrap(recipesApi.POST("/meals", { body })),
    onSuccess: refresh,
  });
}

export function useUpdateMeal() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: { id: number; body: MealPatch }) =>
      unwrap(
        recipesApi.PATCH("/meals/{mealId}", {
          params: { path: { mealId: v.id } },
          body: v.body,
        }),
      ),
    onSuccess: refresh,
  });
}

export function useDeleteMeal() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(
        recipesApi.DELETE("/meals/{mealId}", {
          params: { path: { mealId: id } },
        }),
      ),
    onSuccess: refresh,
  });
}

// ---- B211：联网找新菜谱 ----

export function useCandidates() {
  return useQuery({
    queryKey: recipeKeys.candidates,
    queryFn: () => unwrap(recipesApi.GET("/recipe-candidates")),
    retry: false,
  });
}

/** 联网找菜，可能要跑一分钟。 */
export function useDiscover() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { hint?: string; count?: number }) =>
      unwrap(recipesApi.POST("/recipes/discover", { body })),
    onSettled: () => qc.invalidateQueries({ queryKey: recipeKeys.candidates }),
  });
}

export function useUpdateCandidate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: {
      id: number;
      status: "accepted" | "dismissed";
      recipeId?: number | null;
    }) =>
      unwrap(
        recipesApi.PATCH("/recipe-candidates/{candidateId}", {
          params: { path: { candidateId: v.id } },
          body: { status: v.status, recipeId: v.recipeId ?? null },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: recipeKeys.candidates }),
  });
}
