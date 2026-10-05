import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// B98：字体跟着前端打包，不从 Google Fonts 加载（国内连不上）
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "./styles/index.css";
import App from "./app/App";
import { installGlobalErrorHandlers } from "./lib/errors";

installGlobalErrorHandlers();

const root = document.getElementById("root");
if (!root) throw new Error("Missing application root");
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
