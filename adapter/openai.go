// Package adapter provides OpenAI adaptor implementation using the official SDK.
package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

type OpenAIAdaptor struct {
	client *openai.Client
}

func (a *OpenAIAdaptor) getClient(config *ProviderConfig) *openai.Client {
	if a.client != nil {
		return a.client
	}

	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
	}
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}
	if config.Organization != "" {
		opts = append(opts, option.WithOrganization(config.Organization))
	}
	for key, value := range config.Headers {
		opts = append(opts, option.WithHeader(key, value))
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	if config.Proxy != "" {
		proxyURL, err := url.Parse(config.Proxy)
		if err == nil {
			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
			httpClient = &http.Client{
				Transport: transport,
			}
		}
	}

	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}

	client := openai.NewClient(opts...)
	a.client = &client
	return a.client
}

func (a *OpenAIAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	useResponses, err := openAIUsesResponsesAPI(config, request.Messages)
	if err != nil {
		return nil, err
	}
	if useResponses {
		return a.chatWithResponsesAPI(ctx, config, request)
	}

	client := a.getClient(config)

	params := openai.ChatCompletionNewParams{
		Model:    request.Model,
		Messages: toOpenAIMessages(request.Messages),
	}
	if err := applyOpenAIChatTools(&params, request); err != nil {
		return nil, err
	}
	if request.Temperature != 0 {
		params.Temperature = openai.Float(request.Temperature)
	}
	if request.MaxTokens != 0 {
		params.MaxTokens = openai.Int(int64(request.MaxTokens))
	}

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	res := &dto.MediaResponse{
		ID:      resp.ID,
		Created: resp.Created,
		Model:   resp.Model,
		Choices: make([]dto.ChatChoice, len(resp.Choices)),
		Usage: dto.Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}
	for i, c := range resp.Choices {
		message := dto.Message{
			Role:    string(c.Message.Role),
			Content: c.Message.Content,
		}
		message.ToolCalls = fromOpenAIToolCalls(c.Message.ToolCalls)
		res.Choices[i] = dto.ChatChoice{
			Index:        i,
			Message:      message,
			FinishReason: string(c.FinishReason),
		}
	}
	if len(res.Choices) > 0 {
		res.Text = fmt.Sprint(res.Choices[0].Message.Content)
	}
	return res, nil
}

type openAIResponsesRequest struct {
	Model           string                `json:"model"`
	Input           []json.RawMessage     `json:"input"`
	Tools           []openAIResponsesTool `json:"tools,omitempty"`
	ToolChoice      interface{}           `json:"tool_choice,omitempty"`
	Temperature     *float64              `json:"temperature,omitempty"`
	MaxOutputTokens *int                  `json:"max_output_tokens,omitempty"`
}

type openAIResponsesTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIResponsesInputItem struct {
	Role    string                            `json:"role"`
	Content []openAIResponsesInputContentItem `json:"content"`
}

