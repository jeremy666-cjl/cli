
# drive +fetch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

把飞书云空间（云盘/云存储）里的**文件内容**读成可读 markdown，用于阅读 / 总结 / 喂模型。

与 `drive +download` 的分工：
- **读内容** → `drive +fetch`：返回提取后的可读文本（markdown），不落地文件。
- **取原始文件字节 / 存到本地** → `drive +download`。

`drive +fetch` 经知识服务读取，按当前用户的**读权限**取内容——所以连「**可阅读但禁止下载/复制**」的文件也能读到内容（这类文件 `drive +download` 会因无下载权限失败）。

## 命令

```bash
# 用 file token 读内容（直接输出到 stdout）
lark-cli drive +fetch --file-token boxcn_xxx

# 或直接传文件 URL
lark-cli drive +fetch --url "https://xxx.feishu.cn/file/boxcn_xxx"

# JSON 输出（content 在 data.file.content，并带 source: eqa_drive_fetch）
lark-cli drive +fetch --file-token boxcn_xxx --format json
```

## 行为说明

- 返回体含 `file.content`（markdown）、`file.title`、`file.update_time`，以及 `source: "eqa_drive_fetch"` 标记该路径。
- 内嵌表渲染为 GFM，可用 `--embed-max-rows N`（0 = 不限）截断；图片渲染用 `--image-urls none|one|full`。
- 文件类型不被支持、或无读权限时，返回明确报错并提示改用 `drive +download` 取原始字节；不会给半截结果。

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-drive-download](lark-drive-download.md) -- 下载原始文件字节到本地
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
