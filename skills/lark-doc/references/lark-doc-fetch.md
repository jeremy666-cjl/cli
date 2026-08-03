
# docs +fetch

读取飞书文档（docx / wiki）内容的入口。两种用法：

- **markdown 速览整篇**：`--doc-format markdown` —— 整篇可读 markdown，标题/表/图/画板带 `{#block-id}` 浅锚点；大文档分页；`--inline-embeds` 把内嵌的多维表展成 GFM 表。
- **xml 精读 / 编辑**：默认 xml —— `--scope` 局部精读（目录/章节/区间/关键词）+ `--detail` 拿 block-id/样式，用于定位和编辑。

> 跨类型速览（一个 URL 读文档/表格/幻灯片/文件…，不挑类型）用 `drive +fetch`（见 [lark-drive-fetch.md](../../lark-drive/references/lark-drive-fetch.md)）；docx 的深度精读、拿 block-id、展开内嵌表用本命令。

## 命令

```bash
# 获取文档（默认 XML，simple）
lark-cli docs +fetch --doc "https://xxx.feishu.cn/docx/Z1Fj...tnAc"

# Markdown 格式
lark-cli docs +fetch --doc Z1Fj...tnAc --doc-format markdown

# 带 block ID（用于后续 block 级更新）
lark-cli docs +fetch --doc Z1Fj...tnAc --detail with-ids

# 只拿目录
lark-cli docs +fetch --doc Z1Fj...tnAc --scope outline --max-depth 3

# 按 block id 区间精读
lark-cli docs +fetch --doc Z1Fj...tnAc --scope range --start-block-id blkA --end-block-id blkB --detail with-ids

# URL 带 #share 选区锚点时自动局部读取
lark-cli docs +fetch --doc 'docURL#share-anchor'

# 读整个章节（以标题 id 为锚点，自动展开到下一个同级/更高级标题前）
lark-cli docs +fetch --doc Z1Fj...tnAc \
  --scope section --start-block-id <标题id> --detail with-ids

# 按关键词定位（多关键词用 | 分隔，任一命中即返回）
lark-cli docs +fetch --doc Z1Fj...tnAc \
  --scope keyword --keyword "部署|发布|上线"

# markdown 整篇速览（带 {#block-id} 浅锚点，大文档分页）
lark-cli docs +fetch --doc Z1Fj...tnAc --doc-format markdown

# markdown + 展开内嵌多维表到 GFM
lark-cli docs +fetch --doc Z1Fj...tnAc --doc-format markdown --inline-embeds

# 大 markdown 文档续读下一页
lark-cli docs +fetch --doc Z1Fj...tnAc --doc-format markdown --page-token <next_page_token>
```

## markdown 整篇速览（--doc-format markdown）

整篇读成可读 markdown，标题/表/图/画板后挂 `{#block-id}` 浅锚点（和 `drive +fetch` 读 docx 的输出一致）。大文档默认分页，返回第 1 页 + `has_more` / `next_page_token`，用 `--page-token` 续读、`--full` 一次拿整篇。

- `{#block-id}`：定位文档里的这块内容；要回写编辑先拿可编辑结构（`--doc-format xml --detail with-ids`）。
- `--inline-embeds`：把文档内嵌的**多维表格**（默认是 `[](token=xxx)` 占位）展成 GFM 表；内嵌电子表格默认已展 GFM。都受 `--embed-max-rows` 截断。
- `--full` / `--page-token` / `--page-size`：仅 markdown 整篇可用；`--full` 不能和 `--page-token` / `--page-size` 同用。

> 带 `--revision-id`（历史版本）或显式 `--lang` 时，分页 / `--inline-embeds` 不适用（这两种走标准导出，不支持这些增强）。

精读局部 / 拿 block-id 编辑用下面的 xml 模式（`--scope` + `--detail`）。

## 选 `--detail`（每块详细度）

| 意图 | `--detail` | 说明 |
|------|-----------|------|
| **只读**：浏览或总结文档内容 | `simple`（默认） | 简洁 XML/Markdown，不含 block ID、样式属性、引用元数据 |
| **定位**：需要 block ID 与其他业务交互 | `with-ids` | 包含 block ID（如 `<p id="blkcnXXXX">`），可用于 `+update` 的 `--block-id`，也可用于拼接 `文档URL#block_id` 形式的直达链接 |
| **编辑**：任何修改文档内容的需求 | `full` | 包含 block ID + 样式属性 + 引用元数据，提供完整文档结构信息 |

## 选 `--scope`（读取范围）

