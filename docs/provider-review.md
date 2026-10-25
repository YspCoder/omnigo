# 服务商检查记录

检查日期：2026-10-24。范围覆盖注册表中的 11 个调用服务商，以及 `newapi`、`newapi_ark` 两个素材库适配器。依据为当前实现、已有 SDK 类型与下列官方资料；没有使用真实密钥调用收费 API。构建验证不代表账号权限、模型可用性或端到端生成已验证。

本文记录 10 月 24 日的历史基线。10 月 25 日已继续补齐流式、工具和新版媒体协议；当前能力与用法请以 [服务商能力与迁移说明](provider-capabilities.md) 为准，下面“需要单独规划的能力”保留历史检查结果。

## 本轮更新

| 服务商 | 检查结论与修改 |
| --- | --- |
| OpenAI | 图片对话默认走 Chat Completions，避免视觉流式误入尚未实现的 Responses 流式；修复结构化消息文本丢失、显式 `max_completion_tokens` 与默认 `max_tokens` 冲突，以及 Responses 命名工具选择格式。文件输入和显式 `responses` 配置仍走 Responses。 |
| Groq | 复用上述 OpenAI 兼容协议修复；保留现有 endpoint。模型能力由上游决定，不切换所有模型的默认 token 参数。 |
| Moonshot / Kimi | 复用上述修复，图片不再自动请求不兼容的 Responses 路由；保留国内默认 endpoint，海外账号需显式配置地址。 |
| Anthropic / Claude | 应用自定义 Headers、合并系统消息、拼接全部文本块，避免空回复崩溃；保留回复 ID、模型与结束原因。 |
| Ali / DashScope | 修复图片参数写入 nil map 的崩溃；为明确支持旧协议的 Wan 2.1/2.2/2.5/2.6 恢复 `img_url`、`first_frame_url` / `last_frame_url`、`reference_urls` 及对应路径，保留 Wan 2.7 的现有 `media` 路由。 |
| Google / Gemini | 应用自定义 HTTP client、超时、代理和 Headers；将 assistant 历史角色映射为 model；普通与流式回复保留完整文本、候选、结束原因和用量，跳过思考文本；补齐官方图片比例及 `512` 尺寸。示例从环境变量读取账号可用模型。 |
| Volcengine Ark | 修复图片 Size/Resolution 取值错误；视频支持直接 `Prompt`，保留结构化消息解析。删除误导性的 Jimeng 支持说明，将不可用示例改为 Ark 视频示例。 |
| Vidu | 按官方协议使用 `Authorization: Token`；转义任务 ID；已核实的新模型支持直接参考媒体，旧模型和未知模型保留主体包装行为。 |
| Kling | 保留官方档位 `mode=std/pro/4k`，通过 `task_type` 选择本地能力；legacy 视频 duration 发字符串；HTTP 成功但业务失败时返回错误，任务列表全失败时传播错误。 |
| Pai / PixVerse | 修复数值图片 ID、首尾帧自动路由、多帧请求丢 model/quality、Modify 的 `keyframe_id`；请求追踪 ID 使用 UUID。最新 v6-max/c1 名称可由调用方传入，不增加模型白名单。 |
| Custom | 应用 HTTP 配置；轮询支持配置级 `PollingURL`，单次 endpoint 优先；JSON 数值 ID 保留精度，避免大整数任务 ID 被浮点转换破坏。 |
| NewAPI | 复用现有 HTTP 配置逻辑，修复配置代理或自定义 client 同时指定超时时失效的问题；未改私有网关协议。 |
| NewAPI Ark | 同上；修复完整 `/v1/ark/asset` 地址被错误拼到素材 REST 路径前的问题；说明素材组 Action 与素材 REST 的实际区别。 |

共享媒体提示词现在优先使用 `MediaRequest.Prompt`，空值时回退到用户消息，并继续保留系统提示词。任务状态归一化补充 Vidu 的 `created/queueing` 和 Kling 的 `succeed`。多个 HTTP 适配器复用已有 `providerHTTPClient`，没有引入依赖或扩大自动重试范围。

验证：`go build ./...` 通过；完成变更差异和旧协议兼容性审阅。按项目约定提交前仅执行构建，没有运行测试套件。只扩充一个已有 Anthropic 回归测试，没有删除测试。

## 需要单独规划的能力

