import { registerZh } from "../../lib/i18n";

registerZh({
  // B139 个人资料问答
  "Ask my data": "资料问答",
  "Ask settings": "资料问答设置",
  "Ask off title": "资料问答还没开启",
  "Ask off body":
    "开启后，笔记、稍后读正文、日记和复习卡片会被切成小段，发给你配置的 AI 供应商转成向量。",
  "Ask turn on": "去开启",
  "Ask question": "问题",
  "Ask question hint": "比如：上次 Caddy 证书续不上是怎么解决的",
  "Ask button": "提问",
  "Ask thinking": "正在查找…",
  "Ask answer": "回答",
  "Ask sources": "出处",
  "Ask source open": "打开",
  "Ask answer check":
    "回答由 AI 根据下面的片段写出，可能有错。重要的内容请打开出处核对。",
  "Ask no answer": "没有可用的 AI 写回答，下面是找到的片段。",
  "Ask index": "索引",
  "Ask index docs": "篇文档",
  "Ask index chunks": "段",
  "Ask index pending": "段等下次同步",
  "Ask last sync": "上次同步",
  "Ask never synced": "还没同步过",
  "Ask sync now": "立即同步",
  "Ask syncing": "正在同步…",
  "Ask no provider": "还没有配置 AI 供应商，不能建索引。",
  "Ask last error": "上次同步出错",
  "Ask source notes": "笔记",
  "Ask source readlater": "稍后读",
  "Ask source journal": "日记",
  "Ask source cards": "复习卡片",
  "Ask enable": "开启资料问答",
  "Ask enable hint":
    "开启后才会向 AI 供应商发送内容。关闭后不再发送，已建的索引保留。",
  "Ask model": "嵌入模型",
  "Ask model hint":
    "不填时 Google 用 gemini-embedding-001，其他用 text-embedding-3-small",
  "Ask embed provider": "嵌入用的供应商",
  "Ask embed provider fast": "和快速 AI 一样",
  "Ask embed provider gone": "已删除的供应商",
  "Ask embed provider hint":
    "要用 Google：先在 AI 设置里加一个供应商，地址填 https://generativelanguage.googleapis.com/v1beta/openai，再在这里选它。换供应商或模型后索引会全部重建。",
  "Ask sources to index": "要建索引的内容",
  "Ask clear index": "清空索引",
  "Ask clear title": "清空全部索引？",
  "Ask clear body":
    "向量和片段会被删除，笔记等原文不受影响。再次同步会重新建。",
  "Ask no results": "还没有结果。",
});
