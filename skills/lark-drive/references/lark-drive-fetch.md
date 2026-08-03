# drive +fetch

把任意飞书云文档读成一份可读 markdown 正文。传入 URL，自动识别类型（文档 / 电子表格 / 多维表 / 幻灯片 / 网盘文件 / 妙记）、自动解包知识库节点，返回提取后的正文。用于快速了解一篇文档讲了什么、看里面的表格和图片内容、判断值不值得进一步细看。

## 什么时候用它，什么时候用别的

drive +fetch 是**速览**：一个 URL 拿到可读正文，轻、快、能看懂表格图片，但不精确、丢结构。它和原生命令互补，按目标选：

| 目标 | 用什么 |
|---|---|
| 快速看全文 / 判断文档主题 / 看表格图片内容 | `drive +fetch` |
| 精确取单元格值、统计行数、筛选排序 | `sheets +cells-get`（表格）/ `base +record-list`（多维表） |
| 按词检索文档、局部精读 | `docs +fetch`（`--scope` / `--keyword`） |
| 拿到可编辑的 block id、精确编辑 | `docs +fetch --doc-format xml --detail with-ids` |
| 要原始文件字节、存到本地 | `drive +download` |

一句话：目标是"看一眼 / 通读"用 fetch；目标是"精确取数 / 精确检索 / 编辑"用对应原生命令（见上表）。

> docx 的深度精读（按 section / range / keyword 局部读、带 block id 的精确编辑）走 `docs +fetch`，见 [lark-doc-fetch.md](../../lark-doc/references/lark-doc-fetch.md)。

## 支持的类型

| 类型 | URL 路径 | fetch 读出来是什么 |
|---|---|---|
| 文档 docx / doc | `/docx/` `/doc/` | 整篇 markdown，标题 / 表 / 图 / 画板挂 `{#block-id}` 锚点；超大文档分页 |
| 电子表格 sheet | `/sheets/` | 文档名 + 每张子表的 GFM 表 |
| 多维表 base | `/base/` | 文档名 + 每张子表的 GFM 表 |
| 幻灯片 slides | `/slides/` | 标题分层 + 表格转 GFM + 图片描述 |
| 网盘文件 file | `/file/` | 提取文本（PDF / Word / Excel / 附件）；超大文件分页 |
| 妙记 minutes | `/minutes/` | 摘要 + 章节 + 待办 + 关键词；`--include transcript` 内联逐字稿，`--include note-doc` 在 warnings 里给纪要文档 token |
| 知识库 wiki | `/wiki/` | 先解包到底层资源，再按上表读 |

## 命令

```bash
# 传 URL（推荐）：自动识别类型，wiki 自动解包
lark-cli drive +fetch --url "https://xxx.feishu.cn/docx/doxcnxxx"

# 裸 token 必须显式 --type
lark-cli drive +fetch --token doxcnxxx --type docx

# wiki 里只读某张子表：?table= / ?sheet= 选择器会保留
lark-cli drive +fetch --url "https://xxx.feishu.cn/wiki/wikcnxxx?table=tblXXX"
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 二选一 | 文档 URL（推荐） |
| `--token` + `--type` | 二选一 | 裸 token 需 `--type`（docx / sheet / bitable / slides / file / minutes / wiki；也接受别名 doc / sheets / base） |
| `--embed-max-rows` | 否 | 物化表格每表最多 N 行（默认 50，0 = 不限），超了截断并提示 |
| `--full` | 否 | 仅 docx / file：一次返回整篇，关闭自动分页 |
| `--page-token` | 否 | 仅 docx / file：续读下一页；不能与 `--full` 同用 |
| `--page-size` | 否 | 仅 docx / file：每页大小提示（默认 0 = 服务端默认），不能与 `--full` 同用 |
| `--include` | 否 | 仅 minutes：`transcript` 内联逐字稿 / `note-doc` 取纪要文档 token |

## 输出

默认输出 JSON：`{content, resource, warnings, has_more, next_page_token}`。

- `content`：可读 markdown 正文
- `resource`：`{type, title, url, token, selector, update_time, create_time, source}`
  - `selector`：URL 里的 `?sheet=` / `?table=` / `?view=` 透传过来
  - `source`：仅 wiki 输入出现，记录解包前的 wiki 节点
  - `create_time`：仅 minutes（minutes 没有 `update_time`）
- `has_more` / `next_page_token`：docx / file 超大文档的分页游标
- `warnings`：提示信息（如妙记逐字稿取不到）

加 `--format pretty` 只打印 markdown 正文。

## 速览的边界（拿不全时怎么办）

- **表格被截断**：GFM 表超过 `--embed-max-rows`（默认 50 行）会截断，尾部写「还有 X 行」。要全量有两种方式——调大 `--embed-max-rows`（设 `0` 拿不截断的 markdown，适合通读全表）；或改用 `sheets +cells-get` / `base +record-list`（适合精确取数、统计、筛选）。
- **docx / file（PDF 等）太大**：默认只返回第 1 页 + `next_page_token`；需要更多用 `--page-token` 续读，不大且确需整篇可用 `--full`。
- **docx 内嵌的电子表格**：默认就展开成 GFM 表（受 `--embed-max-rows` 截断，截断行为同正文表）。
- **docx 内嵌的多维表格**：默认是占位（带 `[](token=xxx)`），要展开整张内嵌表用 `docs +fetch --doc-format markdown --inline-embeds`，或拿 token 去 base 技能取结构化数据。

## 正文里的两种标记

- `{#block-id}`（标题 / 表 / 图 / 画板后）：定位**文档里的这块内容**，要编辑它先用 `docs +fetch --doc-format xml --detail with-ids` 拿到可编辑结构。
- `[名称](token=xxx)`（画板、内嵌表后）：`xxx` 是该**资源**本身的标识，和 block-id 不是一回事。画板 token 可用 `docs +media-download --type whiteboard` 取素材；内嵌多维表格 token 走 base 技能。
