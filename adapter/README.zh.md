# Adaptor 职责说明

Adaptor 是 omnigo 的“协议适配层”，负责将统一的 DTO（`dto.ChatRequest`/`ImageRequest`/`VideoRequest`）转换为各家模型服务的具体 API 格式，并将响应解析回统一 DTO。它不负责发起网络请求，也不负责重试、日志或缓存，这些由 `relay` 和 `llm` 层完成。

## 核心职责

1) 端点解析
- 根据 `ProviderConfig.BaseURL` 与当前模式（chat/image/video），拼出正确的请求 URL。
- 不决定 BaseURL 的来源，只消费 `ProviderConfig` 中已有的值。

2) 认证与请求头
- 根据 `ProviderConfig.AuthHeader/AuthPrefix/APIKey` 设置认证头。
- 设置必要的内容类型或厂商要求的自定义头。

3) 请求体转换
- 将统一 DTO 转为厂商所需的 JSON 结构。
- 处理特定参数、工具调用、system prompt、结构化输出等差异。

4) 响应体解析
- 将厂商响应解析为统一 DTO。
- 必要时抽取 usage、错误码等信息。

5) 流式处理（可选）
- 实现 `StreamAdaptor` 时，负责构造流式请求体并解析单个流式 chunk。

## 不做的事

- 不发 HTTP 请求（由 `relay` 执行）。
- 不处理重试、缓存、日志（由 `llm`/`utils` 负责）。
- 不决定 provider 的默认参数与路由（由 `registry` 管理）。

## 相关结构

- `adapter/interface.go`
  - `Adaptor`：非流式转换接口
  - `StreamAdaptor`：流式扩展接口
- `adapter/registry.go`
  - `ProviderSpec`：默认端点、认证头、能力声明
  - `Registry`：provider 注册与 adaptor 构建
- `relay/relay.go`
  - 执行 HTTP 请求并返回响应体

## 添加新 Adaptor 的步骤

1) 新建 adaptor 文件，例如 `adapter/xxx.go`，实现 `Adaptor`（可选实现 `StreamAdaptor`）。
2) 在 `adapter/registry.go` 注册 provider：
   - 设置 `ProviderSpec{Endpoint, AuthHeader, SupportsStreaming...}`
   - 如果是自定义协议，提供 `AdaptorFactory`。
3) 在上层使用 `config.SetProvider` 与 `config.SetEndpoint`（若需要）创建实例。

## 设计原则

- 只关心协议适配，尽量保持“纯转换”。
- 入参/出参使用统一 DTO，减少上层分支逻辑。
- 对缺失/不支持能力要明确报错，避免静默失败。
