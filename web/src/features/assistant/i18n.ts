import { registerZh } from "../../lib/i18n";

registerZh({
  AI: "AI",
  "New conversation": "新对话",
  "No conversations yet": "还没有对话",
  Expand: "放大",
  Shrink: "缩小",
  "Ask or tell me what to do": "问点什么，或者让我做点什么",
  Message: "消息",
  Send: "发送",
  Stop: "停止",
  Running: "运行中",
  "Waiting for you": "等你确认",
  Done: "已完成",
  Failed: "失败",
  Rejected: "已拒绝",
  Approve: "确认",
  Reject: "拒绝",
  "No parameters": "没有参数",
  Model: "模型",
  "Model ID": "模型 ID",
  "Other model": "其他模型",
  "Confirm every write": "所有写操作都先确认",
  "Key saved": "已保存 Key",
  "No key yet": "还没填 Key",
  "Remove API key": "删除 Key",
  "API key removed": "已删除 API Key",
  "leave empty to keep": "留空表示不改",
  "Remove the API key?": "删除 API Key？",
  "AI stops working until you add a key again.": "重新填写之前 AI 不能使用。",
  // B32：供应商和模型
  models: "个模型",
  Providers: "供应商",
  "Add provider": "添加供应商",
  "Edit provider": "编辑供应商",
  "Delete provider": "删除供应商",
  "Models that use it are cleared from the settings.":
    "用到它的模型设置会被清空。",
  "Add an OpenAI compatible provider, such as OpenAI, DeepSeek, OpenRouter or a local Ollama.":
    "添加一个兼容 OpenAI 接口的供应商。比如 OpenAI、DeepSeek、OpenRouter，或者本机的 Ollama。",
  "Any service with an OpenAI compatible API works, including local models.":
    "兼容 OpenAI 接口的服务都可以用，本机模型也行。",
  Presets: "常用",
  "Starts with http:// or https://": "以 http:// 或 https:// 开头",
  "Stored encrypted on the server. Local models need no key.":
    "加密保存在服务器上。本机模型不用填。",
  Refreshed: "更新于",
  "No key": "没填 Key",
  Test: "测试",
  "Refresh model list": "刷新模型列表",
  Models: "模型",
  "AI now uses OpenAI compatible APIs. The old Anthropic settings are no longer used. Add a provider and choose models again.":
    "AI 现在改用 OpenAI 兼容接口。旧的 Anthropic 设置不再使用。请添加供应商，再选一次模型。",
  "Agent model": "Agent 模型",
  "No tool calls": "不支持工具调用",
  "Used by the AI panel, automations and the server Agent tab. It must support tool calls.":
    "AI 面板、自动化和服务器的 Agent 标签都用它。必须支持工具调用。",
  "Reasoning effort": "思考程度",
  "This model does not support reasoning.": "这个模型不支持思考。",
  "This API rejected the reasoning effort, so it is sent without it.":
    "这个接口不接受思考程度参数，现在调用时不带它。",
  "Fast model": "快速模型",
  "Same as the Agent model": "和 Agent 模型一样",
  "Used for note titles and tags, and to polish the daily brief.":
    "用来给笔记起标题、加标签，还有润色每日简报。",
  "models.dev has no spec for this model. Fill it in yourself.":
    "models.dev 上没有这个模型的规格，请自己填。",
  "Context length": "上下文长度",
  "Supports tool calls": "支持工具调用",
  "Supports reasoning": "支持思考",
  "Spec unknown": "规格未知",
  "Tool calls": "工具调用",
  Reasoning: "思考",
  "Image input": "图片输入",
  "Choose a model": "选择模型",
  "Search models": "搜索模型",
  "Fill in spec": "填写规格",
  "No models yet. Add a provider and refresh its models.":
    "还没有模型。先添加供应商，再刷新它的模型列表。",
  "No matching models": "没有匹配的模型",
  "Prices are US dollars per million tokens, from models.dev.":
    "价格是每百万 token 的美元价，来自 models.dev。",
  "Usage this month": "本月用量",
  "Usage is not available yet.": "用量统计还没上线。",
  "No calls this month.": "这个月还没有调用。",
  Calls: "调用次数",
  "Input tokens": "输入 token",
  "Output tokens": "输出 token",
  Cost: "费用",
  "Estimated from models.dev prices. Your bill may differ.":
    "按 models.dev 的价格估算，和实际账单可能不一样。",
  // B32：笔记
  "Title notes automatically": "自动起标题",
  "Fills in an empty title 3 seconds after saving. Hidden notes are never sent.":
    "保存 3 秒后，给没标题的笔记补上标题。隐藏的笔记不会发给 AI。",
  "Tag notes automatically": "自动加标签",
  "Picks up to 3 of your existing tags.": "从你已有的标签里最多挑 3 个。",
  "Suggested tags": "建议的标签",
  "Show as suggestions first": "先显示成建议",
  "Add them directly": "直接加上",
  "Show all": "显示全部",
  // B39
  "API type": "接口类型",
  "Most services support Chat Completions. Choose Responses if the service asks for it.":
    "大多数服务支持 Chat Completions。服务要求用 Responses 时再选它。",
  "Fast model reasoning effort": "快速模型思考程度",
  "Attach images or text files": "添加图片或文本文件",
  "Up to 10 attachments per message": "一条消息最多 10 个附件",
  "Please look at the attachments.": "请看附件。",
  "Remove attachment": "去掉附件",
  "Off is quickest. Turn it up for polishing long notes.":
    "关掉最快。润色长笔记时可以调高。",
});
