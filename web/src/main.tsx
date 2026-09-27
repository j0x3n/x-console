// 演示数据（临时）：必须在其他模块之前加载。确认界面后删掉这一行和 src/demo。
import "./demo";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/index.css";
import App from "./app/App";

const root = document.getElementById("root");
if (!root) throw new Error("Missing application root");
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
