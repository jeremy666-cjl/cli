---
name: lark-search
version: 1.0.0
description: "飞书全局搜索：当用户想在飞书里搜索、查找、定位资料时使用。支持跨实体搜索，也可以用于单个实体的普通关键词/语义搜索；资料可能来自文档、Wiki、消息内容、邮件、已结束会议、纪要、词典、服务台、评论等。典型请求：搜一下 X、找一下之前关于 X 的资料、查 X 相关讨论、只记得标题/关键词但不知道在哪里、先找上下文。不要用于已知 URL/token 的读取；不要用于群/会话本身搜索或日程、日历事件、会议邀约搜索；不要用于带结构化条件的搜索，例如按发送人/创建人/负责人、时间范围、状态、附件类型、Base 字段、文件夹、owner 等过滤，此类需求应使用对应 lark-* skill 的 search/list 能力。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli search --help"
---

# lark-search

先阅读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，处理认证、身份、权限和通用错误。

本 skill 只做“搜索和定位资料”。不要在这里完成读取全文、总结、写入、发送、下载、修改等操作。

## 何时使用

使用 `lark-cli search`：

- 用户想像使用飞书搜索框一样搜索资料。
- 用户只提供自然语言 query、关键词、主题、标题片段或模糊描述。
- 用户不确定资料在哪个实体里，需要跨文档、Wiki、消息内容、邮件、已结束会议、纪要、词典、服务台等搜索。
- 用户明确只想搜某一类实体，但搜索条件仍然是普通关键词或语义 query。
- 用户要先找到相关资料，再决定下一步操作。

示例：

```bash
# 全局搜索，默认让服务端决定搜索范围
lark-cli search --query "Q3 OKR 进度" --size 10 --format xml

# 单实体普通搜索也可以用
lark-cli search --query "Kubernetes 迁移方案" --in doc --size 10 --format xml

# 搜词典/百科词条
lark-cli search --query "SLA 定义" --in lingo --size 10 --format xml

# 搜服务台知识或问答
lark-cli search --query "VPN 连不上怎么处理" --in helpdesk --size 10 --format xml

# 用户强调最近/最新，但没有严格时间窗时
lark-cli search --query "AI 周报模板" --order time --size 10 --format xml
```

## 何时不用

不要用本 skill 处理结构化条件搜索，改用对应实体 skill：

| 用户需求 | 改用 |
|---|---|
| 已知文档/Wiki URL 或 token，要读取内容 | `lark-doc` / `lark-wiki` |
| 搜云文档/云盘文件，并按 owner、类型、时间、文件夹过滤 | `lark-drive +search` |
| 搜群、会话本身，或按群名/成员找 chat | `lark-im +chat-search` |
| 搜聊天消息，并按 chat、sender、时间、附件过滤 | `lark-im +messages-search` |
| 搜会议，并按时间范围、参会人、会议号过滤 | `lark-vc +search` |
| 查日程、日历事件、会议邀约（calendar-event） | `lark-calendar` |
| 搜任务，并按状态、负责人、截止时间过滤 | `lark-task +search` |
| 搜联系人 | `lark-contact` |
| 搜 Base 记录、字段过滤、聚合分析 | `lark-base` |
| 创建、更新、发送、删除、下载 | 对应实体 skill |

一句话：普通 query 搜索用 `lark-search`；结构化条件搜索用对应实体 skill。

## 命令规则

本 skill 只有一个日常入口：`lark-cli search`。`search` 本身就是命令，不是 `search +search`。

基础命令：

```bash
lark-cli search --query "<query>" --size 10 --format xml
```

参数：

| 参数 | 规则 |
|---|---|
| `--query` | 必填。使用用户原始关键词或自然语言问题，不要改写成复杂提示词。 |
| `--size` | 首次用 `10`；结果不够再升到 `20`；不要默认拉满。 |
| `--format` | CLI 默认 `json`；本 skill 建议 agent 显式传 `--format xml`，便于阅读片段和元信息。需要 `jq` 或机器后处理时用 `json`。 |
| `--order` | 默认 `rank`。用户强调“最近/最新”且没有严格时间窗时可用 `time`。 |
| `--in` | 用户明确限定实体时才传；不确定时留空，让服务端默认跨域搜索。 |

常用 `--in`：

| 用户说 | `--in` |
|---|---|
| 文档、Wiki、方案、材料 | `doc` |
| 消息内容、聊天记录内容、群里说过的内容 | `message` |
| 邮件 | `mail` |
| 会议记录 | `lark:vc` |
| 纪要、妙记 | `minutes` |
| 词典、百科、术语、名词解释 | `lingo` |
| 服务台、帮助中心、客服知识、FAQ | `helpdesk` |
| 评论 | `comment` |

注意：

- 不确定实体时不要传 `--in`。
- `wiki` 不是独立 scope；文档/Wiki 用 `doc`。
- `chat` 不是可用 scope；搜群/会话本身用 `lark-im +chat-search`，搜聊天消息内容可用 `--in message`。
- `meeting` 可视为 `lark:vc`（已结束会议）；纪要、妙记用原生 `minutes`，没有 `lark:minutes` 委派。
- `event` 不是可用 scope；日程、日历事件、会议邀约（calendar-event）不在 `search` 范围。查未来日程用 `lark-calendar`，查已结束会议用 `--in lark:vc`。
- `search` 不提供严格时间窗；需要 `--start/--end` 语义时切到实体 search。
- `lark:base` 不作为推荐入口；Base 文件定位优先用 `lark-drive`，表内查询用 `lark-base`。

## 结果处理

搜索结果是资料线索。读取结果时关注：

- `entity_type`
- `title`
- `url`
- `snippet`
- `id`
- `typed_meta`
- `warnings`

如果用户只要求“找一下/搜一下”，返回最相关候选资料，包含标题、实体类型、URL 和简短匹配理由。

如果用户要求继续读取、总结、回复、修改或下载，先用搜索结果定位目标，再切到对应 skill：

| 搜索结果 | 后续 skill |
|---|---|
| 文档 / Wiki | `lark-doc` / `lark-drive` |
| 消息 / 群聊 | `lark-im` |
| 邮件 | `lark-mail` |
| 会议 | `lark-vc` |
| 纪要 / 妙记 | `lark-minutes` |
| 任务 | `lark-task` |
| 联系人 | `lark-contact` |
| 词典 / 百科 / 术语 | 先返回搜索结果；如需编辑或管理，按实际可用实体能力另行选择 |
| 服务台 / FAQ | 先返回搜索结果；如需进一步操作，按结果 URL 或实体类型选择下游能力 |
| Base / bitable | `lark-base` |

## 自纠错

- `--in docs` 报错时改为 `doc`。
- 搜不到时先放宽 query，或去掉 `--in`。
- 噪声太多时缩小 `--in`，并保持 `--size 10`。
- 用户要求“最近/最新”但没有严格时间范围时，可试 `--order time`。
- 用户提出明确结构化条件时，不要继续调 `lark-cli search`，改用对应实体 search。
