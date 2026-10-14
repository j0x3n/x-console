import { lazy } from "react";
import type { RouteObject } from "react-router";
import { ChefHat, FileInput, Globe, Plus } from "lucide-react";
import { registerCommands } from "../../lib/commands";
import { registerNavAction } from "../../lib/navBadges";
import { registerNavChildren } from "../../lib/navChildren";
import NavChildren from "./NavChildren";
import "./i18n";
import "./recipes.css";

registerNavAction("/recipes", {
  icon: Plus,
  label: "New recipe",
  run: (navigate) => navigate("/recipes/new"),
});

registerNavChildren("/recipes", NavChildren);

// 页面按需加载（B6），主包里只留路由、命令和样式。
const RecipesPage = lazy(() => import("./RecipesPage"));
const RecipePage = lazy(() => import("./RecipePage"));
const RecipeEditPage = lazy(() => import("./RecipeEditPage"));
const CookPage = lazy(() => import("./CookPage"));
const MealsPage = lazy(() => import("./MealsPage"));
const DiscoverPage = lazy(() => import("./DiscoverPage"));

registerCommands([
  {
    id: "recipes.open",
    title: "打开菜谱",
    group: "菜谱",
    icon: ChefHat,
    keywords: "recipes cooking cook menu 菜谱 做菜 菜单 烹饪",
    run: ({ navigate }) => navigate("/recipes"),
  },
  {
    id: "recipes.new",
    title: "新建菜谱",
    group: "菜谱",
    icon: Plus,
    keywords: "recipe new 新建 菜谱",
    run: ({ navigate }) => navigate("/recipes/new"),
  },
  {
    id: "recipes.import",
    title: "从文字导入菜谱",
    group: "菜谱",
    icon: FileInput,
    keywords: "recipe import ai 导入 菜谱 教程",
    run: ({ navigate }) => navigate("/recipes?import=1"),
  },
  {
    id: "recipes.discover",
    title: "联网找新菜谱",
    group: "菜谱",
    icon: Globe,
    keywords: "recipe discover search web 找 菜谱 联网 新菜",
    run: ({ navigate }) => navigate("/recipes/discover"),
  },
]);

export const routes: RouteObject[] = [
  { path: "recipes", element: <RecipesPage />, handle: { title: "Recipes" } },
  {
    path: "recipes/meals",
    element: <MealsPage />,
    handle: { title: "Recipes" },
  },
  {
    path: "recipes/discover",
    element: <DiscoverPage />,
    handle: { title: "Recipes" },
  },
  {
    path: "recipes/new",
    element: <RecipeEditPage />,
    handle: { title: "Recipes" },
  },
  {
    path: "recipes/:id",
    element: <RecipePage />,
    handle: { title: "Recipes" },
  },
  {
    path: "recipes/:id/edit",
    element: <RecipeEditPage />,
    handle: { title: "Recipes" },
  },
  {
    path: "recipes/:id/cook",
    element: <CookPage />,
    handle: { title: "Recipes" },
  },
];
