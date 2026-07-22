
# drive +fetch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把任意飞书云文档读成一份可读 markdown，用于阅读 / 总结 / 喂模型。传一个 URL（或 `--token --type`），自动识别类型并解包 wiki，一次返回 markdown 快照。

## 什么时候用哪个

- **读内容（要文本）** → `drive +fetch`：返回提取后的可读 markdown，不落地文件。
- **doc 精读 / 编辑准备** → `docs +fetch`：支持 `--scope` / `--detail` / `--inline-embeds` / 局部读取。`drive +fetch` 读 doc 走的是整篇快照道，精读仍用 `docs +fetch`。
- **取原始文件字节 / 存到本地** → `drive +download`。
- **要结构化数据**（单元格值、记录列表）→ `sheets +cells-get` / `base +record-list`，不要用 `+fetch`。

## 支持的类型

| 类型 | URL 路径 | 读取方式 |
|------|----------|----------|
| 文档 docx/doc | `/docx/` `/doc/` | 整篇 markdown + `{#block-id}` 锚点 + 分页；取不到时回退原生 docs_ai markdown |
| 电子表格 sheet | `/sheets/` | 渲染为 GFM 表格 |
| 多维表 base | `/base/` | 渲染为 GFM 表格 |
| 幻灯片 slides | `/slides/` | 渲染为 markdown |
| 网盘文件 file | `/file/` | 提取文本（PDF / Word / Excel / 附件等） |
| 妙记 minutes | `/minutes/` | 原生读取：摘要 + 章节 + 待办 + 关键词（逐字稿 / 笔记 doc 可选） |
| 知识库 wiki | `/wiki/` | 先解包到底层资源，再按上表读取；来源 wiki 节点记入 `resource.source` |

`drive +fetch` 经知识服务按当前用户的**读权限**取内容——所以连「**可阅读但禁止下载/复制**」的文件也能读到内容（这类文件 `drive +download` 会因无下载权限失败）。

## 命令

```bash
# 传 URL（推荐；自动识别类型，wiki 会自动解包）
lark-cli drive +fetch --url "https://xxx.feishu.cn/sheets/shtcnxxx"
lark-cli drive +fetch --url "https://xxx.feishu.cn/wiki/wikcnxxx"
lark-cli drive +fetch --url "https://meetings.feishu.cn/minutes/omcnxxx"

# 裸 token 必须显式 --type
lark-cli drive +fetch --token boxcn_xxx --type file

# JSON 输出（含 resource / render / warnings 等元数据）
lark-cli drive +fetch --url "https://xxx.feishu.cn/docx/doxcnxxx" --format json
```

## 参数

| 参数 | 说明 |
|------|------|
| `--url` | 资源 URL（docx / doc / sheet / base / wiki / slides / file / minutes）；与 `--token` 二选一 |
| `--token` | 裸资源 token，必须配合 `--type` |
| `--type` | 资源类型；`--token` 时必填，`--url` 时自动识别（若手填需与 URL 类型一致） |
| `--embed-max-rows` | 每张表最多渲染 N 行数据（默认 50，0 = 不限） |
| `--image-urls` | 图片渲染：`none`（仅说明文字）/ `one`（单 URL + 宽高，默认）/ `full`（全部路由） |
| `--full` | 仅 doc：一次返回整篇，关闭自动分页 |
| `--page-token` | 仅 doc：用上一次返回的 `next_page_token` 继续读下一页 |
| `--page-size` | 仅 doc：每页 token 预算提示（0 = 服务端默认） |
| `--include` | 仅 minutes：追加额外内容，逗号分隔 `transcript`（逐字稿）、`note-doc`（笔记 doc） |

## 行为说明

- 默认输出 JSON 信封（`{content, resource, render, warnings}`）；`--format pretty` 只打印 markdown 正文。
- wiki 链接先解包到底层资源再读取；`?sheet=` / `?table=` 等子资源选择器会保留并记入 `resource.selector`。
- 某类型读取不可用（知识服务未就绪 / 无读权限）时返回明确报错并给出该类型的替代命令（如 sheet → `sheets +cells-get`、file → `drive +download`），不会给半截结果。

## 各类型读取细节

- **doc / docx**：整篇 markdown，标题 / 表 / 图 / 画板挂 `{#block-id}` 锚点；默认自动分页，返回 `has_more` / `next_page_token`，要一次拿整篇用 `--full`。`--page-token` 续读时若本页取不到，直接报错（不静默回退到开头），按提示重跑。eqa 取不到时自动回退原生 docs_ai markdown（会在 `warnings` 提示）。
- **sheet / base**：`# 文档名` → 每个子表 `## 子表名` → 子表内容展开成 GFM 表（人名 `@`、日期升序由服务端渲染）。URL 带 `?sheet=` / `?table=` 只读对应子表；裸 token 读默认子表。超长表按 `--embed-max-rows`（默认 50）截断并尾部提示「还有 X 行」，要全量调大或设 0。qa 不可用时报错并指向 `sheets +cells-get` / `base +record-list`。
- **slides**：标题分层 + 表格转 GFM + 图片带文字描述。**输出是渲染快照，没有 block/shape id、且夹有 `[block_sep]` 标记，不能用于 `slides +replace-slide` 定位**——要改页先用 `xml_presentations.get` 拿带 id 的结构。
- **file**：提取文本（PDF / Word / Excel / 附件等）；「可阅读但禁止下载/复制」的文件也能读到内容。要原始字节用 `drive +download`。
- **minutes**：`# 标题` → `## 总结` → `## 章节` → `## 待办` → `## 关键词`，空段自动省略，章节按时间升序。逐字稿、笔记 doc 用 `--include` 显式追加：`transcript` 把逐字稿整段内联进正文（长妙记可能很大，取不到则跳过并提示）；`note-doc` 只把 AI 纪要文档 token 放进 `warnings`，要用 `docs +fetch` 拉全文。妙记用 `create_time`（无 `update_time`）。

## 输出信封（默认输出）

```json
{
  "content": "...(markdown)...",
  "resource": {
    "type": "sheet",
    "title": "...",
    "url": "https://...",
    "token": "...",
    "selector": { "sheet": "..." },
    "update_time": 1234567890
  },
  "render": { "format": "markdown", "table_format": "gfm", "max_table_rows": 50, "image_urls": "one" },
  "warnings": []
}
```

- `resource.update_time`：大部分类型有；minutes 没有 `update_time`，改给 `create_time`。均 omitempty。
- `resource.selector`：仅当 URL 带 `?sheet=` / `?table=` 时出现。
- `resource.source`：仅当输入是 wiki 链接时出现，记录来源 wiki 节点（`type` / `input_url` / `node_token` / `space_id`）。
- `has_more` / `next_page_token`：仅 doc 分页时出现。
- `warnings`：回退、逐字稿缺失、笔记 doc token 等提示；无则为空或省略。

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-drive-download](lark-drive-download.md) -- 下载原始文件字节到本地
- [lark-drive-inspect](lark-drive-inspect.md) -- 检视 URL 类型 / 解包 wiki
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
