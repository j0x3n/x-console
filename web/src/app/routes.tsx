import type { RouteObject } from "react-router";
import { routes as overview } from "../features/overview/routes";
import { routes as projects } from "../features/projects/routes";
import { routes as coding } from "../features/coding/routes";
import { routes as notes } from "../features/notes/routes";
import { routes as mail } from "../features/mail/routes";
import { routes as drive } from "../features/drive/routes";
import { routes as reminders } from "../features/reminders/routes";
import { routes as habits } from "../features/habits/routes";
import { routes as calendar } from "../features/calendar/routes";
import { routes as servers } from "../features/servers/routes";
import { routes as pc } from "../features/pc/routes";
import { routes as monitoring } from "../features/monitoring/routes";
import { routes as home } from "../features/home/routes";
import { routes as router } from "../features/router/routes";
import { routes as quotas } from "../features/quotas/routes";
import { routes as documents } from "../features/documents/routes";
import { routes as journal } from "../features/journal/routes";
import { routes as contacts } from "../features/contacts/routes";
import { routes as music } from "../features/music/routes";
import { routes as credentials } from "../features/credentials/routes";
import { routes as aiconfig } from "../features/aiconfig/routes";
import { routes as readlater } from "../features/readlater/routes";
import { routes as screentime } from "../features/screentime/routes";
import { routes as automations } from "../features/automations/routes";
import { routes as github } from "../features/github/routes";
import { routes as assistant } from "../features/assistant/routes";
import { routes as settings } from "../features/settings/routes";

// 所有模块的路由。每个模块只改自己的 features/<模块>/routes.tsx。
export const moduleRoutes: RouteObject[] = [
  ...overview,
  ...projects,
  ...coding,
  ...notes,
  ...mail,
  ...drive,
  ...reminders,
  ...habits,
  ...calendar,
  ...servers,
  ...pc,
  ...monitoring,
  ...home,
  ...router,
  ...quotas,
  ...documents,
  ...screentime,
  ...readlater,
  ...journal,
  ...contacts,
  ...music,
  ...credentials,
  ...aiconfig,
  ...automations,
  ...github,
  ...assistant,
  ...settings,
];