type openAIResponsesInputContentItem struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	FileURL     string `json:"file_url,omitempty"`
	FileID      string `json:"file_id,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	ImageDetail string `json:"detail,omitempty"`
}

type openAIResponsesResponse struct {
	ID         string                      `json:"id"`
	Model      string                      `json:"model"`
	OutputText string                      `json:"output_text"`
	Output     []openAIResponsesOutputItem `json:"output"`
	Usage      openAIResponsesUsage        `json:"usage"`
	Error      *openAIResponsesError       `json:"error,omitempty"`
}

type openAIResponsesOutputItem struct {
	Type      string                         `json:"type"`
	Content   []openAIResponsesOutputContent `json:"content"`
	Role      string                         `json:"role"`
	CallID    string                         `json:"call_id"`
	Name      string                         `json:"name"`
	Arguments string                         `json:"arguments"`
}

type openAIResponsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type openAIResponsesError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (a *OpenAIAdaptor) chatWithResponsesAPI(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	input, err := toOpenAIResponsesInput(request.Messages)
	if err != nil {
		return nil, err
	}
	payload := openAIResponsesRequest{
		Model:      request.Model,
		Input:      input,
		Tools:      toOpenAIResponsesTools(request.Tools),
		ToolChoice: openAIResponsesToolChoice(request.ToolChoice),
	}
	if request.Temperature != 0 {
		payload.Temperature = &request.Temperature
	}
	if request.MaxTokens != 0 {
		maxOutputTokens := request.MaxTokens
		payload.MaxOutputTokens = &maxOutputTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(openAIBaseURL(config), "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if config.Organization != "" {
		req.Header.Set("OpenAI-Organization", config.Organization)
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := a.getHTTPClient(config).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai responses api error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var parsed openAIResponsesResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("openai responses api error: %s", parsed.Error.Message)
	}
	text := strings.TrimSpace(parsed.OutputText)
	if text == "" {
		text = strings.TrimSpace(extractOpenAIResponsesText(parsed.Output))
	}

	message := dto.Message{
		Role: "assistant", Content: text,
		ToolCalls: fromOpenAIResponsesToolCalls(parsed.Output),
	}
	finishReason := "completed"
	if len(message.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return &dto.MediaResponse{
		ID:    parsed.ID,
		Model: parsed.Model,
		Text:  text,
		Choices: []dto.ChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: dto.Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

func openAIUsesResponsesAPI(config *ProviderConfig, messages []dto.Message) (bool, error) {
	protocol := ""
	if config != nil {
		protocol = strings.ToLower(strings.TrimSpace(config.ChatProtocol))
	}
	switch protocol {
	case "responses":
		return true, nil
	case "chat":
		return false, nil
	case "":
	default:
		return false, fmt.Errorf("unsupported openai chat protocol %q", protocol)
	}
	for _, message := range messages {
		if message.FileURL != "" || len(openAIMessageImageURLs(message)) > 0 {
			return true, nil
		}
	}
	return false, nil
}

func toOpenAIResponsesInput(messages []dto.Message) ([]json.RawMessage, error) {
	items := make([]json.RawMessage, 0, len(messages)*2)
	for _, message := range messages {
		if message.Role == "tool" {
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, fmt.Errorf("openai tool message requires tool_call_id")
			}
			item, err := json.Marshal(map[string]interface{}{
				"type": "function_call_output", "call_id": message.ToolCallID,
				"output": openAIMessageTextContent(message),
			})
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			continue
		}
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		content := make([]openAIResponsesInputContentItem, 0, 2)
		if text := openAIMessageTextContent(message); text != "" {
			content = append(content, openAIResponsesInputContentItem{
				Type: "input_text",
				Text: text,
			})
		}
		for _, imageURL := range openAIMessageImageURLs(message) {
			content = append(content, openAIResponsesInputContentItem{
				Type:        "input_image",
				ImageURL:    imageURL,
				ImageDetail: message.ImageDetail,
			})
		}
		if message.FileURL != "" {
			content = append(content, openAIResponsesInputContentItem{
				Type:     "input_file",
				FileURL:  message.FileURL,
				Filename: message.Name,
			})
		}
		if len(content) > 0 {
			item, err := json.Marshal(openAIResponsesInputItem{
				Role:    role,
				Content: content,
			})
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		for _, call := range message.ToolCalls {
			callItem, err := json.Marshal(map[string]interface{}{
				"type": "function_call", "call_id": call.ID,
				"name": call.Function.Name, "arguments": string(call.Function.Arguments),
			})
			if err != nil {
				return nil, err
			}
			items = append(items, callItem)
		}
	}
	return items, nil
}

func extractOpenAIResponsesText(items []openAIResponsesOutputItem) string {
	parts := make([]string, 0, 4)
	for _, item := range items {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) == "" {
				continue
			}
			parts = append(parts, strings.TrimSpace(content.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func fromOpenAIResponsesToolCalls(items []openAIResponsesOutputItem) []dto.ToolCall {
	result := make([]dto.ToolCall, 0, len(items))
	for _, item := range items {
		if item.Type != "function_call" {
			continue
		}
		result = append(result, dto.ToolCall{
			ID: item.CallID, Type: "function",
			Function: dto.ToolCallFunction{
				Name: item.Name, Arguments: json.RawMessage(item.Arguments),
			},
		})
	}
	return result
}

func toOpenAIResponsesTools(tools []dto.Tool) []openAIResponsesTool {
	result := make([]openAIResponsesTool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, openAIResponsesTool{
			Type: firstNonEmptyString(tool.Type, "function"),
			Name: tool.Function.Name, Description: tool.Function.Description,
			Parameters: tool.Function.Parameters,
		})
	}
	return result
}

func openAIResponsesToolChoice(value interface{}) interface{} {
	if mode, ok := openAIToolChoiceMode(value); ok {
		return mode
	}
	return value
}

func toOpenAIMessages(messages []dto.Message) []openai.ChatCompletionMessageParamUnion {
	res := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, m := range messages {
		content := openAIMessageTextContent(m)
		parts := openAIChatMessageParts(m)
		switch m.Role {
		case "system":
			res[i] = openai.SystemMessage(content)
		case "user":
			if len(parts) > 0 {
				res[i] = openai.UserMessage(parts)
			} else {
				res[i] = openai.UserMessage(content)
			}
		case "assistant":
			message := openai.AssistantMessage(content)
			message.OfAssistant.ToolCalls = toOpenAIToolCallParams(m.ToolCalls)
			res[i] = message
		case "tool":
			res[i] = openai.ToolMessage(content, m.ToolCallID)
		case "developer":
			res[i] = openai.DeveloperMessage(content)
		default:
			if len(parts) > 0 {
				res[i] = openai.UserMessage(parts)
			} else {
				res[i] = openai.UserMessage(content)
			}
		}
	}
	return res
}

func toOpenAIToolCallParams(calls []dto.ToolCall) []openai.ChatCompletionMessageToolCallUnionParam {
	result := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, call := range calls {
		result = append(result, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: call.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name: call.Function.Name, Arguments: string(call.Function.Arguments),
				},
			},
		})
	}
	return result
}

func fromOpenAIToolCalls(calls []openai.ChatCompletionMessageToolCallUnion) []dto.ToolCall {
	result := make([]dto.ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.Type != "function" && call.Function.Name == "" {
			continue
		}
		result = append(result, dto.ToolCall{
			ID: call.ID, Type: "function",
			Function: dto.ToolCallFunction{
				Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments),
			},
		})
	}
	return result
}

func applyOpenAIChatTools(params *openai.ChatCompletionNewParams, request *dto.MediaRequest) error {
	for _, tool := range request.Tools {
		toolType := strings.ToLower(strings.TrimSpace(tool.Type))
		if toolType != "" && toolType != "function" {
			return fmt.Errorf("unsupported openai tool type %q", tool.Type)
		}
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name: tool.Function.Name, Description: openai.String(tool.Function.Description),
			Parameters: tool.Function.Parameters,
		}))
	}
	if request.ToolChoice == nil {
		return nil
	}
	if mode, ok := openAIToolChoiceMode(request.ToolChoice); ok {
		params.ToolChoice.OfAuto = openai.String(mode)
		return nil
	}
	choice, ok := request.ToolChoice.(map[string]interface{})
	if !ok || strings.ToLower(strings.TrimSpace(fmt.Sprint(choice["type"]))) != "function" {
		return fmt.Errorf("unsupported openai tool choice")
	}
	function, ok := choice["function"].(map[string]interface{})
	if !ok || strings.TrimSpace(fmt.Sprint(function["name"])) == "" {
		return fmt.Errorf("openai function tool choice requires a name")
	}
	params.ToolChoice = openai.ToolChoiceOptionFunctionToolChoice(
		openai.ChatCompletionNamedToolChoiceFunctionParam{Name: strings.TrimSpace(fmt.Sprint(function["name"]))},
	)
	return nil
}

func openAIToolChoiceMode(value interface{}) (string, bool) {
	mode := ""
	switch typed := value.(type) {
	case string:
		mode = typed
	case map[string]interface{}:
		mode = fmt.Sprint(typed["type"])
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "none", "auto", "required":
		return mode, true
	default:
		return "", false
	}
}

func openAIChatMessageParts(message dto.Message) []openai.ChatCompletionContentPartUnionParam {
	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, 1+len(openAIMessageImageURLs(message)))
	if text := openAIMessageTextContent(message); text != "" {
		parts = append(parts, openai.TextContentPart(text))
	}
	for _, imageURL := range openAIMessageImageURLs(message) {
		parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
			URL:    imageURL,
			Detail: message.ImageDetail,
		}))
	}
	return parts
}

func openAIMessageImageURLs(message dto.Message) []string {
	urls := make([]string, 0, 2)
	if imageURL := strings.TrimSpace(message.ImageURL); imageURL != "" {
		urls = append(urls, imageURL)
	}
	if extraImages := openAIContentImageURLs(message.Content); len(extraImages) > 0 {
		urls = append(urls, extraImages...)
	}
	return urls
}

func openAIMessageTextContent(message dto.Message) string {
	switch message.Content.(type) {
	case nil, []interface{}, []string, map[string]interface{}, map[string]string:
		return ""
	default:
		text := strings.TrimSpace(fmt.Sprint(message.Content))
		if text == "" || text == "<nil>" {
			return ""
		}
		return text
	}
}

func openAIContentImageURLs(content interface{}) []string {
	switch content.(type) {
	case []interface{}, []string, map[string]interface{}, map[string]string:
		return utils.ContentImageURLs(content)
	default:
		return nil
	}
}

func (a *OpenAIAdaptor) getHTTPClient(config *ProviderConfig) *http.Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if config.Proxy != "" {
		proxyURL, err := url.Parse(config.Proxy)
		if err == nil {
			httpClient = &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
				},
			}
		}
	}
	return httpClient
}

func openAIBaseURL(config *ProviderConfig) string {
	if strings.TrimSpace(config.BaseURL) != "" {
		return strings.TrimSpace(config.BaseURL)
	}
	return "https://api.openai.com/v1"
}

type openAIStreamWrapper struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk]
}

func (w *openAIStreamWrapper) Next(ctx context.Context) (*dto.StreamToken, error) {
	if !w.stream.Next() {
		if err := w.stream.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	resp := w.stream.Current()
	if len(resp.Choices) == 0 {
		return &dto.StreamToken{Text: ""}, nil
	}

	return &dto.StreamToken{
		Text:  resp.Choices[0].Delta.Content,
		Type:  "text",
		Index: int(resp.Choices[0].Index),
	}, nil
}

func (w *openAIStreamWrapper) Close() error {
	return w.stream.Close()
}

func (a *OpenAIAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	useResponses, err := openAIUsesResponsesAPI(config, request.Messages)
	if err != nil {
		return nil, err
	}
	if useResponses {
		return nil, fmt.Errorf("openai responses streaming is not supported")
	}
	client := a.getClient(config)

	params := openai.ChatCompletionNewParams{
		Model:    request.Model,
		Messages: toOpenAIMessages(request.Messages),
	}
	if err := applyOpenAIChatTools(&params, request); err != nil {
		return nil, err
	}
	if request.Temperature != 0 {
		params.Temperature = openai.Float(request.Temperature)
	}
	if request.MaxTokens != 0 {
		params.MaxTokens = openai.Int(int64(request.MaxTokens))
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	return &openAIStreamWrapper{stream: stream}, nil
}

func (a *OpenAIAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	client := a.getClient(config)
	async, _ := utils.GetBoolExtra(request.Extra, "async")

	switch request.Type {
	case dto.MediaTypeImage:
		inputs := utils.ParseExtraImageInputs(request.Extra)
		if len(inputs) > 0 {
			referenceImages, err := openAIImageReferenceReaders(ctx, a.getHTTPClient(config), inputs)
			if err != nil {
				return nil, err
			}

			params := openai.ImageEditParams{
				Prompt: utils.MediaPromptWithSystem(request),
				Model:  request.Model,
				Image:  openAIImageEditInput(referenceImages),
			}
			if request.N > 0 {
				params.N = openai.Int(int64(request.N))
			}
			if request.Size != "" {
				params.Size = openai.ImageEditParamsSize(request.Size)
			}
			if request.ResponseFormat != "" {
				params.ResponseFormat = openai.ImageEditParamsResponseFormat(request.ResponseFormat)
			}
			if request.Resolution != "" {
				params.Quality = openai.ImageEditParamsQuality(request.Resolution)
			}
			resp, err := client.Images.Edit(ctx, params)
			if err != nil {
				return nil, err
			}
			if async {
				return openAIAsyncImageResponse(resp)
			}
			return openAIImageResponse(resp), nil
		}

		params := openai.ImageGenerateParams{
			Prompt: utils.MediaPromptWithSystem(request),
			Model:  request.Model,
		}
		if request.N > 0 {
			params.N = openai.Int(int64(request.N))
		}
		if request.Size != "" {
			params.Size = openai.ImageGenerateParamsSize(request.Size)
		}
		if request.ResponseFormat != "" {
			params.ResponseFormat = openai.ImageGenerateParamsResponseFormat(request.ResponseFormat)
		}
		if request.Resolution != "" {
			params.Quality = openai.ImageGenerateParamsQuality(request.Resolution)
		}
		resp, err := client.Images.Generate(ctx, params)
		if err != nil {
			return nil, err
		}
		if async {
			return openAIAsyncImageResponse(resp)
		}
		return openAIImageResponse(resp), nil
	default:
		return nil, fmt.Errorf("unsupported media mode: %s", request.Type)
	}
}

type openAIAsyncTaskResponse struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	RequestID string          `json:"request_id"`
	Object    string          `json:"object"`
	Model     string          `json:"model"`
	Status    string          `json:"status"`
	State     string          `json:"state"`
	URL       string          `json:"url"`
	VideoURL  string          `json:"video_url"`
	Data      []dto.ImageData `json:"data"`
	Code      interface{}     `json:"code"`
	Message   string          `json:"message"`
	Error     *struct {
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	} `json:"error"`
}

func openAIAsyncImageResponse(resp *openai.ImagesResponse) (*dto.MediaResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("openai returned an empty async image response")
	}

	var task openAIAsyncTaskResponse
	if err := json.Unmarshal([]byte(resp.RawJSON()), &task); err != nil {
		return nil, fmt.Errorf("decode openai async image response: %w", err)
	}

	result := openAIImageResponse(resp)
	if len(result.Data) == 0 && len(task.Data) > 0 {
		result.Data = append([]dto.ImageData(nil), task.Data...)
		result.URL = openAIFirstImageURL(task.Data)
	}
	result.ID = task.ID
	result.Object = task.Object
	result.Model = task.Model
	result.RequestID = task.RequestID
	result.TaskID = firstNonEmptyString(task.TaskID, task.ID)
	result.Status = firstNonEmptyString(task.Status, task.State)
	result.URL = firstNonEmptyString(result.URL, task.URL, openAIFirstImageURL(task.Data), task.VideoURL)
	result.ErrorCode = openAIErrorCode(task)
	result.ErrorMessage = openAIErrorMessage(task)
	return result, nil
}

func openAIImageResponse(resp *openai.ImagesResponse) *dto.MediaResponse {
	res := &dto.MediaResponse{}
	for _, img := range resp.Data {
		res.Data = append(res.Data, dto.ImageData{
			URL:     img.URL,
			B64JSON: img.B64JSON,
		})
	}
	if len(res.Data) > 0 {
		res.URL = res.Data[0].URL
	}
	return res
}

func openAIImageEditInput(images []io.Reader) openai.ImageEditParamsImageUnion {
	if len(images) == 1 {
		return openai.ImageEditParamsImageUnion{OfFile: images[0]}
	}
	return openai.ImageEditParamsImageUnion{OfFileArray: images}
}

func openAIImageReferenceReaders(ctx context.Context, httpClient *http.Client, inputs []string) ([]io.Reader, error) {
	readers := make([]io.Reader, 0, len(inputs))
	for i, input := range inputs {
		reader, err := openAIImageReferenceReader(ctx, httpClient, input, i)
		if err != nil {
			return nil, err
		}
		readers = append(readers, reader)
	}
	return readers, nil
}

type openAIImageReader struct {
	*bytes.Reader
	filename    string
	contentType string
}

func (r openAIImageReader) Filename() string {
	return r.filename
}

func (r openAIImageReader) ContentType() string {
	return r.contentType
}

func openAIImageReferenceReader(ctx context.Context, httpClient *http.Client, input string, index int) (io.Reader, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("openai image reference %d is empty", index)
	}
	if strings.HasPrefix(input, "data:") {
		return openAIImageReaderFromDataURL(input, index)
	}
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return openAIImageReaderFromURL(ctx, httpClient, input)
	}
	if data, err := os.ReadFile(input); err == nil {
		return newOpenAIImageReader(data, filepath.Base(input), ""), nil
	}
	if data, err := base64.StdEncoding.DecodeString(input); err == nil {
		return newOpenAIImageReader(data, openAIImageFilename(index, http.DetectContentType(data)), ""), nil
	}
	return nil, fmt.Errorf("openai image reference %d must be a URL, data URL, base64 string, or readable file path", index)
}

func openAIImageReaderFromDataURL(input string, index int) (io.Reader, error) {
	header, encoded, ok := strings.Cut(input, ",")
	if !ok {
		return nil, fmt.Errorf("openai image reference %d has invalid data URL", index)
	}
	contentType := strings.TrimPrefix(header, "data:")
	if semi := strings.Index(contentType, ";"); semi >= 0 {
		contentType = contentType[:semi]
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("openai image reference %d has invalid base64 data: %w", index, err)
	}
	return newOpenAIImageReader(data, openAIImageFilename(index, contentType), contentType), nil
}

func openAIImageReaderFromURL(ctx context.Context, httpClient *http.Client, input string) (io.Reader, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai image reference fetch failed: status=%d url=%s", resp.StatusCode, input)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 50<<20 {
		return nil, fmt.Errorf("openai image reference exceeds 50MB: %s", input)
	}
	contentType := resp.Header.Get("Content-Type")
	parsed, _ := url.Parse(input)
	filename := path.Base(parsed.Path)
	if filename == "." || filename == "/" || filename == "" {
		filename = openAIImageFilename(0, contentType)
	}
	return newOpenAIImageReader(data, filename, contentType), nil
}

func newOpenAIImageReader(data []byte, filename, contentType string) openAIImageReader {
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if filename == "" || filename == "." || filename == "/" {
		filename = openAIImageFilename(0, contentType)
	}
	return openAIImageReader{
		Reader:      bytes.NewReader(data),
		filename:    filename,
		contentType: contentType,
	}
}

func openAIImageFilename(index int, contentType string) string {
	ext := ".png"
	if extensions, err := mime.ExtensionsByType(contentType); err == nil && len(extensions) > 0 {
		ext = extensions[0]
	}
	return fmt.Sprintf("image-%d%s", index+1, ext)
}

func (a *OpenAIAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error) {
	endpoint, err := openAITaskEndpoint(config, taskID, query...)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	if config.Organization != "" {
		req.Header.Set("OpenAI-Organization", config.Organization)
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := a.getHTTPClient(config).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai task status error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var task openAIAsyncTaskResponse
	if err := json.Unmarshal(rawBody, &task); err != nil {
		return nil, fmt.Errorf("decode openai task status response: %w", err)
	}
	return openAITaskStatusResponse(task, taskID), nil
}

func openAITaskEndpoint(config *ProviderConfig, taskID string, query ...map[string]string) (string, error) {
	if strings.TrimSpace(taskID) == "" {
		return "", fmt.Errorf("openai task id is required")
	}
	if config == nil || strings.TrimSpace(config.PollingURL) == "" {
		return "", fmt.Errorf("openai polling URL is required")
	}

	rawEndpoint := strings.TrimSpace(config.PollingURL)
	escapedTaskID := url.PathEscape(taskID)
	hasTaskTemplate := strings.Contains(rawEndpoint, "{task_id}")
	if hasTaskTemplate {
		rawEndpoint = strings.ReplaceAll(rawEndpoint, "{task_id}", escapedTaskID)
	}

	endpoint, err := url.Parse(rawEndpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", fmt.Errorf("openai polling URL must be an absolute http(s) URL: %q", config.PollingURL)
	}
	if !hasTaskTemplate {
		basePath := strings.TrimRight(endpoint.Path, "/")
		baseEscapedPath := strings.TrimRight(endpoint.EscapedPath(), "/")
		endpoint.Path = basePath + "/" + taskID
		endpoint.RawPath = baseEscapedPath + "/" + escapedTaskID
	}

	values := endpoint.Query()
	if len(query) > 0 {
		for key, value := range query[0] {
			values.Set(key, value)
		}
	}
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

func openAITaskStatusResponse(task openAIAsyncTaskResponse, fallbackTaskID string) *dto.TaskStatusResponse {
	resultURL := firstNonEmptyString(task.URL, openAIFirstImageURL(task.Data), task.VideoURL)
	status := firstNonEmptyString(task.Status, task.State)
	if status == "" {
		switch {
		case openAIErrorMessage(task) != "":
			status = dto.TaskStatusFailed
		case resultURL != "":
			status = dto.TaskStatusSucceeded
		}
	}

	return &dto.TaskStatusResponse{
		RequestID: task.RequestID,
		Output: dto.TaskStatusOutput{
			TaskID:     firstNonEmptyString(task.TaskID, task.ID, fallbackTaskID),
			TaskStatus: status,
			URL:        resultURL,
			VideoURL:   task.VideoURL,
			Code:       openAIErrorCode(task),
			Message:    openAIErrorMessage(task),
		},
	}
}

func openAIFirstImageURL(data []dto.ImageData) string {
	if len(data) == 0 {
		return ""
	}
	return data[0].URL
}

func openAIErrorCode(task openAIAsyncTaskResponse) string {
	if task.Error != nil && task.Error.Code != nil {
		return strings.TrimSpace(fmt.Sprint(task.Error.Code))
	}
	if task.Code == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Code))
}

func openAIErrorMessage(task openAIAsyncTaskResponse) string {
	if task.Error != nil && strings.TrimSpace(task.Error.Message) != "" {
		return task.Error.Message
	}
	return task.Message
}

func (a *OpenAIAdaptor) ListTasks(ctx context.Context, config *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	return nil, fmt.Errorf("task list not supported by OpenAI")
}

func (a *OpenAIAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by OpenAI adaptor")
}
