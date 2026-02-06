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
- **可扩展的 Provider Registry**：默认提供 OpenAI 与 Ali（DashScope），支持自定义扩展。
- **流式与非流式统一处理**：一套接口支持 streaming 与非 streaming。
- **结构化输出与校验**：支持 JSON Schema 校验与提示词结构化。
- **可配置性强**：支持环境变量加载 + 代码选项式配置。
- **日志与重试**：内置日志级别与重试策略。

## 支持的服务商

当前内置 Provider Spec（可扩展）：

- OpenAI (`openai`)
- Ali / DashScope (`ali`)
- Jimeng / Volcengine (`jimeng`)
- Google / Gemini (`google`)

> 说明：以上名称为 `SetProvider(...)` 传入值。

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
    omnigo.SetTemperature(0.7),
    omnigo.SetMaxTokens(300),
)
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
    Prompt:         "一只戴着墨镜的猫在沙滩上",
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
        Type:   dto.MediaTypeImage,
        Model:  "qwen-image-max",
        Prompt: "一只戴着墨镜的猫在沙滩上",
        N:      1,
        Size:   "1024x1024",
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
        Type:   dto.MediaTypeVideo,
        Model:  "wan2.2-kf2v-flash",
        Prompt: "写实风格，一只黑色小猫好奇地看向天空",
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
        Type:   dto.MediaTypeVideo,
        Model:  "wan2.2-kf2v-flash",
        Prompt: "写实风格，一只黑色小猫好奇地看向天空",
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
        Type:   dto.MediaTypeVideo,
        Prompt: "赛博朋克风格的白兔执行官在指挥中心，全息屏闪烁",
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

Google 适配器支持通过 `predict` 接口进行图像与视频生成。

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
        Type:   dto.MediaTypeImage,
        Prompt: "A sophisticated white rabbit in a sharp navy suit, cinematic lighting",
        Size:   "1:1", // 纵横比
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
    Prompt:         "日落时分的城市航拍，暖色调",
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
