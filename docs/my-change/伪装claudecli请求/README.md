# Claude CLI 请求伪装功能

## 功能说明

在 claudecode 类型渠道的添加/编辑弹框中新增「伪装 CLI 请求」开关。开启后，AxonHub 会自动识别请求来源，对非 Claude CLI 的请求进行伪装，使其看起来像真实的 Claude Code CLI 发出的请求。

## 核心逻辑

```
开关关闭（默认）→ 不做任何注入，请求原样透传
开关开启 → 三步检测请求来源：
  ├─ 真实 CLI 请求 → 透传，保留原始 UA
  └─ 非 CLI 请求   → 执行伪装注入：
       1. 注入/替换 billing header（system[0]）
       2. 注入 Claude Code 系统提示词
       3. 注入/替换 metadata.user_id
       4. 设置 Claude Code 专用请求头和 UA
```

### 三步检测

**第一步：User-Agent**

检查 UA 是否以 `claude-cli/` 开头。

示例（通过）：
```
User-Agent: claude-cli/1.0.83 (external, cli)
User-Agent: claude-cli/2.0.31 (external, cli)
```

示例（不通过）：
```
User-Agent: curl/7.68.0
User-Agent: Mozilla/5.0 ...
```

**第二步：系统消息身份**

检查 system role 消息中是否包含 Claude Code 身份标识。支持以下 3 种变体（子串匹配）：

| 变体 | 身份标识文本 | 来源 |
|------|------------|------|
| 标准 CLI | `"You are Claude Code, Anthropic's official CLI for Claude."` | Claude Code CLI 标准版 |
| Agent SDK 变体1 | `"You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK."` | Claude Code 在 Agent SDK 内运行 |
| Agent SDK 变体2 | `"You are a Claude agent, built on Anthropic's Claude Agent SDK."` | Agent SDK 独立提示词 |

> 代码中用 `strings.Contains` 做子串匹配，所以变体1被第一条规则覆盖（`"You are Claude Code, Anthropic's official CLI for Claude"` 是其子串）。

兼容新旧版本的 system 结构：
- 旧版本：身份标识在 system[0]
- 新版本：system[0] 是 billing header，身份标识在 system[1]

**第三步：metadata.user_id 格式**

检查是否匹配格式：`user_{64位十六进制}_account__session_{uuid}`

示例（通过）：
```
user_01a2b3c4d5e6f7890123456789abcdef01a2b3c4d5e6f7890123456789abcdef_account__session_550e8400-e29b-41d4-a716-446655440000
```

三步全部通过才判定为真实 CLI 请求。

### 伪装注入内容

当检测到非 CLI 请求时，按以下顺序注入：

**1. Billing Header（system[0]）**

如果配置了 `billingHeaderValue`：
- 请求中已有 `x-anthropic-billing-header` → 替换为配置的值
- 请求中没有 → 注入为新的 system[0]

示例配置值：
```
x-anthropic-billing-header: cc_version=2.1.50.b97; cc_entrypoint=cli; cch=00000;
```

注入后的 system 数组结构：
```json
{
  "system": [
    {
      "type": "text",
      "text": "x-anthropic-billing-header: cc_version=2.1.50.b97; cc_entrypoint=cli; cch=00000;"
    },
    {
      "type": "text",
      "text": "You are Claude Code, Anthropic's official CLI for Claude.",
      "cache_control": { "type": "ephemeral" }
    }
  ]
}
```

**2. Claude Code 系统提示词**

注入 `"You are Claude Code, Anthropic's official CLI for Claude."` 作为 system 消息（如不存在）。

**3. metadata.user_id**

- 配置了统一客户端标识 → 使用该标识替换 user_id 中的客户端部分
- 未配置 → 随机生成一个 64 位十六进制标识

示例（使用统一标识 `aabbccdd...`）：
```
user_aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899_account__session_550e8400-e29b-41d4-a716-446655440000
```

**4. 请求头和 UA**

设置 Claude Code 专用请求头（Anthropic-Beta、Anthropic-Version、X-App 等）和 UA（`claude-cli/1.0.83 (external, cli)`）。

### 可配置项

| 配置项 | 说明 | 示例值 |
|--------|------|--------|
| 伪装 CLI 请求 | 开关，控制是否启用伪装 | `true` / `false` |
| 统一客户端标识 | 64 位十六进制字符串，替换 user_id 中的客户端部分。留空则每次随机生成 | `aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899` |
| Billing Header | 注入到 system[0] 的值。留空则不注入 billing header | `x-anthropic-billing-header: cc_version=2.1.50.b97; cc_entrypoint=cli; cch=00000;` |

### 数据库影响

**无**。`ChannelSettings` 是 JSON 字段，新增的 Go struct 字段会自动序列化，不需要数据库迁移。

## 修改文件清单

### 后端

| 文件 | 说明 |
|------|------|
| `internal/objects/channel.go` | ChannelSettings 新增 `DisguiseCliRequest`、`UnifiedClientId`、`BillingHeaderValue` 三个字段 |
| `internal/server/gql/axonhub.graphql` | ChannelSettings type 和 ChannelSettingsInput 各新增三个 GraphQL 字段 |
| `llm/transformer/anthropic/claudecode/outbound.go` | Params/struct 新增字段；TransformRequest 中加入 shouldInject 条件判断逻辑 |
| `llm/transformer/anthropic/claudecode/utils.go` | 新增四个函数：`isRealCliRequest`（三步检测）、`hasClaudeCodeIdentity`（系统消息身份检查）、`injectOrReplaceBillingHeader`（billing header 注入/替换）、`injectOrReplaceUserID`（统一客户端标识替换） |
| `internal/server/biz/channel_llm.go` | 从 channel settings 提取伪装配置，传入 transformer Params |

### 前端

| 文件 | 说明 |
|------|------|
| `frontend/src/features/channels/data/schema.ts` | Zod schema 新增三个字段 |
| `frontend/src/features/channels/data/channels.ts` | GraphQL query 和 mutation 响应中新增三个字段 |
| `frontend/src/features/channels/components/channels-action-dialog.tsx` | 新增 state 变量、UI 开关和配置面板、提交/重置逻辑 |
| `frontend/src/features/channels/utils/merge.ts` | mergeChannelSettingsForUpdate 新增三个字段 |
| `frontend/src/locales/zh-CN/channels.json` | 中文翻译（9 个 key） |
| `frontend/src/locales/en/channels.json` | 英文翻译（9 个 key） |

## 构建注意

修改了 GraphQL schema，需要 cd d:\code\sanyun\axonhub\internal\server\gql 运行 `go generate` 重新生成代码后再编译。
