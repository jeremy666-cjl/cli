
# base +fetch

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把一个多维表格读成**一串 markdown 正文**，可直接阅读、总结或喂给模型。每张子表的内容展开成一张 GFM（管道）表格，并由服务端做好归一化：有序列、人名渲染成 `@`、日期升序。只读操作，走问答(qa) fetch 链路（与 `docs +fetch --inline-embeds`、`sheets +fetch` 同一后端）。

本 skill 对应 shortcut：`lark-cli base +fetch`。

> **与 `base +record-list` 的分工**：`base +fetch` 产出**整个多维表的一串可读 markdown 正文**（read 语义，对齐 `docs +fetch`，人名/日期已归一化）；[`base +record-list`](lark-base-record.md) 产出**某一张表的结构化 record**（裸值、多一列 `_record_id`、列序原样、单元格未归一化）。要"把整表读成内容 / 总结"用本命令，要"结构化 record / 取数 / 写回"用 `+record-list` / `+record-get` / `+data-query`。

## 命令

```bash
# 读一个多维表（默认正文 = 各子表展开成的 GFM 表格），接受 URL 或裸 app_token
lark-cli base +fetch --url https://sample.feishu.cn/base/appxxxxxxxxxxxxxxxxxx
lark-cli base +fetch --base-token appxxxxxxxxxxxxxxxxxx

# URL 带 ?table= 时只读对应子表（服务端解析）
lark-cli base +fetch --url 'https://sample.feishu.cn/base/appxxxx?table=tblxxxx'

# 限制每张表渲染的行数（默认 50，0 = 不截断）
lark-cli base +fetch --url <url> --embed-max-rows 100

# 预览 API 调用
lark-cli base +fetch --url <url> --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 二选一 | 多维表 URL（**保留 `?table=`**，服务端据此选子表） |
| `--base-token <token>` | 二选一 | 多维表 app_token（与 `--url` 互斥） |
| `--embed-max-rows <n>` | 否 | 每张渲染出的表格的行数上限，默认 50（0 = 不截断），超出加"还有 X 行"提示 |
| `--image-urls <mode>` | 否 | 图片渲染：`none`（仅 caption）/ `one`（单图 URL + 宽高，默认）/ `full`（全路由，程序消费） |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 正文形态：单串 markdown，表格已展开为 GFM

正文由 qa 服务端渲染：每张子表的内容展开成一张 GFM 表格（不是占位标签，是真实行列），**有序列、人名 `@`、日期升序由服务端产出**，cli 只转发 URL 并渲染图片 / 截断超长表。这与 `+record-list` 的裸 record（数组值、`_record_id` 列、未归一化）不同。

### 2. 转发原始 URL，服务端解析子表

cli 把**原始 URL 原样**发给 qa fetch；`?table=<tableId>` 由服务端解析选子表。传裸 app_token 时读默认子表。

### 3. `--embed-max-rows` 截断

超长表按 `--embed-max-rows`（默认 50）截断，尾部加一行"还有 X 行（用 base +record-list 取全量）"提示。要全量调大或设 0，或改用 `+record-list` 分页。

### 4. 失败即报错、指向原生命令

`base +fetch` 是 qa fetch 专属通道。qa 不可用 / 表未被索引时**直接报错**并提示改用 `base +record-list`（新命令无既有原生 fetch 可静默回退，报错比给半成品更诚实）。

### 5. 所需权限

| 身份 | 所需权限 |
|------|---------|
| user / bot | `base:record:read` |

## 输出结果

```json
{
  "bitable": {
    "content": "## 表A\n\n| 列1 | 列2 |\n| --- | --- |\n| ... |\n\n## 表B\n\n...",
    "title": "多维表名",
    "update_time": 1700000000
  },
  "source": "eqa_base_fetch"
}
```

| 字段 | 说明 |
|------|------|
| `bitable.content` | 正文（单串 markdown，各子表展开成的 GFM 表格，人名 `@` / 日期升序） |
| `bitable.title` | 多维表标题 |
| `bitable.update_time` | 更新时间（服务端提供） |
| `source` | 固定 `"eqa_base_fetch"`，标记走 qa fetch 通道 |

## 如何获取 base URL / app_token

| 来源 | 获取方式 |
|------|---------|
| 多维表 URL | 直接把 URL 传给 `--url`，如 `https://sample.feishu.cn/base/appxxxx`（带 `?table=` 选子表） |
| 云空间搜索 | `lark-cli drive +search --query <keyword> --doc-types bitable` 先定位 |
| Wiki 节点 | `lark-cli wiki +node-get`，`obj_type=bitable` 时取 `obj_token` 当 app_token |

## 提示

- 要"读整张多维表 / 总结 / 整理成 markdown"用本命令；要"结构化 record / 取数 / 写回"用 [base +record-list](lark-base-record.md) / `+record-get` / `+data-query`。
- URL 带 `?table=` 即读对应子表，无需手动拆 table-id。
- 长表用 `--embed-max-rows` 控制体量，或改 `+record-list` 分页取全量。

## 参考

- [lark-base](../SKILL.md) — 多维表全部命令
- [lark-base-record](lark-base-record.md) — 记录读写（`+record-list` 取结构化 record）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
