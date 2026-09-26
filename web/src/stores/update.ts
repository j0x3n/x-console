import type { SetStateAction } from "react";

export function resolveUpdate<T>(update: SetStateAction<T>, current: T): T {
  return typeof update === "function"
    ? (update as (previous: T) => T)(current)
    : update;
}
