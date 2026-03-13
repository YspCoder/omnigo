// Package adapter provides OpenAI adaptor implementation using the official SDK.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared"
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
	if openAIUsesResponsesAPI(request.Messages) {
		return a.chatWithResponsesAPI(ctx, config, request)
	}

	client := a.getClient(config)

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(request.Model),
		Messages: toOpenAIMessages(request.Messages),
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
		res.Choices[i] = dto.ChatChoice{
			Index: i,
			Message: dto.Message{
				Role:    string(c.Message.Role),
				Content: c.Message.Content,
			},
			FinishReason: string(c.FinishReason),
		}
	}
	if len(res.Choices) > 0 {
		res.Text = fmt.Sprint(res.Choices[0].Message.Content)
	}
	return res, nil
}

type openAIResponsesRequest struct {
	Model           string                     `json:"model"`
	Input           []openAIResponsesInputItem `json:"input"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	MaxOutputTokens *int                       `json:"max_output_tokens,omitempty"`
}

type openAIResponsesInputItem struct {
	Role    string                            `json:"role"`
	Content []openAIResponsesInputContentItem `json:"content"`
}

type openAIResponsesInputContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
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
	Type    string                         `json:"type"`
	Content []openAIResponsesOutputContent `json:"content"`
	Role    string                         `json:"role"`
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
}

func (a *OpenAIAdaptor) chatWithResponsesAPI(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	payload := openAIResponsesRequest{
		Model: request.Model,
		Input: toOpenAIResponsesInput(request.Messages),
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

	return &dto.MediaResponse{
		ID:    parsed.ID,
		Model: parsed.Model,
		Text:  text,
		Choices: []dto.ChatChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: text,
			},
		}},
		Usage: dto.Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

func openAIUsesResponsesAPI(messages []dto.Message) bool {
	for _, message := range messages {
		if message.FileURL != "" || message.FileID != "" {
			return true
		}
	}
	return false
}

func toOpenAIResponsesInput(messages []dto.Message) []openAIResponsesInputItem {
	items := make([]openAIResponsesInputItem, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		content := make([]openAIResponsesInputContentItem, 0, 2)
		if text := strings.TrimSpace(fmt.Sprint(message.Content)); text != "" && text != "<nil>" {
			content = append(content, openAIResponsesInputContentItem{
				Type: "input_text",
				Text: text,
			})
		}
		if message.FileURL != "" || message.FileID != "" {
			content = append(content, openAIResponsesInputContentItem{
				Type:     "input_file",
				FileURL:  message.FileURL,
				FileID:   message.FileID,
				Filename: firstNonEmpty(message.FileName, message.Name),
			})
		}
		if len(content) == 0 {
			continue
		}
		items = append(items, openAIResponsesInputItem{
			Role:    role,
			Content: content,
		})
	}
	return items
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

func toOpenAIMessages(messages []dto.Message) []openai.ChatCompletionMessageParamUnion {
	res := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, m := range messages {
		content := fmt.Sprint(m.Content)
		switch m.Role {
		case "system":
			res[i] = openai.SystemMessage(content)
		case "user":
			res[i] = openai.UserMessage(content)
		case "assistant":
			res[i] = openai.AssistantMessage(content)
		default:
			res[i] = openai.UserMessage(content)
		}
	}
	return res
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	client := a.getClient(config)

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(request.Model),
		Messages: toOpenAIMessages(request.Messages),
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

	switch request.Type {
	case dto.MediaTypeImage:
		params := openai.ImageGenerateParams{
			Prompt: mediaPromptWithSystem(request),
			Model:  openai.ImageModel(request.Model),
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

		resp, err := client.Images.Generate(ctx, params)
		if err != nil {
			return nil, err
		}

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
		return res, nil
	default:
		return nil, fmt.Errorf("unsupported media mode: %s", request.Type)
	}
}

func (a *OpenAIAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status not supported by OpenAI")
}

func (a *OpenAIAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by OpenAI adaptor")
}
