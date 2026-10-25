# 服务商用量审计与修复记录

日期：2026-10-25。审计基线：`main` / `92012de`；本文描述本轮修复后的行为。

同日以 `6f0dd9d` 为基线追加复查，修复结果见下方“追加复查”。

同日继续补齐新能力；当前调用方法与协议边界见 [服务商能力与迁移说明](provider-capabilities.md)。

范围覆盖全部 11 个内置调用服务商，以及 NewAPI、NewAPI Ark 两个素材库适配器。已修复已确认的用量映射、流式终态、默认参数及错误传播问题。没有新增依赖或升级 SDK，也没有调用真实收费 API。

## 各服务商修复结果

| 服务商 | 修复后的行为 | 仍需区分的边界 |
| --- | --- | --- |
| OpenAI | Chat、Chat Stream、Responses 普通/流式、图片生成/编辑普通/流式及兼容网关任务查询保留 token、明细和原生 usage；Responses 截断保留 incomplete 状态及 length 结束原因 | 官方 Sora / Videos API 已于 2026-09-24 下线，未启用 |
| Groq | 复用 Chat/Stream 映射；兼容顶层 usage 与 x_groq.usage，保留 usage_breakdown 等扩展 | 不能仅因共用 OpenAI 适配器就推断官方支持图像/视频生成 |
| Moonshot / Kimi | Chat/Stream 保留三项 token、缓存/推理明细及原生 usage | 汇总已经包含缓存/推理，不再次相加 |
| Anthropic / Claude | 普通/流式输入合计包含普通输入、缓存读取和缓存创建；保留 SDK 用量细项；支持工具及原生图片/文档输入 | 不生成图片/视频；SDK 无法保留它未识别的未来字段 |
| Ali / DashScope | 兼容 Chat/Stream 保留 token；创建、同步图片、任务查询、任务列表、媒体流统一解析原生 usage；支持 input/output 别名及媒体计费字段 | 图片模型可按张计费；视频用量不应强行转换成 token |
| Google / Gemini | GenerateContent 普通/流式、原生图片流及 Interactions 保留用量；补齐工具与签名回传、Imagen 结果事件及 Veo 轮询流 | 当前 Imagen/Veo SDK 没有 token usage 字段 |
| Volcengine Ark | 普通文本、文本流、图片、图片流、视频任务查询及视频流保留用量；失败任务有实际用量时不再清空；图片错误正确传播 | SDK 的值类型字段不能完全区分原生字段缺失与显式零 |
| Vidu | 任务查询、任务列表及视频流保留 credits | 当前核对的协议报告积分，不报告 token |
| Kling | legacy 保留原扣费字段；新版模型路径与 /tasks 查询/列表/流式保留 billing、usage、多结果和游标 | 新协议显式选择，不自动改写旧模型和路径 |
| Pai / PixVerse | 创建及 mask 响应保留 credits；视频流首次即返回创建用量，终态仍保留快照 | 当前 ListTasks 为 restyle 风格目录，没有任务计费字段；查询接口不会凭空补出创建响应的 credits |
| Custom | 标准 token 和 input/output 别名统一映射；空 usage 不再遮蔽内层；显式零保留；多层 data 结果可读取 | 保持首个非空外层 usage 优先，不跨层累计；私有协议仍需按部署实例联调 |
| NewAPI / NewAPI Ark 素材库 | 构造时快照配置、复制 Headers、准备共享 HTTP transport | 素材 CRUD 无需生成 token 用量；私有网关扩展未端到端验证 |

## 统一用量结构

原有 PromptTokens、CompletionTokens、TotalTokens 三个整数和 TaskStatusUsage 的 VideoDuration、VideoCount、SR 字段保留，增加共享 UsageDetails：

- `prompt_tokens_details` / `completion_tokens_details`：缓存、推理及模态等细分。
- `billing`：credits、扣费单位、余额扣费、时长、生成数量等，保留原字段值和单位，不转换成 token。
- `raw_usage`：服务商或 SDK 提供的用量载荷，保留未归一化细项及显式零值。它不是完整 HTTP 响应；原生 SDK 可能已丢弃未知字段。

