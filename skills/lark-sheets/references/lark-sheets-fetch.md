
# sheets +fetch

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把一张电子表格读成**一串 markdown 正文**，可直接阅读、总结或喂给模型。结构是三段式：`# 文档名` → 每张子表 `## 子表名` → 子表内容渲染成一张 GFM（管道）表格。只读操作，走问答(qa) fetch 链路（与 `docs +fetch --inline-embeds`、`base +fetch` 同一后端）。

本 skill 对应 shortcut：`lark-cli sheets +fetch`。

> **与 `sheets +read` 的分工**：`sheets +fetch` 产出**整张表的一串可读 markdown 正文**（read 语义，对齐 `docs +fetch`）；[`sheets +read`](lark-sheets-cell-data.md#read) 产出**某个区间的裸二维数组**（需显式 `--range`）。要"把整张表读成内容 / 总结"用本命令，要"取某区间的结构化数据"用 `sheets +read`。

## 命令

```bash
# 读一张电子表格（默认正文 = 文档名 + 各子表 GFM 表），接受 URL 或裸 token
lark-cli sheets +fetch --url https://sample.feishu.cn/sheets/shtxxxxxxxxxxxxxxxxxx
lark-cli sheets +fetch --spreadsheet-token shtxxxxxxxxxxxxxxxxxx

# URL 带 ?sheet= 时只读对应子表（服务端解析）
lark-cli sheets +fetch --url 'https://sample.feishu.cn/sheets/shtxxxx?sheet=abcdef'

# 限制每张表渲染的行数（默认 50，0 = 不截断）
lark-cli sheets +fetch --url <url> --embed-max-rows 100

# 预览 API 调用
lark-cli sheets +fetch --url <url> --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 二选一 | 电子表格 URL（**保留 `?sheet=`**，服务端据此选子表） |
| `--spreadsheet-token <token>` | 二选一 | 电子表格裸 token（与 `--url` 互斥） |
| `--embed-max-rows <n>` | 否 | 每张渲染出的表格的行数上限，默认 50（0 = 不截断），超出加"还有 X 行"提示 |
| `--image-urls <mode>` | 否 | 图片渲染：`none`（仅 caption）/ `one`（单图 URL + 宽高，默认）/ `full`（全路由，程序消费） |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 正文形态：单串 markdown，表格已展开为 GFM

正文由 qa 服务端渲染：`# 文档名` → 每张子表 `## 子表名` → 子表内容展开成一张 GFM 表格（不是占位标签，是真实行列）。**子表排版、人名 `@`、日期升序由服务端产出**，cli 只转发 URL 并渲染图片 / 截断超长表。

### 2. 转发原始 URL，服务端解析子表

cli 把**原始 URL 原样**发给 qa fetch；`?sheet=<sheetId>` 由服务端解析选子表。传裸 token 时读整表默认子表。

### 3. `--embed-max-rows` 截断

超长表按 `--embed-max-rows`（默认 50）截断，尾部加一行"还有 X 行（用 base +record-list 取全量）"提示。要全量调大或设 0。

### 4. 失败即报错、指向原生命令

`sheets +fetch` 是 qa fetch 专属通道。qa 不可用 / 表未被索引时**直接报错**并提示改用 `sheets +read` / `sheets +info`（新命令无既有原生 fetch 可静默回退，报错比给半成品更诚实）。

### 5. 所需权限

| 身份 | 所需权限 |
|------|---------|
| user / bot | `sheets:spreadsheet:read` |

## 输出结果

```json
{
  "spreadsheet": {
    "content": "# 文档名\n\n## 子表A\n\n| 列1 | 列2 |\n| --- | --- |\n| ... |",
    "title": "文档名",
    "update_time": 1700000000
  },
  "source": "eqa_sheet_fetch"
}
```

| 字段 | 说明 |
|------|------|
| `spreadsheet.content` | 正文（单串 markdown，文档名 + 各子表展开成的 GFM 表格） |
| `spreadsheet.title` | 表格标题 |
| `spreadsheet.update_time` | 更新时间（服务端提供） |
| `source` | 固定 `"eqa_sheet_fetch"`，标记走 qa fetch 通道 |

## 如何获取 spreadsheet URL / token

| 来源 | 获取方式 |
|------|---------|
| 表格 URL | 直接把 URL 传给 `--url`，如 `https://sample.feishu.cn/sheets/shtxxxx`（带 `?sheet=` 选子表） |
| 云空间搜索 | `lark-cli drive +search` 先定位表格文件 |
| Wiki 节点 | `lark-cli wiki spaces get_node` 取 `obj_token`（obj_type=sheet） |

## 提示

- 要"读整张表 / 总结这张表 / 整理成 markdown"用本命令；要"取某区间的裸二维数组"用 [sheets +read](lark-sheets-cell-data.md#read)。
- URL 带 `?sheet=` 即读对应子表，无需手动拆 sheet-id。
- 长表用 `--embed-max-rows` 控制体量。

## 参考

- [lark-sheets](../SKILL.md) — 电子表格全部命令
- [lark-sheets-cell-data](lark-sheets-cell-data.md) — 单元格读写（`+read` 取裸区间数据）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
