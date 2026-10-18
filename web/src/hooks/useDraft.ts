import {
  useCallback,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { sessionMap, type SessionMap } from "./sessionMap";

/*
 * 切换模块时保留写了一半的输入和展开、选中的状态（B249）。
 * 和 useState 一样用，多一个 key。存法同 B80：内存加 sessionStorage，关掉标签页就忘掉。
 * key 要带上对象的 id，比如 `projects.comment.${issueId}`。key 是空字符串时不存，等于 useState。
 */

const drafts = sessionMap<string>("xc.drafts", { max: 200 });
const kept = sessionMap<unknown>("xc.kept", { max: 300 });

function useStored<T>(
  map: SessionMap<T>,
  key: string,
  initial: T,
  isEmpty: (value: T) => boolean,
): [T, Dispatch<SetStateAction<T>>] {
  const read = () => (key && map.has(key) ? (map.get(key) as T) : initial);
  const [value, setValue] = useState<T>(read);
  const [shownKey, setShownKey] = useState(key);
  const current = useRef(value);
  // 换了对象（key 变了）时读那个对象存的值
  let shown = value;
  if (shownKey !== key) {
    shown = read();
    setShownKey(key);
    setValue(shown);
  }
  current.current = shown;
  const set = useCallback<Dispatch<SetStateAction<T>>>(
    (update) => {
      const next =
        typeof update === "function"
          ? (update as (prev: T) => T)(current.current)
          : update;
      current.current = next;
      if (key) {
        if (isEmpty(next)) map.delete(key);
        else map.set(key, next);
      }
      setValue(next);
    },
    // isEmpty 是固定的函数
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [map, key],
  );
  return [shown, set];
}

const emptyText = (v: string) => v === "";

/**
 * 写了一半的输入。提交成功后调第三个返回值 clear() 清掉。
 *
 *   const [body, setBody, clearBody] = useDraft(`projects.comment.${issueId}`);
 */
export function useDraft(
  key: string,
  initial = "",
): [string, Dispatch<SetStateAction<string>>, () => void] {
  const [value, set] = useStored(drafts, key, initial, emptyText);
  const clear = useCallback(() => {
    if (key) drafts.delete(key);
    set("");
  }, [key, set]);
  return [value, set, clear];
}

const never = () => false;

/** 展开的行、切到的子标签这类状态。不清，按 key 一直记到关掉标签页。 */
export function useKeptState<T>(
  key: string,
  initial: T,
): [T, Dispatch<SetStateAction<T>>] {
  return useStored(kept as SessionMap<T>, key, initial, never);
}