MediaResponse、TaskStatusResponse、StreamToken 和可用的 TaskListItem 均传递对应结构。`GenerateResponse.Raw` 仍是归一化 MediaResponse；完整用量载荷应读取其中的 `Usage.RawUsage`。

`Usage.HasUsage()` 用于区分是否有报告用量。没有用量时 MediaResponse JSON 省略 usage；上游明确报告零值时，通过 raw_usage 保留。Custom 的空对象按无有效用量处理；非 nil SDK 用量结构可能因 omitempty 编码为 {}，仍保留该对象存在的信息。无法从 SDK 值类型重建已经丢掉的逐字段存在性。

流中的 usage 均按快照消费：更新之前的快照，不把每次 progress/usage/终态的数值相加。终态用量随结果或 usage 事件在 EOF 前发出；失败但有用量时，先返回用量和 nil error，下一次 Next 返回错误，再次调用返回 EOF。媒体流共用终态错误处理，避免适配器重复实现。

兼容性注意：结构新增 map/RawMessage 明细，因此 Usage 和 TaskStatusUsage 不再支持直接用 Go `==` 比较；调用方应按需要比较计数字段或使用深比较。没有用量时 JSON 从 `usage: {}` 改为省略该字段。原整数访问方式保持不变。

## 其他已修复问题

- 文本 Media 未设置 MaxTokens 时继承客户端默认值，避免 Claude 收到 max_tokens=0。
- OpenAI Responses 的 incomplete/max_output_tokens 返回 length，不再误报 completed。
- Ark 图片流保留错误码/信息；带用量的失败事件先交付用量，再传播错误。
- Kling 的任务 data 缺失、null 或类型异常不再被吞掉。
- Custom 多层包装中的任务 ID、状态和结果 URL 可以被读取。
- NewAssetClient 不再直接持有调用者配置对象，代理 transport 可复用。
- StreamOption 支持正数 BufferSize 有界预读和 RetryStrategy 创建失败重试；零缓冲保持按需读取，负数报错。已经建立的流不自动重放；关闭和取消会释放底层流。

## 尚未实现与未验证

上述 Responses/图片流、Anthropic 多模态/工具/流、Google/Ark 工具、Google Interactions、Wan 原生 media/驱动音频、Kling 新协议均已补齐。运行时 RegisterProvider 工厂注册原本就已实现，不能再列为缺失能力。

以下仍属于外部验证或协议边界：

- 官方已退役的 OpenAI Videos API；私有兼容网关是否仍提供视频需按实例核验。
- Custom/NewAPI 私有协议及真实账号的权限、模型差异与计费结果。
- 上游没有提供的 token、Imagen/Veo SDK 未暴露的用量、SDK 未识别的未来字段，均不能靠本地映射补全。

## 追加复查

- Ark 多图流中的 `image_generation.partial_failed` 是单张失败，现返回带错误码的 `Type=error` 事件并继续读取其他图片及最终用量；整条流失败仍以 Go error 报告。
- Ali、Ark、Vidu、Kling 在状态不变但用量变化时也会发出快照；最终响应省略用量时保留最近一次报告。Kling 创建阶段的用量会在首次 Next 发出，避免首个查询失败时丢失。
- OpenAI Responses 失败响应保留用量和错误字段，返回非 nil 响应与 error；LLM 的 GenerateWithResponse、文本 Media 同样保留响应，Generate 保留可用的部分文本。无候选但有用量的响应也不再被丢弃。调用方处理错误时仍可检查非 nil 响应中的用量。
- Pai 的 `auto_mask_selection=false` 不再触发辅助 mask 请求；自动选区产生的扣费通过 `billing.steps` 逐步骤保存，即使后续主请求失败也可读取。顶层 `billing.credits` 仍表示主请求消耗，辅助步骤是额外消耗，不能仅按顶层 credits 结算。RawUsage 保留操作名称及各步骤的来源。
- Pai 同步 mask 的 StreamMedia 直接返回结果与用量；辅助步骤已扣费、后续创建失败时，先返回用量事件，再返回错误。
- 已归一化用量通过 Custom 转发时保留 billing 与原始 raw_usage，避免每次转发重复嵌套；`raw_usage:null` 和只有 null 计数的对象不再误判为已报告用量，明确的零仍保留。

