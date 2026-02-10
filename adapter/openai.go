// Package adapter provides OpenAI adaptor implementation using the official SDK.
package adapter

import (
	"context"
	"fmt"
	"io"

	"github.com/YspCoder/omnigo/dto"
	"github.com/sashabaranov/go-openai"
)

type OpenAIAdaptor struct {
	client *openai.Client
}

func (a *OpenAIAdaptor) getClient(config *ProviderConfig) *openai.Client {
	if a.client != nil {
		return a.client
	}
	
	c := openai.DefaultConfig(config.APIKey)
	if config.BaseURL != "" {
		c.BaseURL = config.BaseURL
	}
	if config.Organization != "" {
		c.OrgID = config.Organization
	}
	if config.HTTPClient != nil {
		c.HTTPClient = config.HTTPClient
	}
	
	a.client = openai.NewClientWithConfig(c)
	return a.client
}

func (a *OpenAIAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	client := a.getClient(config)

	req := openai.ChatCompletionRequest{
		Model:       request.Model,
		Temperature: float32(request.Temperature),
		MaxTokens:   request.MaxTokens,
		Messages:    make([]openai.ChatCompletionMessage, len(request.Messages)),
	}

	for i, m := range request.Messages {
		req.Messages[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: fmt.Sprint(m.Content),
		}
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	res := &dto.ChatResponse{
		Choices: make([]dto.ChatChoice, len(resp.Choices)),
		Usage: dto.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	for i, c := range resp.Choices {
		res.Choices[i] = dto.ChatChoice{
			Index: c.Index,
			Message: dto.Message{
				Role:    c.Message.Role,
				Content: c.Message.Content,
			},
			FinishReason: string(c.FinishReason),
		}
	}
	return res, nil
}

type openAIStreamWrapper struct {
	stream *openai.ChatCompletionStream
}

func (w *openAIStreamWrapper) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.stream.Recv()
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, io.EOF
	}

	return &dto.StreamToken{
		Text:  resp.Choices[0].Delta.Content,
		Type:  "text",
		Index: resp.Choices[0].Index,
	}, nil
}

func (w *openAIStreamWrapper) Close() error {
	return w.stream.Close()
}

func (a *OpenAIAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	client := a.getClient(config)

	req := openai.ChatCompletionRequest{
		Model:       request.Model,
		Temperature: float32(request.Temperature),
		MaxTokens:   request.MaxTokens,
		Messages:    make([]openai.ChatCompletionMessage, len(request.Messages)),
		Stream:      true,
	}

	for i, m := range request.Messages {
		req.Messages[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: fmt.Sprint(m.Content),
		}
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	return &openAIStreamWrapper{stream: stream}, nil
}

func (a *OpenAIAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	client := a.getClient(config)

	switch request.Type {
	case dto.MediaTypeImage:
		req := openai.ImageRequest{
			Prompt:         request.Prompt,
			Model:          request.Model,
			N:              request.N,
			Size:           request.Size,
			ResponseFormat: request.ResponseFormat,
		}
		resp, err := client.CreateImage(ctx, req)
		if err != nil {
			return nil, err
		}
		
		res := &dto.MediaResponse{
			Created: resp.Created,
		}
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
