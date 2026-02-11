// Package adapter provides Volcengine Ark (火山方舟) adaptor implementation.
package adapter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/utils"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
)

// ArkAdaptor converts requests and responses for Volcengine Ark APIs using the official SDK.
type ArkAdaptor struct {
	client *arkruntime.Client
}

func (a *ArkAdaptor) getClient(config *ProviderConfig) *arkruntime.Client {
	if a.client != nil {
		return a.client
	}

	opts := []arkruntime.ConfigOption{
		arkruntime.WithRegion(config.Region),
	}
	if config.BaseURL != "" {
		opts = append(opts, arkruntime.WithBaseUrl(config.BaseURL))
	}
	if config.HTTPClient != nil {
		opts = append(opts, arkruntime.WithHTTPClient(config.HTTPClient))
	}

	if config.APIKey != "" {
		a.client = arkruntime.NewClientWithApiKey(config.APIKey, opts...)
	} else {
		a.client = arkruntime.NewClientWithAkSk(config.AccessKey, config.SecretKey, opts...)
	}
	return a.client
}

func (a *ArkAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	client := a.getClient(config)

	req := model.CreateChatCompletionRequest{
		Model:    request.Model,
		Messages: make([]*model.ChatCompletionMessage, len(request.Messages)),
	}
	for i, m := range request.Messages {
		req.Messages[i] = &model.ChatCompletionMessage{
			Role:    m.Role,
			Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(fmt.Sprint(m.Content))},
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
		content := ""
		if c.Message.Content != nil && c.Message.Content.StringValue != nil {
			content = *c.Message.Content.StringValue
		}
		res.Choices[i] = dto.ChatChoice{
			Index: c.Index,
			Message: dto.Message{
				Role:    c.Message.Role,
				Content: content,
			},
			FinishReason: string(c.FinishReason),
		}
	}
	return res, nil
}

type arkStreamWrapper struct {
	reader *utils.ChatCompletionStreamReader
}

func (w *arkStreamWrapper) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.reader.Recv()
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

func (w *arkStreamWrapper) Close() error {
	return w.reader.Close()
}

func (a *ArkAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	client := a.getClient(config)

	req := model.CreateChatCompletionRequest{
		Model:    request.Model,
		Messages: make([]*model.ChatCompletionMessage, len(request.Messages)),
	}
	for i, m := range request.Messages {
		req.Messages[i] = &model.ChatCompletionMessage{
			Role:    m.Role,
			Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(fmt.Sprint(m.Content))},
		}
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	return &arkStreamWrapper{reader: stream}, nil
}

func (a *ArkAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	client := a.getClient(config)

	if request.Type == dto.MediaTypeImage {
		req := model.GenerateImagesRequest{
			Model:  request.Model,
			Prompt: request.Prompt,
		}
		if request.Size != "" {
			req.Size = &request.Size
		}

		resp, err := client.GenerateImages(ctx, req)
		if err != nil {
			return nil, err
		}

		mediaRes := &dto.MediaResponse{
			Created: resp.Created,
		}
		for _, d := range resp.Data {
			data := dto.ImageData{}
			if d.Url != nil {
				data.URL = *d.Url
			}
			if d.B64Json != nil {
				data.B64JSON = *d.B64Json
			}
			mediaRes.Data = append(mediaRes.Data, data)
		}
		if len(mediaRes.Data) > 0 {
			mediaRes.URL = mediaRes.Data[0].URL
		}
		return mediaRes, nil
	}

	// Video generation
	req := model.CreateContentGenerationTaskRequest{
		Model: request.Model,
		Content: []*model.CreateContentGenerationContentItem{
			{
				Type: model.ContentGenerationContentItemTypeText,
				Text: &request.Prompt,
			},
		},
	}
	if request.Duration > 0 {
		duration := int64(request.Duration)
		req.Duration = &duration
	}

	resp, err := client.CreateContentGenerationTask(ctx, req)
	if err != nil {
		return nil, err
	}

	return &dto.MediaResponse{
		TaskID: resp.ID,
	}, nil
}

