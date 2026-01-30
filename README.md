# omnigo - Go 语言 LLM 集成工具包

`omnigo` 是一个面向 Go 的 LLM 集成工具包，核心是统一的 adaptor/relay 架构：
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

> 注意：并非所有 Provider 都支持图像/视频。以下示例以 `openai` 为例。

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
