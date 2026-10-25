# 服务商能力与迁移说明

更新日期：2026-10-25。保留既有默认路由，不升级 SDK 或增加依赖。以下是本轮补齐的调用能力；模型是否接受某项参数仍由对应服务商及账号权限决定。

| 服务商 | 新增能力 | 入口 |
| --- | --- | --- |
| OpenAI | Responses 文本与工具流、图片生成／编辑流 | `SetChatProtocol("responses")` + `Stream`；图片使用 `StreamMedia` |
| Anthropic | 工具调用与结果回传、图片／文档输入、文本与工具流 | 文本 `Media` / `StreamMedia`，或 `GenerateWithResponse` / `Stream` |
| Google | 工具调用与签名回传、Gemini 原生图片流、Imagen 结果事件、Veo 轮询流 | 统一 `Tools`、`ToolChoice`、`ToolCalls`；`StreamMedia` |
| Google | 现行 Interactions 的文本／图像、流式与后台任务查询 | `SetChatProtocol("interactions")`，或请求 `Options["api"]="interactions"` |
| Ark | Responses 工具声明、选择、历史调用／结果、工具流 | 统一 `Tools`、`ToolChoice`、`ToolCalls` |
| Ali | Wan 原生 `media`、驱动音频 | 视频请求 `Extra["media"]` / `Extra["driving_audio"]` |
| Kling | 新版模型路径、`/tasks` 查询与游标列表、原生计费及多结果 | `Extra["protocol"]="tasks"`；查询使用 `query["protocol"]="tasks"` |

Groq、Moonshot 继续使用现有 OpenAI 兼容聊天协议。Vidu、Pai 和 Custom 的既有媒体接口保留。`adapter.RegisterProvider` 已支持运行时注册自定义工厂；声明 `ProviderSpec.Capabilities` 可让公开入口提前检查支持的操作。

## OpenAI

`SetChatProtocol("responses")` 现在可用于普通和流式文本。流事件保留文本、工具 ID／名称／参数分片、结束原因、终态用量。失败但有用量时先交付用量，再返回错误；截断的流不能当作正常完成。

图片 `StreamMedia` 支持生成和编辑，部分图和最终图以事件返回。调用方需要同时处理事件类型、图片数据和用量，不能把所有 `Text` 当作自然语言。`Extra["partial_images"]` 可传官方支持的部分图数量。

**官方 Sora / Videos API 已于 2026-09-24 下线**，因此没有把 SDK 残留的视频类型注册为可用能力。现有自定义网关 `PollingURL` 查询仍保留。

## 工具调用与多模态输入

通过 `MediaRequest.Tools` 声明函数，`ToolChoice` 支持 `auto`、`none`、`required` 或具名函数。回复中的 `Message.ToolCalls` 记录函数名、JSON 参数和调用 ID；随后追加原 assistant 消息及 `Role:"tool"`、`ToolCallID` 对应的结果消息，再次调用文本 `Media`。

Google 返回的 `ToolCall.Metadata["thought_signature"]` 必须随原调用回传。`dto.ToolCall`、流式 `ToolCallDelta` 和公开 `omnigo.ToolCall` 均保留 Metadata。不要只复制函数名和参数。Google Gemini API 流返回完整函数参数块；Ark、Claude 和 OpenAI 可能按分片返回。按候选 `token.Index` 和工具 `call.Index` 分组拼接参数，JSON 完整后再执行。

Claude 支持 `Message.ImageURL`（HTTP(S) 或 base64 data URL）、`Message.FileURL`（文档），以及原生 content block 数组。响应中含 thinking／签名等原生内容时，完整保留在 `Message.Content`，便于下一轮原样回传；公开 `Generate` 只返回可见文本。Claude 不生成图片／视频，理解多模态输入仍使用文本请求类型。

原生结构化内容的往返请使用 `Media(Type:text)` 与 `dto.Message.Content`。`PromptMessage.Content` 保持原有 string 类型，不能承载 thinking／多模态块。

## Google Interactions

显式选择 Interactions 时，统一文本和图片请求使用 `/v1beta/interactions`；原 GenerateContent／Imagen／Veo 路由继续作为默认。现行协议使用 `steps` 和 `step.start` / `step.delta` / `step.stop`，没有按旧 `outputs` 格式处理。

`Extra` 可指定原生 `input`、`previous_interaction_id`、`store`、`background`、`generation_config`、`response_format` 和 `tools` 等字段。后台返回的 TaskID 带 `interactions/` 前缀，可直接传给 `TaskStatus`。`Extra["api_version"]` 可显式选择文档中的 `v1beta2` 版本。

普通响应的 `Metadata["steps"]` 保留完整原生步骤（含思考签名和服务端工具），流中的原始事件保留在 `Metadata["event"]`。无状态多轮需将完整历史步骤放回 `Extra["input"]`，或使用 `previous_interaction_id` 延续服务端状态。图像保留 MIME 类型；图片请求不会继承文本默认 MaxTokens，需要限制图像生成预算时显式填写请求的 MaxTokens。

## Ali 原生素材