`--scope` 和 `--detail` 正交可组合。**省略 `--scope` 即读整篇；获取一小节时优先用局部读取。**

| 模式 | 何时用 | 关键参数 | 行为要点 |
|-|-|-|-|
| `outline` | 不知道结构，先看目录 | `--max-depth`（标题层级上限） | 扁平列出所有标题，**包括嵌在容器里的内嵌标题**（如 callout 里的 h3）；这些 id 可直接作后续 `section` / `range` 端点 |
| `section` | 读某个标题对应的整节 | `--start-block-id`（必填） | 顶层标题 → 展开到下一同级/更高级标题前；容器内节点（含内嵌标题） → 按"最小包容单元"返回容器/表格切片，不做 heading 扩展；顶层非标题块 → 仅该块 |
| `range` | 已知精确起止 | `--start-block-id` / `--end-block-id` 至少一个；`-1` = 读到末尾 | 两端同顶层 → 顶层序列切片；两端同一容器 → 容器整体；两端同一表格 → 瘦身切片；**跨顶层 → 端点所在顶层块整块输出，不做瘦身** |
| `keyword` | 只有模糊关键词 | `--keyword`（**多级自动 fallback**：子串 → 归一化 → 分词形变 → RE2 正则；`\|` 分隔多分支 OR） | 每处命中按"最小包容单元"输出；**自动去重**（同容器多命中 → 单个容器，同表格多行命中 → 合并切片） |

> 💡 **多关键词用 `\|` 拼接（OR 语义，任一命中即返回）**：例 `"部署\|发布\|上线"`，三词任一命中都进结果，适合**同义词/别名/多业务术语**一次召回（如 `bug\|缺陷\|故障`）。

**设置 `--scope` 时共用** `--context-before` / `--context-after` / `--max-depth`。

- `--max-depth`：`outline` = 标题层级上限（3 = h1~h3）；其它模式 = 被选块的子树遍历深度（`-1` 不限，`0` 仅块自身）。
- `--context-before/--context-after`：**只对整块顶层单元生效**；命中落在容器/表格内（返回容器或切片）时 before/after 被忽略，需要更大范围改用 `section` / `range` 显式指定。

**决策顺序**（核心原则：**局部获取优于全量获取**，根据需求形态选起点，必要时多步组合收敛范围）：
1. 需求**直接给出待查的具体术语/错误码/标识** → 直接走 `keyword` 粗匹配（多级 fallback 自动覆盖形变），需要更大上下文时用返回的 `top-block-id` 走 `section` / `range`
2. 需求**指向某个章节/标题**（"修改 XX 章"、"总结第 3 节"、"关于 xx 的内容"）→ 先 `outline --max-depth 3` 拿目录 → `section --start-block-id <标题id>` 精读
3. 已知**精确起止 / 跨节连续区间** → `range`
4. **结构未知且无明确关键词/章节线索** → `outline` 探测，再回到 2/3
5. **兜底**：仅在确需整篇时才省略 `--scope`；不要为省事直接读整篇

## 局部读取的输出结构：`<fragment>` 与 `<excerpt>`

设置 `--scope` 时返回的 `content` 被一个 `<fragment>` 节点包裹，属性包含 `mode` / `requested-start` / `requested-end` / `keyword`（按需）。子节点只有两种形态：

- **顶层块**：完整块直接作为 `<fragment>` 的子节点，无额外包裹。
- **`<excerpt top-block-id="..." parent-block-path="...">`**：非顶层节选（容器整体 / 表格瘦身切片）。
  - `top-block-id`：所在顶层块 id，想看该块全貌时作 `section` / `range` 锚点再拉一次。
  - `parent-block-path`：从顶层块到 excerpt 内容直接父节点的 id 路径，`/` 分隔（表格切片时即表格自身 id）。

**看到 `<excerpt>` 即意味着这是节选**，不能假设看到了该顶层块的全貌。

