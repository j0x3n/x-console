import { lazy, Suspense, useEffect, useRef } from "react";
import { useLocation, useNavigate, useNavigationType } from "react-router";
import { SETTINGS_PARAM, rememberPage } from "./nav";

// 弹窗里有所有设置页，按需加载。
const SettingsDialog = lazy(() => import("./SettingsDialog"));

/** 地址里有 ?settings= 时在当前页面上打开设置弹窗（B180）。 */
export default function SettingsHost() {
  const location = useLocation();
  const navigate = useNavigate();
  const navigationType = useNavigationType();
  const params = new URLSearchParams(location.search);
  const tab = params.get(SETTINGS_PARAM);
  const open = tab !== null;
  // 从页面里点开的（多记了一条历史）关闭时后退，浏览器的返回键也能关。
  // 直接打开设置地址时前面没有页面可退，关闭时只去掉参数。
  const seenPage = useRef(false);
  const wasOpen = useRef(false);
  const pushed = useRef(false);
  const pageBeforeOpen = useRef("");

  useEffect(() => {
    rememberPage(location.pathname, location.search);
  }, [location.pathname, location.search]);

  useEffect(() => {
    const plain = new URLSearchParams(location.search);
    plain.delete(SETTINGS_PARAM);
    plain.delete("section");
    const address = location.pathname + (plain.size ? `?${plain}` : "");
    if (open && !wasOpen.current)
      pushed.current =
        seenPage.current &&
        navigationType === "PUSH" &&
        pageBeforeOpen.current === address;
    if (!open) pageBeforeOpen.current = address;
    wasOpen.current = open;
    if (!location.pathname.startsWith("/settings")) seenPage.current = true;
  }, [open, navigationType, location.pathname, location.search]);

  if (!open) return null;

  const withTab = (id: string | null) => {
    const next = new URLSearchParams(location.search);
    if (id === null) next.delete(SETTINGS_PARAM);
    else next.set(SETTINGS_PARAM, id);
    const search = next.toString();
    return { pathname: location.pathname, search: search ? `?${search}` : "" };
  };
  const select = (id: string) => navigate(withTab(id), { replace: true });
  const close = () => {
    if (pushed.current) navigate(-1);
    else navigate(withTab(null), { replace: true });
  };

  return (
    <Suspense fallback={null}>
      <SettingsDialog tab={tab} onSelect={select} onClose={close} />
    </Suspense>
  );
}
