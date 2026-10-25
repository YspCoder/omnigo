package adapter

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/YspCoder/omnigo/dto"
	"github.com/liushuangls/go-anthropic/v2"
)

func (a *AnthropicAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	params, err := anthropicRequest(request)
	if err != nil {
		return nil, err
	}
	client, err := a.getClientWithError(config)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream := &anthropicTokenStream{ctx: streamCtx, events: make(chan anthropicStreamResult)}
	go stream.run(client, params)
	return newCancellableStream(stream, cancel), nil
}

type anthropicStreamResult struct {
	token *dto.StreamToken
	err   error
}

// The SDK drives callbacks; an unbuffered channel preserves pull backpressure.
type anthropicTokenStream struct {
	ctx    context.Context
	events chan anthropicStreamResult
}

func (s *anthropicTokenStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case event, ok := <-s.events:
		if !ok {
			return nil, io.EOF
		}
		return event.token, event.err
	}
}

func (s *anthropicTokenStream) Close() error { return nil }

func (s *anthropicTokenStream) send(token *dto.StreamToken, err error) {
	select {
	case s.events <- anthropicStreamResult{token, err}:
	case <-s.ctx.Done():
	}
}

func (s *anthropicTokenStream) run(client *anthropic.Client, params anthropic.MessagesRequest) {
	defer close(s.events)
	readCtx, cancelRead := context.WithCancel(s.ctx)
	defer cancelRead()
	var usage anthropic.MessagesUsage
	started, stopped := false, false
	toolArguments := make(map[int]bool)
	streamRequest := anthropic.MessagesStreamRequest{
		MessagesRequest: params,
		OnMessageStart: func(event anthropic.MessagesEventMessageStartData) {
			started, usage = true, event.Message.Usage
			s.send(&dto.StreamToken{Type: "usage", Usage: usagePointer(anthropicUsage(usage)),
				Metadata: map[string]any{"id": event.Message.ID, "model": event.Message.Model}}, nil)
		},
		OnContentBlockStart: func(event anthropic.MessagesEventContentBlockStartData) {
			block := event.ContentBlock
			switch block.Type {
			case anthropic.MessagesContentTypeText:
				if text := block.GetText(); text != "" {
					s.send(&dto.StreamToken{Type: "text", Text: text}, nil)
				}
			case anthropic.MessagesContentTypeToolUse:
				if call := block.MessageContentToolUse; call != nil {
					arguments := ""
					if raw := bytes.TrimSpace(call.Input); len(raw) > 0 && !bytes.Equal(raw, []byte("{}")) {
						arguments = string(raw)
					}
					toolArguments[event.Index] = arguments != ""
					s.send(&dto.StreamToken{Type: "function_call", ToolCalls: []dto.ToolCallDelta{{
						Index: event.Index, ID: call.ID, Type: "function",
						Function: dto.ToolCallFunctionDelta{Name: call.Name, Arguments: arguments},
					}}}, nil)
				}
			default:
				s.send(anthropicNativeToken("content_block_start", event.Index, block), nil)
			}
		},
		OnContentBlockDelta: func(event anthropic.MessagesEventContentBlockDeltaData) {
			switch event.Delta.Type {
			case anthropic.MessagesContentTypeTextDelta:
				s.send(&dto.StreamToken{Type: "text", Text: event.Delta.GetText()}, nil)
			case anthropic.MessagesContentTypeInputJsonDelta:
				if _, clientTool := toolArguments[event.Index]; clientTool && event.Delta.PartialJson != nil {
					part := *event.Delta.PartialJson
					toolArguments[event.Index] = toolArguments[event.Index] || part != ""
					s.send(&dto.StreamToken{Type: "function_call", ToolCalls: []dto.ToolCallDelta{{
						Index: event.Index, Function: dto.ToolCallFunctionDelta{Arguments: part},
					}}}, nil)
				} else {
					s.send(anthropicNativeToken("content_block_delta", event.Index, event.Delta), nil)
				}
			default:
				s.send(anthropicNativeToken("content_block_delta", event.Index, event.Delta), nil)
			}
		},
		OnContentBlockStop: func(event anthropic.MessagesEventContentBlockStopData, block anthropic.MessageContent) {
			if hasArguments, clientTool := toolArguments[event.Index]; clientTool && !hasArguments {
				s.send(&dto.StreamToken{Type: "function_call", ToolCalls: []dto.ToolCallDelta{{
					Index: event.Index, Function: dto.ToolCallFunctionDelta{Arguments: "{}"},
				}}}, nil)
			}
		},
		OnMessageDelta: func(event anthropic.MessagesEventMessageDeltaData) {
			usage.OutputTokens = event.Usage.OutputTokens
			if event.Usage.InputTokens != 0 {
				usage.InputTokens = event.Usage.InputTokens
			}
			if event.Usage.CacheReadInputTokens != 0 {
				usage.CacheReadInputTokens = event.Usage.CacheReadInputTokens
			}
			if event.Usage.CacheCreationInputTokens != 0 {
				usage.CacheCreationInputTokens = event.Usage.CacheCreationInputTokens
			}
			if event.Usage.ServerToolUse != nil {
				usage.ServerToolUse = event.Usage.ServerToolUse
			}
			if event.Usage.CacheCreation != (anthropic.MessageUsageCacheCreation{}) {
				usage.CacheCreation = event.Usage.CacheCreation
			}
			s.send(&dto.StreamToken{Type: "usage", Usage: usagePointer(anthropicUsage(usage))}, nil)
		},
		OnMessageStop: func(anthropic.MessagesEventMessageStopData) {
			stopped = true
			cancelRead() // A terminal event ends the stream even if a proxy keeps the body open.
		},
	}
	response, err := client.CreateMessagesStream(readCtx, streamRequest)
	if stopped && errors.Is(err, context.Canceled) && s.ctx.Err() == nil {
		err = nil
	}
	if err == nil && !stopped {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		s.send(nil, err)
		return
	}
	var finalUsage *dto.Usage
	if started {
		finalUsage = usagePointer(anthropicUsage(usage))
	}
	s.send(&dto.StreamToken{Type: "finish", FinishReason: string(response.StopReason), Usage: finalUsage}, nil)
}

func anthropicNativeToken(eventType string, index int, content anthropic.MessageContent) *dto.StreamToken {
	return &dto.StreamToken{Type: string(content.Type), Metadata: map[string]any{
		"event_type": eventType, "content_index": index, "content": content,
	}}
}