追加回归覆盖单张失败后的剩余图片、同状态下的新用量与终态回退、失败响应的公开入口透传、Pai 辅助调用次数和失败计费，以及用量重复转发后的稳定性。以上使用模拟 HTTP 响应验证。

新能力轮次另修复了 `omnigo` 最外层 GenerateWithResponse 对错误响应的丢弃，并保留 Generate 的部分文本；结构化 thinking/tool 内容不会被 fmt.Sprint 当作可见文本。新增工具签名 Metadata、任务列表 NextCursor、任务原生 Outputs 与结构化 Result。OpenAI 与 Google Interactions 共用 SSE 解码，保留 llm 原有解码 API。

文本默认 MaxTokens 仅应用于文本请求，避免把低文本预算套到 Google 图像生成。响应 Metadata 保留 Interactions 原生步骤，ImageData.MIMEType 保留图像格式。

## 验证

使用已有假 HTTP helper 扩展少量回归，覆盖 OpenAI 图片 10/20/30 token 及模态明细、Claude 缓存合计 100255、Responses 截断、Custom 别名/空对象/显式零/多层结果、Ark 失败任务保留用量、流终态先用量后错误、文本 completed 用量只发一次，以及默认 MaxTokens 不修改调用者请求。

共享用量测试覆盖缺失、空对象、显式零、规范字段优先、非 token 计费原值及小数时长。VideoDuration 为原有整数类型，小数部分完整保存在 Billing 中。

针对本轮变更的 adapter、llm 回归及本地假 HTTP 复现通过；提交前执行 `go build ./...` 和差异检查。未运行真实收费 API，构建和模拟响应验证不代表所有模型都已端到端联调。

原有未跟踪文件 dto/task_status_test.go 未修改、未纳入提交。

## 官方依据与 SDK 基线

- OpenAI Docs：[Images](https://developers.openai.com/api/reference/resources/images/methods/generate)、[Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Responses](https://developers.openai.com/api/reference/resources/responses/methods/create)。
- Claude：[Prompt Caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)、[Messages](https://platform.claude.com/docs/en/api/messages/create)。
- Groq：[API Reference](https://console.groq.com/docs/api-reference)。Moonshot：[Chat API](https://platform.moonshot.cn/docs/api/chat)。
- Google：[GenerateContent](https://ai.google.dev/api/generate-content)、[图像生成](https://ai.google.dev/gemini-api/docs/image-generation)。
- Ali：[万相图像](https://help.aliyun.com/zh/model-studio/wan-image-generation-api-reference)、[文生视频](https://help.aliyun.com/zh/model-studio/text-to-video-api-reference)。
- Ark：[图像 SDK 类型](https://github.com/volcengine/volcengine-go-sdk/blob/v1.2.45/service/arkruntime/model/images.go)，同时核对本地 Responses、视频任务类型。
- Vidu：[任务详情](https://platform.vidu.com/docs/api-reference/task-management/task-detail)。PixVerse：[图生视频 OpenAPI](https://docs.platform.pixverse.ai/image-to-video-and-video-templates-13016633e0.md)。
- Kling：[文生视频](https://app.klingai.com/global/dev/document-api/apiReference/model/textToVideo)及其[官方文档资源](https://s15-kling.klingai.com/kos/s101/nlav112918/api-doc/assets/api-CWSsgT8g.js)。

SDK：openai-go/v3 v3.47.0、go-anthropic/v2 v2.24.0、volcengine-go-sdk v1.2.45、genai v1.66.0。未升级依赖。
