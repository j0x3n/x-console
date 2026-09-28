import { useQueryClient } from "@tanstack/react-query";

export function useInvalidate(key: readonly unknown[]) {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: key });
}
