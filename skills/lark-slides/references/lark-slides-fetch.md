
# slides +fetch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把一份幻灯片（PPT）的**正文**读成可读 markdown，用于阅读 / 总结 / 问答 / 转写（转文档、纪要、大纲、文章），或喂给模型。按当前用户的**读权限**取内容——读不到的 deck 会明确报错，不会越权。

## 什么时候用（与其它命令的分工）

| 意图 | 用 | 说明 |
|------|----|------|
| **读懂 / 总结 / 问答 / 转写** | **`slides +fetch`** | 一次拿全 deck 的可读 markdown（标题分层、表格转 GFM、图片带文字描述） |
| 改某一页 | `slides +replace-slide` | 块级替换/插入，需要 block id |
| 要逐页**可寻址结构**（按 shape id 精确编辑） | `xml_presentations.get` / `xml_presentation.slide.get` | 返回带 id 的 SML 结构 |

> ⚠️ **`+fetch` 的输出是渲染快照，没有任何 block/shape id**，而且夹有 `[block_sep]` 分隔标记。**不能**拿它的结果去做 `+replace-slide` 的定位——要改页，先用 `xml_presentations.get` 拿带 id 的结构。

## 命令

```bash
# 读 deck 正文（默认 JSON，content 在 data.slides.content）
lark-cli slides +fetch --presentation ApU6...n0d --as user

# 直接传 URL（/slides/ 或 /wiki/ 均可，wiki 链接会自动解析为 slides）
lark-cli slides +fetch --presentation "https://xxx.larkoffice.com/slides/ApU6...n0d" --as user
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/` URL，或解析到 slides 的 `/wiki/` URL |
| `--image-urls` | 否 | 图片渲染：`none`（仅描述）｜ `one`（默认，单链接 + 宽高）｜ `full`（全部路由） |
| `--embed-max-rows` | 否 | 内嵌表每张截断到 N 行（默认 50，`0` = 不限） |

## 行为说明

- 返回体含 `slides.content`（markdown）、`slides.title`、`slides.update_time`，以及 `source: "eqa_slides_fetch"` 标记该路径。
- 只返回**当前用户有读权限**的 deck 正文；无权限、类型不支持或服务不可用时返回明确报错并提示重试 / 去 Lark 打开，不给半截结果。

## 参考

- [lark-slides](../SKILL.md) — 幻灯片全部命令与决策路由
- [lark-slides-replace-slide](lark-slides-replace-slide.md) — 块级修改已有页面
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