Wan 2.7 图生视频可指定首帧、尾帧、首段视频、驱动音频。显式 `media` 优先于从消息推导的素材，原生字段保留：

```go
request := &dto.MediaRequest{
    Type: dto.MediaTypeVideo,
    Model: "wan2.7-i2v-2026-04-25",
    Prompt: "角色随音乐舞动",
    Extra: map[string]interface{}{
        "media": []map[string]interface{}{
            {"type": "first_frame", "url": "https://example.com/frame.png"},
            {"type": "driving_audio", "url": "https://example.com/music.mp3"},
        },
    },
}
```

也可使用 `Extra["driving_audio"]` 传音频 URL，与推导的图像组合。Wan 2.6 及更早模型保留旧字段；不会静默丢弃误传给旧模型的原生 `media`，而是返回参数错误。素材组合、时长和大小限制由服务端验证。

## Kling 新协议

创建请求显式设置 `Extra["protocol"]="tasks"`，模型使用新版原生名称（例如 `kling-3.0`）；文生视频走 `/text-to-video/{model}`，有图输入时走 `/image-to-video/{model}`。可以用 `Extra["task_type"]` 明确选择，或用 `Extra["endpoint"]` 指定官方新版模型路径。旧 `kling-v*` 名称不会被自动猜测转换。

原生 `settings`、`options`、`contents` 或完整 `Extra["payload"]` 可用于复杂请求。查询需携带相同协议选择：

```go
status, err := client.TaskStatus(ctx, taskID, map[string]string{"protocol": "tasks"})
page, err := client.ListTasks(ctx, map[string]string{
    "protocol": "tasks", "limit": "100", "cursor": nextCursor,
})
```

流会记住创建时所用协议。`Billing["billing"]` 保留原生扣费数组，`Billing["usage"]` 保留 `input_duration` 等原生计费指标；金额、币种和小数时长均不转换为 token。

## 流式缓冲与重试

`StreamOption` 的 BufferSize 现支持有界预读；零保留按需读取。负数返回错误。取消 `Next` 的 context 或 `Close` 会中止预读并关闭底层流。

```go
stream, err := client.Stream(ctx, prompt, func(c *omnigo.StreamConfig) {
    c.BufferSize = 8
    c.RetryStrategy = &omnigo.DefaultRetryStrategy{
        MaxRetries: 2, InitialWait: time.Second, MaxWait: 5 * time.Second,
    }
})
```

重试仅发生在 adapter 创建流返回错误时；已返回的流发生读取错误会向调用方传播，不重新生成或拼接不同请求的结果。某些 SDK 延迟到首次 Next 才建立 HTTP 连接，这类错误也作为读取错误返回。策略实例不能被多个并发 Stream 调用共享。OpenAI 客户端自身的 HTTP 重试仍独立生效，配置时应计入总尝试次数。

## 用量与结果约定

用量是快照，后续事件替换之前的快照，不能累加重复 progress／finish 的值。上游未提供的 token 不补估算。非 token 扣费保留在 `Billing`，原始用量在 `RawUsage`。最外层 `GenerateWithResponse` 在失败时仍可能返回非 nil 响应，`Generate` 也保留部分文本；处理 error 时仍应检查返回值。

Kling 新协议列表保留 `NextCursor` 和 `HasMore`，任务查询的 `Output.Outputs` 保存全部原生结果；原单 URL 字段继续取第一个可用结果。Google Interactions 后台查询的 `Output.Result` 保存统一文本、图像、工具调用及用量。

## 验证与来源

通过本地模拟 HTTP／SSE 验证新增协议、工具往返、失败用量和取消行为，并执行 `go build ./...`。没有使用真实密钥调用收费 API，私有网关版本及账号权限仍需部署实例验证。

`go test ./adapter ./llm ./relay ./utils . -count=1 -timeout=60s` 通过。缓冲流生命周期及 OpenAI／Interactions 取消路径另通过竞态检测。自定义 HTTP Transport 即使不响应请求 context，新增 HTTP 流仍会主动关闭响应体以中断读取。

Interactions 的音频、文档及服务端工具结果通过原生步骤／事件保留，尚未全部转换为独立公共结果类型；Imagen / Veo SDK 未提供的 token 用量仍保持缺失。

- [OpenAI Responses](https://developers.openai.com/api/reference/resources/responses/methods/create)、[图片生成](https://developers.openai.com/api/reference/resources/images/methods/generate)、[已退役的视频 API](https://developers.openai.com/api/reference/resources/videos/methods/create.md)。
- [Claude Messages](https://platform.claude.com/docs/en/api/messages/create)。
- [Google Interactions](https://ai.google.dev/gemini-api/docs/interactions-overview)、[GenerateContent](https://ai.google.dev/api/generate-content)。
- [Ali 新版图生视频](https://help.aliyun.com/zh/model-studio/image-to-video-general-api-reference)。
- Kling 官方[新版创建协议](https://s15-kling.klingai.com/kos/s101/nlav112918/api-doc/assets/api-DKVHjUJH.js)与[任务查询协议](https://s15-kling.klingai.com/kos/s101/nlav112918/api-doc/assets/query-task-endpoints-BaBh8lnN.js)。
