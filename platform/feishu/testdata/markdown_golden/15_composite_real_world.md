# 根因

**一句话:** `agent/claudecode/claudecode.go:1156` 那行 `json.Marshal(m)` 把 prompt 里的真换行符序列化成了 JSON 字面 `\n`(两字符)。

链路:

1. `claudeSession.handleAssistant`(session.go:385)收到 `tool_use`
2. `summarizeInput`(claudecode.go:1128)的 switch 只对 Read/Edit/Write/Bash/Grep/Glob 有专门 case
3. 返回的单行 JSON 字符串作为 `ToolInput` 上送
4. `formatProgressToolInput` 拿到这个文本,因为没有真 `\n` 但 `len > 180`,包成 ` ```text\n%s\n``` `
5. 代码块里**只有一行**,飞书渲染就只有 `1 {…}`,字面 `\n` 是普通文本,不会变换行

# 验证点

`agent/claudecode/claudecode.go:1156` 这行 `b, err := json.Marshal(m)` 就是元凶。