func (a *ArkAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	client := a.getClient(config)

	resp, err := client.GetContentGenerationTask(ctx, model.GetContentGenerationTaskRequest{
		ID: taskID,
	})
	if err != nil {
		return nil, err
	}

	statusRes := &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     resp.ID,
			TaskStatus: resp.Status,
		},
	}

	if resp.Error != nil {
		statusRes.Output.Code = resp.Error.Code
		statusRes.Output.Message = resp.Error.Message
	}

	if resp.Content.VideoURL != "" {
		statusRes.Output.VideoURL = resp.Content.VideoURL
	} else if resp.Content.FileURL != "" {
		statusRes.Output.VideoURL = resp.Content.FileURL
	}

	return statusRes, nil
}

type arkMediaStreamWrapper struct {
	reader *utils.ImageGenerationStreamReader
}

func (w *arkMediaStreamWrapper) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.reader.Recv()
	if err != nil {
		return nil, err
	}

	token := &dto.StreamToken{
		Type:  resp.Type,
		Index: int(resp.ImageIndex),
	}
	if resp.Url != nil {
		token.URL = *resp.Url
	}
	if resp.B64Json != nil {
		token.Text = *resp.B64Json
	}
	return token, nil
}

func (w *arkMediaStreamWrapper) Close() error {
	return w.reader.Close()
}

type arkVideoProgressStreamWrapper struct {
	client *arkruntime.Client
	taskID string
	ctx    context.Context
	done   bool
	last   string
}

func (w *arkVideoProgressStreamWrapper) Next(ctx context.Context) (*dto.StreamToken, error) {
	if w.done {
		return nil, io.EOF
	}

	for {
		resp, err := w.client.GetContentGenerationTask(ctx, model.GetContentGenerationTaskRequest{
			ID: w.taskID,
		})
		if err != nil {
			return nil, err
		}

		if resp.Status == w.last && resp.Status != model.StatusSucceeded && resp.Status != model.StatusFailed {
			// No change, wait and retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}

		w.last = resp.Status
		token := &dto.StreamToken{
			Type: "progress",
			Text: resp.Status,
		}

		switch resp.Status {
		case model.StatusSucceeded:
			w.done = true
			token.Type = "url"
			if resp.Content.VideoURL != "" {
				token.URL = resp.Content.VideoURL
			} else {
				token.URL = resp.Content.FileURL
			}
			return token, nil
		case model.StatusFailed:
			w.done = true
			token.Type = "error"
			if resp.Error != nil {
				token.Text = resp.Error.Message
			}
			return token, nil
		default:
			// queued, running
			return token, nil
		}
	}
}

func (w *arkVideoProgressStreamWrapper) Close() error {
	return nil
}

func (a *ArkAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	client := a.getClient(config)

	if request.Type == dto.MediaTypeImage {
		req := model.GenerateImagesRequest{
			Model:  request.Model,
			Prompt: request.Prompt,
		}
		if request.Size != "" {
			req.Size = &request.Size
		}

		stream, err := client.GenerateImagesStreaming(ctx, req)
		if err != nil {
			return nil, err
		}

		return &arkMediaStreamWrapper{reader: stream}, nil
	}

	// Video generation streaming (Status polling)
	req := model.CreateContentGenerationTaskRequest{
		Model: request.Model,
		Content: []*model.CreateContentGenerationContentItem{
			{
				Type: model.ContentGenerationContentItemTypeText,
				Text: &request.Prompt,
			},
		},
	}
	if request.Duration > 0 {
		duration := int64(request.Duration)
		req.Duration = &duration
	}

	resp, err := client.CreateContentGenerationTask(ctx, req)
	if err != nil {
		return nil, err
	}

	return &arkVideoProgressStreamWrapper{
		client: client,
		taskID: resp.ID,
		ctx:    ctx,
	}, nil
}