**表格默认瘦身**：即便 `<table>` 本身是顶层块也只返回 thead + 命中 tr。想拿整张表 → `range --start-block-id <table-id> --end-block-id <table-id>`；切片范围恰好覆盖全部 tr 时 SDK 自动升级为整块、不包 `<excerpt>`。

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "document": {
      "document_id": "doxcnXXXX",
      "revision_id": 12,
      "content": "<title>标题</title><p>文档内容...</p>",
      "reference_map": {
        "<block_type>": {
          "<ref>": {
            "<real-attr-key>": "<real-attr-value>"
          }
        }
      },
      "tips": "<safe replay or degradation guidance>"
    }
  }
}
```

`content` 的格式由 `--doc-format` 决定。`reference_map` 是正文引用数据的结构化 sidecar：一级键 `block_type` 表示引用所在的块类型，二级键 `ref` 对应正文中的临时引用；每个引用的值是由 `real-attr-key` 和 `real-attr-value` 组成的真实属性映射，具体属性由块类型决定。没有提取数据时，`reference_map` 可能为空。`content` 和 `reference_map` 属于同一份响应，保留或回放内容时应配套处理。`tips` 给出安全回放或降级提示。`im-markdown` 仅用于获取内容后在 `lark-im` 场景下使用。设置 `--scope` 时会被 `<fragment>` 包裹，详见上文"局部读取的输出结构"。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--doc` | 是 | 文档 URL 或 token（支持 `/docx/` 和 `/wiki/`） |
| `--doc-format` | 否 | `xml`（默认）\| `markdown` \| `im-markdown`（仅用于获取内容后在 `lark-im` 场景下使用） |
| `--detail` | 否 | `simple`（默认）\| `with-ids` \| `full` |
| `--revision-id` | 否 | 文档版本号，`-1` = 最新（默认） |
| `--scope` | 否 | `outline` \| `range` \| `keyword` \| `section`（省略 = 读整篇） |
| `--start-block-id` | 否 | `range`/`section` 起始/锚点 id（`section` 必填） |
| `--end-block-id` | 否 | `range` 结束 id；`-1` 表示读到末尾 |
| `--keyword` | 否 | `keyword` 模式关键词，**4 层自动 fallback**（子串 → 归一化 → 分词形变 → RE2 正则）；`\|` 分隔多分支 OR |
| `--context-before` | 否 | 命中前拉几个兄弟块（仅对顶层单元生效，默认 `0`） |
| `--context-after` | 否 | 命中后拉几个兄弟块（仅对顶层单元生效，默认 `0`） |
| `--max-depth` | 否 | `outline` = 标题层级上限；其它 = 子树深度（`-1` 不限，默认） |
| `--inline-embeds` | 否 | 仅 markdown 整篇：展开内嵌多维表/电子表格到 GFM |
| `--full` | 否 | 仅 markdown 整篇：一次返回整篇，关闭分页 |
| `--page-token` | 否 | 仅 markdown 整篇：续读下一页；不能与 `--full` 同用 |
| `--page-size` | 否 | 仅 markdown 整篇：每页大小提示（0 = 服务端默认），不能与 `--full` 同用 |
| `--embed-max-rows` | 否 | 仅 markdown：每表最多 N 行（默认 50，0 = 不限） |
| `--format` | 否 | `json`（默认）\| `pretty` |

## 图片、文件、画板的处理

**文档中的素材以 XML 标签形式出现：**

```xml
<img token="..." url="https://..." width="..." height="..."/>
<source token="..." url="https://..." name="skills.zip"/>
<whiteboard token="..."/>
```

- `<img>` / `<source>` 带 `url` 时，直接用该 URL 下载即可（普通 HTTP GET），无需走 shortcut。
- 没有 `url`、或只想预览 → `docs +media-preview --token <token> --output ./preview_media`
- 明确下载，或目标是 `<whiteboard>`（画板只能走 shortcut） → `docs +media-download --token <token> --output ./downloaded_media`
- 文档封面图不是正文素材；下载/更新/删除封面图 → `docs +resource-download/+resource-update/+resource-delete --type cover`

## 嵌入电子表格 / 多维表格

**xml 模式**：返回中可能含 `<sheet>`、`<bitable>`、`<cite file-type="sheets|bitable">`。内部数据无法通过 `docs +fetch` 获取，提取 `token` 等属性后切到 [`lark-sheets`](../../lark-sheets/SKILL.md) / [`lark-base`](../../lark-base/SKILL.md) 下钻，详见 [SKILL.md 快速决策](../SKILL.md) 路由表。

**markdown 整篇模式**：内嵌电子表格默认展成 GFM 表；内嵌多维表格默认是 `[](token=xxx)` 占位（拿 token 去 base 技能取结构化数据），加 `--inline-embeds` 展成 GFM 表（受 `--embed-max-rows` 截断）。

## 参考

- [lark-doc-create](lark-doc-create.md) — 创建文档
- [lark-doc-update](lark-doc-update.md) — 更新文档
- [lark-doc-media-preview](lark-doc-media-preview.md) — 预览素材
- [lark-doc-media-download](lark-doc-media-download.md) — 下载素材/画板缩略图
- [lark-doc-resource-cover](lark-doc-resource-cover.md) — 读取、更新、删除文档封面图
