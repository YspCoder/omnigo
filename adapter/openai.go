// Package adapter provides OpenAI adaptor implementation using the official SDK.
package adapter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

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
