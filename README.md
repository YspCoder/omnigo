# omnigo - Go 语言 LLM 集成工具包

`omnigo` 是一个面向 Go 的 LLM 集成工具包，核心是统一的 adapter/relay 架构：

- 对外提供稳定统一的调用接口
- 对内隔离不同模型服务的差异
- 支持流式输出、结构化输出、工具调用等常见能力

它适合作为你业务中 LLM 访问层的基础组件。

## 目录

- [特性](#特性)
- [支持的服务商](#支持的服务商)
- [安装](#安装)
- [快速开始](#快速开始)
- [快速参考](#快速参考)
- [高级用法](#高级用法)
- [最佳实践](#最佳实践)
- [项目状态](#项目状态)
- [贡献](#贡献)
- [许可证](#许可证)

## 特性

- **统一调用 API**：屏蔽不同服务商的请求格式差异。
- **可扩展的 Provider Registry**：内置常用文本与多媒体服务商，支持自定义扩展。
- **流式与非流式统一处理**：一套接口支持 streaming 与非 streaming。
- **结构化输出与校验**：支持 JSON Schema 校验与提示词结构化。
- **可配置性强**：支持环境变量加载 + 代码选项式配置。
- **日志与重试**：内置日志级别与重试策略。

## 支持的服务商

当前内置 Provider Spec（可扩展）：

- OpenAI (`openai`)
- Groq (`groq`)
- Moonshot (`moonshot`)
- DeepSeek (`deepseek`)
- Mistral AI (`mistral`)
- OpenRouter (`openrouter`)
- Ollama (`ollama`)
- Anthropic (`anthropic`)
- Cohere (`cohere`)
- Ali / DashScope (`ali`)
- Google / Gemini (`google`)
- Google / Gemini OpenAI compatibility (`google-openai`)
- Volcengine Ark (`ark`)
- Vidu (`vidu`)
- Kling AI (`kling`)
- Pai / PixVerse (`pai`)
- 第三方 OpenAI-compatible base URL (`custom-openai`)
- 第三方完整 URL (`custom`)

> 说明：以上名称为 `SetProvider(...)` 传入值。

`custom-openai` 适用于兼容 OpenAI API 的文本服务，必须通过 `SetEndpoint` 传入 base URL（例如 `https://example.com/v1`）；它支持对话、流式输出和工具调用。媒体任务若要求传入完整创建 URL，继续使用 `custom`。

### Kling AI 示例

```go
videoReq := &dto.MediaRequest{
    Type:     dto.MediaTypeVideo,
    Model:    "kling-v2-6",
    Duration: 5,
    Size:     "16:9",
    Messages: []dto.Message{
        {Role: "user", Content: "一只可爱的小兔子，戴着眼镜，坐在桌边，看报纸"},
    },
    Extra: map[string]interface{}{
        "mode":  "text-to-video",
        "sound": "on",
    },
}

llm, err := omnigo.NewLLM(
    omnigo.SetProvider("kling"),
    omnigo.SetModel("kling-v2-6"),
    omnigo.SetAPIKey(os.Getenv("KLING_API_KEY")),
)
```

### Pai / PixVerse 示例

```go
videoReq := &dto.MediaRequest{
    Type:     dto.MediaTypeVideo,
    Model:    "v6",
    Duration: 5,
    Size:     "16:9",
    Messages: []dto.Message{
        {Role: "user", Content: "一只机械狐狸在雪夜森林里奔跑，镜头跟拍"},
    },
    Extra: map[string]interface{}{
        "quality":    "540p",
        "water_mark": false,
    },
}

llm, err := omnigo.NewLLM(
    omnigo.SetProvider("pai"),
    omnigo.SetModel("v6"),
    omnigo.SetAPIKey(os.Getenv("PAI_API_KEY")),
)
```

当前 `pai` 适配已支持这些视频能力：

- `text-to-video`
- `image-to-video`
- `transition`
- `extend`
- `swap`
- `multi-transition`
- `mimic`
- `lip-sync`
- `mask-selection`
- `sound-effect`
- `restyle`
- `restyle` list query
- `modify`

说明：

- `image-to-video`、`transition`、`mimic`、`multi-transition` 在缺少 `img_id` 时会自动上传图片。
- `extend`、`swap`、`mimic`、`lip-sync` 需要通过 `extra` 传入官方接口要求的 `source_video_id` 或 `video_media_id`。
- `mask-selection` 是同步辅助接口，当前会把官方返回 JSON 放到 `MediaResponse.Text`，并把 `keyframe_url` 映射到 `MediaResponse.URL`。
- `swap` 支持 `extra.auto_mask_selection=true`，会先调用 `mask-selection` 自动补 `keyframe_id` 和首个 `mask_id`。
- `ListTasks(..., map[string]string{\"mode\": \"restyle\"})` 现在会返回 Pai 官方 `restyle/list` 中的可用风格项。
- `modify` 已接入，但官方文档目前仍标记为 `developing`，建议优先在真实环境做一次验证后再在生产路径使用。

### 第三方完整 URL 示例

协议为扁平 JSON、使用 Bearer Token 的异步任务接口，可以统一走 `custom`，无需为每个服务商新增 adapter。`SetEndpoint` 必须传创建任务的完整 URL，而不是只传域名：

```go
client, err := omnigo.NewLLM(
    omnigo.SetProvider("custom"),
    omnigo.SetModel("seedance-2.0-fast-480p"),
    omnigo.SetEndpoint("https://ai.xxxx.cn/v1/videos"),
    omnigo.SetAPIKey(os.Getenv("API_KEY")),
)
if err != nil {
    log.Fatal(err)
}

resp, err := client.Media(context.Background(), &dto.MediaRequest{
    Type:     dto.MediaTypeVideo,
    Prompt:   "雨夜霓虹街道，镜头缓慢推进，电影感光影",
    Duration: 8,
    Extra: map[string]interface{}{
        "aspect_ratio": "16:9",
    },
})
if err != nil {
    log.Fatal(err)
}

status, err := client.TaskStatus(context.Background(), resp.TaskID)
```

任务状态可以统一归一化，业务端无需维护不同服务商的字符串白名单：

```go
normalized, err := dto.NormalizeTaskStatus(status.Output.TaskStatus)
if err != nil {
    // normalized 仍保留上游原始值，可记录后再决定兼容策略。
    log.Fatalf("unsupported task status %q: %v", normalized, err)
}

switch normalized {
case dto.TaskStatusQueued, dto.TaskStatusInProgress:
    // 继续轮询
case dto.TaskStatusSucceeded:
    log.Println(status.Output.VideoURL)
case dto.TaskStatusFailed:
    log.Fatal(status.Output.Message)
}
```

`NormalizeTaskStatus` 会将 `pending/submitted` 映射为 `queued`，将 `running/processing/in_progress` 映射为 `in_progress`，将 `success/completed` 映射为 `succeeded`，并将 `failure/error/rejected/cancelled/canceled` 映射为 `failed`。也可直接使用 `dto.IsPending`、`dto.IsSucceeded`、`dto.IsFailed`。未知状态会保留原始值，并返回可通过 `errors.Is(err, dto.ErrUnsupportedTaskStatus)` 判断的兼容性错误。

`TaskStatus` 默认查询 `SetEndpoint` 后追加 `/{task_id}` 的地址。查询路径不符合该规则时，传入包含 `{task_id}` 的完整 URL 模板：

```go
status, err := client.TaskStatus(context.Background(), resp.TaskID, map[string]string{
    "endpoint": "https://api.example.com/tasks/{task_id}/result",
})
```

查询响应中的视频地址不在顶层 `video_url` 时，可以通过 `video_url_path` 指定点分隔字段路径。`video_url_path` 只用于本地解析，不会发送给第三方 API：

```go
status, err := client.TaskStatus(context.Background(), resp.TaskID, map[string]string{
    "video_url_path": "metadata.url",
})
```

顶层字段使用 `"video_url"`，嵌套字段使用 `"metadata.url"`。解析结果会同时写入 `status.Output.URL` 和 `status.Output.VideoURL`。

`MediaRequest.Extra` 会平铺到请求 JSON，并覆盖同名通用字段。额外鉴权或租户 Header 可通过 `SetExtraHeaders` 设置；其中的 `Authorization` 会覆盖默认的 `Bearer <APIKey>`。完整示例见 `examples/custom/generate_video.go`。

#### OpenAI 兼容异步图片

第三方图片接口使用 OpenAI 请求字段但只走异步任务时，仍使用 `custom` 并把 `SetEndpoint` 设置为完整创建 URL：

```go
client, err := omnigo.NewLLM(
    omnigo.SetProvider("custom"),
    omnigo.SetModel("nano-banana-pro-1k"),
    omnigo.SetEndpoint("https://ai.xxxx.cn/v1/images/generations"),
    omnigo.SetAPIKey(os.Getenv("API_KEY")),
)

created, err := client.Media(context.Background(), &dto.MediaRequest{
    Type:       dto.MediaTypeImage,
    Prompt:     "电影感城市夜景",
    Resolution: "1K",
    Extra: map[string]interface{}{
        "aspect_ratio": "16:9",
        "images": []string{"https://example.com/reference.png"},
    },
})
status, err := client.TaskStatus(context.Background(), created.TaskID)
```

`custom` 图片请求固定发送 `async=true`，不实现同步图片返回。`Resolution` 会映射为 `output_resolution`；公共字段中的 `seed` 和 `response_format` 不会发送，且 `n` 仅支持 1。查询响应中的 `data[0].url` 会映射到 `TaskStatusResponse.Output.URL`。

当完整创建 URL 以 `/images/edits` 结尾时，`custom` 自动发送 multipart 请求。通过 `Extra.image`/`Extra.images` 传入最多 9 张图片，通过 `Extra.mask` 传入一个蒙版；支持公网 URL、data URI、Base64 和本地文件路径，单个文件最大 10MB。

官方 `openai` adapter 默认仍使用同步图片 generation/edit。传入 `Extra["async"]=true` 时，创建请求继续使用 OpenAI 标准 generation/edit 协议（`async` 不会发送给上游），但会从响应中提取 `id/task_id` 并启用 `TaskStatus` 查询。轮询地址通过 `SetPollingURL` 配置；地址包含 `{task_id}` 时会替换占位符，否则会在末尾追加任务 ID：

```go
client, err := omnigo.NewLLM(
    omnigo.SetProvider("openai"),
    omnigo.SetModel("gpt-image-1"),
    omnigo.SetEndpoint("https://api.example.com/v1"),
    omnigo.SetPollingURL("https://api.example.com/v1/images/generations/{task_id}"),
    omnigo.SetAPIKey(os.Getenv("API_KEY")),
)

created, err := client.Media(context.Background(), &dto.MediaRequest{
    Type:   dto.MediaTypeImage,
    Prompt: "电影感城市夜景",
    Extra:  map[string]interface{}{"async": true},
})
status, err := client.TaskStatus(context.Background(), created.TaskID)
```

不兼容 OpenAI 标准创建协议、需要平铺第三方字段或自定义完整创建 URL 时，继续使用 `custom`。完整示例见 `examples/custom/image/generate_image.go`。

## 安装

```bash
go get github.com/YspCoder/omnigo
```

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/YspCoder/omnigo"
)

func main() {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        log.Fatalf("OPENAI_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("openai"),
        omnigo.SetModel("gpt-4o-mini"),
        omnigo.SetAPIKey(apiKey),
        omnigo.SetMaxTokens(200),
        omnigo.SetTimeout(30*time.Second),
        omnigo.SetLogLevel(omnigo.LogLevelInfo),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    ctx := context.Background()
    prompt := omnigo.NewPrompt("用一句话解释递归")

    resp, err := llm.Generate(ctx, prompt)
    if err != nil {
        log.Fatalf("generate failed: %v", err)
    }

    fmt.Println(resp)
}
```

## 快速参考

### 创建 LLM 与配置

```go
llm, err := omnigo.NewLLM(
    omnigo.SetProvider("openai"),
    omnigo.SetModel("gpt-4o-mini"),
    omnigo.SetAPIKey("your-api-key"),
    omnigo.SetSystemPrompt("你是一个严谨的中文助手"),
    omnigo.SetTemperature(0.7),
    omnigo.SetMaxTokens(300),
)
```

也可以通过环境变量配置默认 system prompt：

```bash
export LLM_SYSTEM_PROMPT="你是一个严谨的中文助手"
export LLM_SYSTEM_PROMPT_CACHE_TYPE="ephemeral"
```

### Prompt 结构化

```go
prompt := omnigo.NewPrompt(
    "解释递归，并给一个简短示例",
    omnigo.WithContext("面向初学者"),
    omnigo.WithDirectives("简洁", "给出示例"),
    omnigo.WithOutput("分为定义、示例、注意事项"),
    omnigo.WithMaxLength(500),
)
```

### JSON Schema 校验

```go
type Result struct {
    Topic      string   `json:"topic"`
    Pros       []string `json:"pros"`
    Cons       []string `json:"cons"`
    Conclusion string   `json:"conclusion"`
}

prompt := omnigo.NewPrompt(
    "分析远程办公的优缺点",
    omnigo.WithOutput("用 JSON 输出 topic/pros/cons/conclusion"),
)

resp, err := llm.Generate(ctx, prompt, omnigo.WithJSONSchemaValidation())
if err != nil {
    log.Fatalf("generate failed: %v", err)
}

resp = omnigo.CleanResponse(resp)
```

### 流式输出

```go
prompt := omnigo.NewPrompt("写一段简短的产品介绍")
stream, err := llm.Stream(ctx, prompt)
if err != nil {
    log.Fatalf("stream failed: %v", err)
}
defer stream.Close()

for {
    token, err := stream.Next(ctx)
    if err != nil {
        break
    }
    fmt.Print(token.Text)
}
```

补充说明：

1. 流式响应统一按 OpenAI 的事件格式解析（适用于兼容 OpenAI stream 的服务）。
2. `omnigo` 会在流式请求体中自动加入：
   - `"stream": true`
   - `"stream_options": { "include_usage": true }`
3. 某些服务商需要额外的流式请求头（如 Ali 的 `X-DashScope-SSE: enable`），这些由 adaptor 自动注入。

### 流式对话示例（OpenAI）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
)

func main() {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        log.Fatal("OPENAI_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("openai"),
        omnigo.SetModel("gpt-4o-mini"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    ctx := context.Background()
    prompt := omnigo.NewPrompt("用三句话解释递归")

    stream, err := llm.Stream(ctx, prompt)
    if err != nil {
        log.Fatalf("stream failed: %v", err)
    }
    defer stream.Close()

    for {
        token, err := stream.Next(ctx)
        if err != nil {
            break
        }
        fmt.Print(token.Text)
    }
}
```

### 流式对话示例（Ali / DashScope）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
)

func main() {
    apiKey := os.Getenv("DASHSCOPE_API_KEY")
    if apiKey == "" {
        log.Fatal("DASHSCOPE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("ali"),
        omnigo.SetModel("qwen-plus"),
        omnigo.SetAPIKey(apiKey),
        omnigo.SetEndpoint("https://dashscope.aliyuncs.com/compatible-mode/v1"),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    ctx := context.Background()
    prompt := omnigo.NewPrompt("用三句话解释递归")

    stream, err := llm.Stream(ctx, prompt)
    if err != nil {
        log.Fatalf("stream failed: %v", err)
    }
    defer stream.Close()

    for {
        token, err := stream.Next(ctx)
        if err != nil {
            break
        }
        fmt.Print(token.Text)
    }
}
```

## 高级用法

### 环境变量配置

可通过环境变量配置默认值（部分示例）：

- `LLM_PROVIDER`
- `LLM_MODEL`
- `LLM_ENDPOINT`
- `LLM_TEMPERATURE`
- `LLM_MAX_TOKENS`
- `LLM_TIMEOUT`
- `LLM_MAX_RETRIES`
- `LLM_RETRY_DELAY`
- `LLM_LOG_LEVEL`
- `LLM_ENABLE_CACHING`
- `LLM_ENABLE_STREAMING`

API Key 会自动从 `*_API_KEY` 形式的环境变量中加载（如 `OPENAI_API_KEY`）。

### 图像生成（示例）

> 注意：Ali 的图片生成走 `multimodal-generation` 接口（结构体为 `AliMultimodalGenerationRequest`）。以下示例以 `openai` 与 `ali` 各给一个。

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        log.Fatal("OPENAI_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("openai"),
        omnigo.SetModel("your-image-model"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

resp, err := llm.Media(context.Background(), &dto.MediaRequest{
    Type:           dto.MediaTypeImage,
    Model:          "your-image-model",
    Messages:       []dto.Message{{Role: "user", Content: "一只戴着墨镜的猫在沙滩上"}},
    N:              1,
    Size:           "1024x1024",
    ResponseFormat: "url",
})
    if err != nil {
        log.Fatalf("image failed: %v", err)
    }

    if len(resp.Data) > 0 {
        log.Println("image url:", resp.Data[0].URL)
    }
}
```

#### 图像生成示例（Ali / DashScope）

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("DASHSCOPE_API_KEY")
    if apiKey == "" {
        log.Fatal("DASHSCOPE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("ali"),
        omnigo.SetModel("qwen-image-max"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    req := &dto.MediaRequest{
        Type:     dto.MediaTypeImage,
        Model:    "qwen-image-max",
        Messages: []dto.Message{{Role: "user", Content: "一只戴着墨镜的猫在沙滩上"}},
        N:        1,
        Size:     "1024x1024",
        Extra: map[string]interface{}{
            "negative_prompt": "低质量, 模糊",
            "prompt_extend":   true,
            "watermark":       false,
        },
    }

    resp, err := llm.Media(context.Background(), req)
    if err != nil {
        log.Fatalf("image failed: %v", err)
    }

    log.Println("image url:", resp.URL)
}
```

### 视频生成示例（Ali / DashScope）

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("DASHSCOPE_API_KEY")
    if apiKey == "" {
        log.Fatal("DASHSCOPE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("ali"),
        omnigo.SetModel("wan2.2-kf2v-flash"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    req := &dto.MediaRequest{
        Type:     dto.MediaTypeVideo,
        Model:    "wan2.2-kf2v-flash",
        Messages: []dto.Message{{Role: "user", Content: "写实风格，一只黑色小猫好奇地看向天空"}},
        Extra: map[string]interface{}{
            "first_frame_url": "https://wanx.alicdn.com/material/20250318/first_frame.png",
            "last_frame_url":  "https://wanx.alicdn.com/material/20250318/last_frame.png",
            "resolution":      "480P",
            "prompt_extend":   true,
        },
    }

    resp, err := llm.Media(context.Background(), req)
    if err != nil {
        log.Fatalf("video failed: %v", err)
    }

    log.Println("task_id:", resp.TaskID)
    log.Println("status:", resp.Status)
}
```

### 任务状态查询示例（Ali / DashScope）

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
)

func main() {
    apiKey := os.Getenv("DASHSCOPE_API_KEY")
    if apiKey == "" {
        log.Fatal("DASHSCOPE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("ali"),
        omnigo.SetModel("wan2.2-kf2v-flash"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    resp, err := llm.TaskStatus(context.Background(), "your-task-id")
    if err != nil {
        log.Fatalf("task status failed: %v", err)
    }

    log.Println("status:", resp.Output.TaskStatus)
    log.Println("video_url:", resp.Output.VideoURL)
}
```

### 视频生成后轮询任务状态（Ali / DashScope）

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("DASHSCOPE_API_KEY")
    if apiKey == "" {
        log.Fatal("DASHSCOPE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("ali"),
        omnigo.SetModel("wan2.2-kf2v-flash"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    req := &dto.MediaRequest{
        Type:     dto.MediaTypeVideo,
        Model:    "wan2.2-kf2v-flash",
        Messages: []dto.Message{{Role: "user", Content: "写实风格，一只黑色小猫好奇地看向天空"}},
        Extra: map[string]interface{}{
            "first_frame_url": "https://wanx.alicdn.com/material/20250318/first_frame.png",
            "last_frame_url":  "https://wanx.alicdn.com/material/20250318/last_frame.png",
            "resolution":      "480P",
            "prompt_extend":   true,
        },
    }

    resp, err := llm.Media(context.Background(), req)
    if err != nil {
        log.Fatalf("video failed: %v", err)
    }
    if resp.TaskID == "" {
        log.Fatalf("empty task id")
    }

    for {
        status, err := llm.TaskStatus(context.Background(), resp.TaskID)
        if err != nil {
            log.Fatalf("task status failed: %v", err)
        }

        log.Println("status:", status.Output.TaskStatus)
        if status.Output.TaskStatus == "SUCCEEDED" || status.Output.TaskStatus == "FAILED" || status.Output.TaskStatus == "CANCELED" {
            log.Println("video_url:", status.Output.VideoURL)
            break
        }

        time.Sleep(5 * time.Second)
    }
}
```

### 视频生成示例 (Jimeng / 即梦)

即梦 (Jimeng) 适配器支持动态模型映射。您可以通过 `SetModel` 指定模型编号（如 `jimeng_ti2v_v30_pro`），或在 `Extra` 中通过 `req_key` 覆盖。

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("JIMENG_API_KEY") // 火山引擎 API Key
    if apiKey == "" {
        log.Fatal("JIMENG_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("jimeng"),
        omnigo.SetModel("jimeng_ti2v_v30_pro"), // 指定即梦模型编号
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    req := &dto.MediaRequest{
        Type:     dto.MediaTypeVideo,
        Messages: []dto.Message{{Role: "user", Content: "赛博朋克风格的白兔执行官在指挥中心，全息屏闪烁"}},
        Extra: map[string]interface{}{
            "image_url": "https://example.com/character.png", // 可选的首帧图
        },
    }

    resp, err := llm.Media(context.Background(), req)
    if err != nil {
        log.Fatalf("video generation failed: %v", err)
    }

    log.Printf("Task Submitted. ID: %s, Status: %s", resp.TaskID, resp.Status)
}
```

### 视频生成示例 (Vidu)

Vidu 适配器目前覆盖以下视频能力：

- `text-to-video`
- `image-to-video`
- `reference-to-video`
- `start-end-to-video`
- `multi-frame`
- `reference-to-image`
- `text-to-audio`
- `timing-to-audio`
- `text-to-speech`
- `voice-clone`
- `lip-sync`

支持两种路由方式：

- 推荐：通过 `Extra["mode"]` 显式指定能力
- 兼容：只传 `Type` 时，omnigo 会按 `Type + Model + 输入形态` 自动适配  
  例如 `Type=video` 会按图片数 / `subjects` / `frames` 自动区分图生、参考生、首尾帧、多帧；`Type=text` 在 Vidu 下会按模型名称优先路由到音频/TTS能力，而不是聊天接口

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("VIDU_API_KEY")
    if apiKey == "" {
        log.Fatal("VIDU_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("vidu"),
        omnigo.SetModel("viduq2"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

    req := &dto.MediaRequest{
        Type:       dto.MediaTypeVideo,
        Model:      "viduq2",
        Messages:   []dto.Message{{Role: "user", Content: "一个机器人站在雨夜霓虹街头，镜头缓慢推进"}},
        Size:       "16:9",
        Duration:   5,
        Resolution: "720p",
        Extra: map[string]interface{}{
            "mode":               "text-to-video",
            "movement_amplitude": "medium",
            "bgm":                true,
        },
    }

    resp, err := llm.Media(context.Background(), req)
    if err != nil {
        log.Fatalf("video generation failed: %v", err)
    }
    log.Printf("Task submitted. ID=%s Status=%s", resp.TaskID, resp.Status)

    for {
        status, err := llm.TaskStatus(context.Background(), resp.TaskID)
        if err != nil {
            log.Fatalf("query task failed: %v", err)
        }
        log.Println("status:", status.Output.TaskStatus)
        if status.Output.TaskStatus == "success" {
            log.Println("video_url:", status.Output.VideoURL)
            break
        }
        if status.Output.TaskStatus == "failed" {
            log.Fatalf("video failed: %s", status.Output.Message)
        }
        time.Sleep(5 * time.Second)
    }
}
```

Vidu 的 `Extra` 入参约定：

- `mode`: `text-to-video` / `image-to-video` / `reference-to-video` / `start-end-to-video` / `multi-frame`
- `mode`: `reference-to-image` / `text-to-audio` / `timing-to-audio` / `text-to-speech` / `voice-clone` / `lip-sync`
- `image` 或 `images`: 图生/首尾帧输入图片 URL
- `start_image`, `end_image`: 首尾帧模式
- `subjects`: 参考生视频主体数组，原样透传到 Vidu
- `frames`: 多帧模式帧数组，原样透传到 Vidu
- `style`, `movement_amplitude`, `bgm`, `callback_url`, `off_peak`, `watermark`, `wm_position`, `wm_url`, `payload`, `meta_data`: Vidu 扩展参数
- `lip-sync` 建议直接把官方接口字段放进 `Extra`，例如 `video_url`、`audio_url`、`text`、`voice` 等，omnigo 会原样透传到 Vidu

图片与音频调用示例：

```go
// 参考生图
imgReq := &dto.MediaRequest{
    Type:       dto.MediaTypeImage,
    Model:      "vidu2.0",
    Messages:   []dto.Message{{Role: "user", Content: "生成一张角色一致的海报图"}},
    Size:       "1:1",
    Resolution: "2k",
    Extra: map[string]interface{}{
        "mode": "reference-to-image",
        "subjects": []map[string]interface{}{
            {"images": []string{"https://example.com/character.png"}},
        },
    },
}

// 文生音频
audioReq := &dto.MediaRequest{
    Type:     dto.MediaTypeAudio,
    Model:    "vidu2.0",
    Messages: []dto.Message{{Role: "user", Content: "雨夜城市环境音，远处有列车经过"}},
    Extra: map[string]interface{}{
        "mode": "text-to-audio",
    },
}

// 语音合成
ttsReq := &dto.MediaRequest{
    Type:  dto.MediaTypeAudio,
    Model: "vidu2.0",
    Extra: map[string]interface{}{
        "mode":  "text-to-speech",
        "text":  "欢迎使用 omnigo 接入 Vidu。",
        "voice": "your_voice_id",
    },
}
```

任务接口：

```go
status, err := llm.TaskStatus(ctx, "your-task-id")
if err != nil {
    log.Fatal(err)
}
log.Println(status.Output.TaskStatus, status.Output.URL)

tasks, err := llm.ListTasks(ctx, map[string]string{
    "page_num":  "1",
    "page_size": "20",
})
if err != nil {
    log.Fatal(err)
}
log.Println("total:", tasks.Total, "items:", len(tasks.Items))
```

其中 `subjects` / `frames` 建议直接按 Vidu 官方接口结构传入，omnigo 不额外重写其内部字段。

### 文本生成示例 (Google / Gemini)

Google 适配器支持 Gemini 全系列模型。它采用了与 `google.golang.org/genai` (BackendGeminiAPI) 兼容的 REST 协议。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
)

func main() {
    apiKey := os.Getenv("GOOGLE_API_KEY")
    if apiKey == "" {
        log.Fatal("GOOGLE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("google"),
        omnigo.SetModel("gemini-2.0-flash-exp"), // 指定模型名称
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("failed to create llm: %v", err)
    }

    ctx := context.Background()
    prompt := omnigo.NewPrompt("你好，请介绍一下你自己")

    resp, err := llm.Generate(ctx, prompt)
    if err != nil {
        log.Fatalf("generate failed: %v", err)
    }

    fmt.Println("Response:", resp)
}
```

### 图像生成示例 (Google / Gemini)

Google 适配器对不同模型使用不同接口：

- `imagen-*` 图像模型走 `GenerateImages` / `predict`
- `gemini-*` 原生图像生成模型走 `GenerateContent`，并通过 `responseModalities=["TEXT","IMAGE"]` 返回图像
- 视频模型继续走 `GenerateVideos`

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("GOOGLE_API_KEY")
    if apiKey == "" {
        log.Fatal("GOOGLE_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("google"),
        omnigo.SetModel("imagen-3.0-generate-001"), // 指定视觉模型
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("failed to create llm: %v", err)
    }

    req := &dto.MediaRequest{
        Type:     dto.MediaTypeImage,
        Messages: []dto.Message{{Role: "user", Content: "A sophisticated white rabbit in a sharp navy suit, cinematic lighting"}},
        Size:     "1:1", // 纵横比
    }

    resp, err := llm.Media(context.Background(), req)
    if err != nil {
        log.Fatalf("image failed: %v", err)
    }

    if resp.URL != "" {
        fmt.Println("Image URL/Data:", resp.URL)
    } else if resp.TaskID != "" {
        fmt.Println("Async Task ID:", resp.TaskID)
    }
}
```

### 视频生成（示例）

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/YspCoder/omnigo"
    "github.com/YspCoder/omnigo/dto"
)

func main() {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        log.Fatal("OPENAI_API_KEY is not set")
    }

    llm, err := omnigo.NewLLM(
        omnigo.SetProvider("openai"),
        omnigo.SetModel("your-video-model"),
        omnigo.SetAPIKey(apiKey),
    )
    if err != nil {
        log.Fatalf("create LLM failed: %v", err)
    }

resp, err := llm.Media(context.Background(), &dto.MediaRequest{
    Type:           dto.MediaTypeVideo,
    Model:          "your-video-model",
    Messages:       []dto.Message{{Role: "user", Content: "日落时分的城市航拍，暖色调"}},
    Size:           "1024x1024",
    Duration:       5,
    Fps:            24,
    ResponseFormat: "url",
})
    if err != nil {
        log.Fatalf("video failed: %v", err)
    }

    log.Println("video status:", resp.Status)
    log.Println("video url:", resp.Video.URL)
}
```

## 最佳实践

1. **清晰结构化提示词**：结合 `WithContext` / `WithDirectives` / `WithOutput` 让输出稳定。
2. **显式限制输出长度**：使用 `WithMaxLength` 或 `SetMaxTokens`。
3. **合理的重试与日志级别**：生产环境建议设置 `SetMaxRetries` 与 `SetLogLevel`。
4. **结构化输出时启用校验**：结合 `WithJSONSchemaValidation` 与 `CleanResponse`。

## 项目状态

该项目仍在快速迭代中，API 可能会有调整。欢迎反馈与 PR。

## 贡献

- 提交 Issue 或 PR 之前请先简单描述问题/需求。
- 新增 Provider 请参考 `adapter/registry.go` 的结构。

## 许可证

MIT License
