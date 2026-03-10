// Package adapter provides Alibaba DashScope adaptor implementation.
package adapter

import (
	"context"
	"fmt"

	"github.com/YspCoder/omnigo/dto"
)

// AliAdaptor converts requests and responses for DashScope APIs.
// Most modern DashScope models are OpenAI compatible.
type AliAdaptor struct{}

func (a *AliAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return (&OpenAIAdaptor{}).Chat(ctx, config, request)
}

func (a *AliAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return (&OpenAIAdaptor{}).Stream(ctx, config, request)
}

func (a *AliAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return (&OpenAIAdaptor{}).Media(ctx, config, request)
}

func (a *AliAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status query for Ali not yet implemented in refactored adaptor")
}

func (a *AliAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Ali adaptor")
}