- OpenAI：Responses 流式、官方视频 API 尚未实现。README 已去掉会直接返回“不支持”的 OpenAI 视频示例。
- Anthropic：当前适配器仍只实现普通文本调用；流式、工具调用与原生多模态尚需独立接入。
- Google：最新图像能力中的 Interactions 协议需要独立适配；当前实现覆盖 GenerateContent、Imagen 与 Veo 路径，不宣称支持所有新模型。
- Ali：Wan 2.7 的驱动音频、任意原生 `media` 字段以及新计费用量字段需要进一步接入和联调；本轮没有把新协议套到旧模型。
- Ark：现有 Seedance 字段已检查，但账号权限、不同模型的参考素材限制尚未实测。Jimeng Visual 使用独立的 AK/SK 签名与 Action 协议，不能将 `jimeng` 简单映射到 `ark`。
- Kling：官方同时提供新版模型路径及 `/tasks` 协议；本轮修复当前仍有官方文档的 legacy `/v1/videos/...` 适配。新版迁移需要单独设计，不能直接替换路径。
- Custom / NewAPI：没有本项目所用网关的可核对公开协议或实例地址。本轮仅修复可从代码确认的问题；字段、鉴权覆盖及网关版本兼容性仍需结合实际部署确认。

模型 ID 继续由调用方指定，不统一替换默认模型，也不更新 SDK 依赖。服务商上线新模型不一定要求 SDK 增加枚举；是否需要接入取决于请求路径、字段和返回协议是否变化。

## 官方依据

- OpenAI：[Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Responses](https://developers.openai.com/api/reference/resources/responses/methods/create)、[工具调用](https://developers.openai.com/api/docs/guides/function-calling)。
- Groq：[OpenAI 兼容说明](https://console.groq.com/docs/openai)、[API 参考](https://console.groq.com/docs/api-reference)。
- Moonshot：[Chat API](https://platform.moonshot.ai/docs/api/chat)。
- Anthropic：[Messages](https://platform.claude.com/docs/en/api/messages/create)。
- Google：[GenerateContent API](https://ai.google.dev/api/generate-content)、[图像生成](https://ai.google.dev/gemini-api/docs/image-generation)。
- Ali：[文生视频](https://help.aliyun.com/zh/model-studio/text-to-video-api-reference)、[新版图生视频](https://help.aliyun.com/zh/model-studio/image-to-video-general-api-reference)、[旧版图生视频](https://help.aliyun.com/zh/model-studio/legacy-image-to-video-api-reference)、[旧版首尾帧](https://help.aliyun.com/zh/model-studio/legacy-image-to-video-by-first-and-last-frame-api-reference)、[旧版参考生视频](https://help.aliyun.com/zh/model-studio/legacy-wan-reference-to-video-api-reference)、[万相图像](https://help.aliyun.com/zh/model-studio/wan-image-generation-api-reference)。
- Ark：[官方图像 SDK 类型](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/model/images.go)；Jimeng 协议区别依据已安装的官方 SDK，对应[视觉服务配置源码](https://github.com/volcengine/volc-sdk-golang/blob/main/service/visual/config.go)。
- Vidu：[任务查询](https://platform.vidu.com/docs/api-reference/task-management/task-detail)、[Q1](https://platform.vidu.com/docs/api-reference/video-models/vidu-q1/reference-to-video)、[Q2](https://platform.vidu.com/docs/api-reference/video-models/vidu-q2/reference-to-video)、[Q3](https://platform.vidu.com/docs/api-reference/video-models/vidu-q3/reference-to-video)、[Q4 Preview](https://platform.vidu.com/docs/api-reference/video-models/vidu-q4-preview/reference-to-video)。
- Kling：[官方接口入口](https://app.klingai.com/global/dev/document-api/apiReference/model/textToVideo)，其前端加载的 [legacy 协议资源](https://s15-kling.klingai.com/kos/s101/nlav112918/api-doc/assets/api-CWSsgT8g.js)及[新版协议资源](https://s15-kling.klingai.com/kos/s101/nlav112918/api-doc/assets/api-DKVHjUJH.js)。
- PixVerse：[图生视频](https://docs.platform.pixverse.ai/image-to-video-and-video-templates-13016633e0.md)、[多帧视频](https://docs.platform.pixverse.ai/multi-transition-video-43051215e0.md)、[Modify](https://docs.platform.pixverse.ai/video-edit-modify-43051223e0.md)。Modify 官方文档已标记 released。
