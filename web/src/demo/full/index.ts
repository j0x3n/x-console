/*
 * 演示打开时，已经有后端的模块也用假数据（见 ../mode.ts）。
 * 顺序有关系：前面的处理函数不管时交给后面的。
 */
import { demoFull } from "../mode";
import * as projects from "./projects";
import * as notes from "./notes";
import * as reminders from "./reminders";
import * as habits from "./habits";
import * as calendar from "./calendar";
import * as hosts from "./hosts";
import * as monitoring from "./monitoring";
import * as coding from "./coding";
import * as integrations from "./integrations";

if (demoFull)
  for (const m of [
    projects,
    notes,
    reminders,
    habits,
    calendar,
    hosts,
    monitoring,
    coding,
    integrations,
  ])
    m.register();
