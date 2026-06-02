
# minutes +fetch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把一篇妙记读成**一串 markdown 正文**（标题 + 总结 + 章节 + 待办），用于阅读、总结或喂给模型。只读操作，走原生妙记 OpenAPI（不依赖问答/eqa）。

本 skill 对应 shortcut：`lark-cli minutes +fetch`。

> **与 `vc +notes` 的分工**：`minutes +fetch` 产出**一串可读 markdown 正文**（read 语义，对齐 `docs +fetch` 的 `document.content`）；[`vc +notes --minute-tokens`](../../lark-vc/references/lark-vc-notes.md) 产出**结构化纪要字段**并把逐字稿 / 纪要文档**下载成文件**。要"读内容"用本命令，要"结构化字段 / 落盘文件"用 `vc +notes`。

## 命令

```bash
# 读一篇妙记（默认正文 = 总结 + 章节 + 待办），接受 URL 或裸 token
lark-cli minutes +fetch --minute-token https://meetings.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx
lark-cli minutes +fetch --minute-token obcnxxxxxxxxxxxxxxxxxxxx

# 追加逐字稿（内联进正文的 ## 逐字稿 段，内容可能较大）
lark-cli minutes +fetch --minute-token obcnxxxxxxxxxxxxxxxxxxxx --include transcript

# 追加 AI 纪要文档 token（放进信封，便于再用 docs +fetch 拉全文）
lark-cli minutes +fetch --minute-token obcnxxxxxxxxxxxxxxxxxxxx --include note-doc

# 同时追加逐字稿与 note-doc
lark-cli minutes +fetch --minute-token obcnxxxxxxxxxxxxxxxxxxxx --include transcript,note-doc

# 预览 API 调用
lark-cli minutes +fetch --minute-token obcnxxxxxxxxxxxxxxxxxxxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <url\|token>` | 是 | 妙记 URL 或裸 token（URL 自动提取末段 token） |
| `--include <items>` | 否 | 逗号分隔的扩展项：`transcript`（逐字稿内联进正文）、`note-doc`（AI 纪要文档 token 进信封） |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 正文形态：单串 markdown

默认正文按 `# 标题` → `## 总结` → `## 章节`（每节 `### 标题` + 内容）→ `## 待办` → `## 关键词` 拼成一串 markdown，**空段自动省略**。章节在妙记接口返回有时间字段时按时间升序排列，否则保留接口顺序。

### 2. 妙记无 update_time

妙记接口只提供 `create_time`，没有 `update_time`。返回里 `minute.create_time` 即创建时间，信封带一条 `note` 注明这一点。

### 3. `--include transcript` 内联体量

逐字稿会**整段内联**进正文的 `## 逐字稿` 段，长妙记可能很大；默认不含，仅在确需原始逐字稿时显式开启。取不到逐字稿时打一行 stderr 提示并跳过，不影响正文。

### 4. `--include note-doc` 仅给 token

`note-doc` 不内联文档内容（与总结/章节高度重叠），只把 AI 纪要文档 / 逐字稿文档的 token 放进信封，配合提示用 `docs +fetch` 拉全文。非会议来源的妙记可能没有 `note_id`，此时跳过并提示。

### 5. 所需权限

| 身份 | 所需权限 |
|------|---------|
| user / bot | `minutes:minutes:readonly`、`minutes:minutes.artifacts:read`、`minutes:minutes.transcript:export`（仅 `--include transcript` 用到） |

## 输出结果

```json
{
  "minute": {
    "content": "# 标题\n\n## 总结\n\n...\n\n## 章节\n\n### 开场\n\n...\n\n## 待办\n\n- ...",
    "title": "测试纪要",
    "create_time": "2025-01-01 10:00"
  },
  "source": "minutes_native",
  "note": "妙记无 update_time，create_time 为创建时间",
  "note_doc_token": "doxcn...",
  "verbatim_doc_token": "doxcn...",
  "note_doc_hint": "fetch the rich note doc with `lark-cli docs +fetch --api-version v2 --doc <token>`"
}
```

| 字段 | 说明 |
|------|------|
| `minute.content` | 妙记正文（单串 markdown，总结 + 章节 + 待办，按需含逐字稿） |
| `minute.title` | 妙记标题 |
| `minute.create_time` | 创建时间（妙记无 update_time） |
| `source` | 固定 `"minutes_native"`，标记走原生妙记接口 |
| `note` | 关于无 update_time 的说明 |
| `note_doc_token` / `verbatim_doc_token` | 仅 `--include note-doc` 时出现：AI 纪要文档 / 逐字稿文档 token |
| `note_doc_hint` | 仅 `--include note-doc` 时出现：用 `docs +fetch` 拉文档全文的提示 |

## 如何获取 minute_token

| 来源 | 获取方式 |
|------|---------|
| 妙记 URL | 直接把 URL 传给 `--minute-token`（自动提取末段 token），如 `https://sample.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx` |
| 妙记搜索 | `lark-cli minutes +search`（详见 [lark-minutes-search](lark-minutes-search.md)） |
| 会议录制查询 | `lark-cli vc +recording --meeting-ids <id>` 或 `--calendar-event-ids <event_id>` |

## 提示

- 要"读内容 / 总结这篇妙记 / 整理成 markdown" 用本命令；要"结构化纪要字段 / 把逐字稿落盘成文件" 用 [vc +notes](../../lark-vc/references/lark-vc-notes.md)。
- `--include transcript` 内联逐字稿可能很长，按需开启。
- `note-doc` 给的是文档 token，要全文再用 [docs +fetch](../../lark-doc/references/lark-doc-fetch.md)。
- 任一扩展项取数失败只跳过该项并打一行 stderr 提示，正文照常返回。

## 参考

- [lark-minutes](../SKILL.md) — 妙记全部命令
- [lark-vc-notes](../../lark-vc/references/lark-vc-notes.md) — 会议纪要查询（结构化 / 落盘）
- [lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md) — 文档读取（note-doc token 拉全文）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
