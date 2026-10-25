package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

type googleInteractionEvent struct {
	Type   string                `json:"event_type"`
	ID     string                `json:"interaction_id"`
	Status string                `json:"status"`
	Raw    json.RawMessage       `json:"-"`
	Index  int                   `json:"index"`
	Step   googleInteractionStep `json:"step"`
	Delta  struct {
		googleInteractionContent
		Arguments json.RawMessage `json:"arguments"`
	} `json:"delta"`
	Interaction googleInteraction `json:"interaction"`
	Usage       json.RawMessage   `json:"usage"`
	Metadata    struct {
		TotalUsage json.RawMessage `json:"total_usage"`
	} `json:"metadata"`
	Error struct {
		Message string      `json:"message"`
		Code    interface{} `json:"code"`
	} `json:"error"`
}

type googleInteractionToolStream struct {
	id, name  string
	arguments bool
}

type googleInteractionStream struct {
	body    io.ReadCloser
	decoder *utils.SSEDecoder
	pending []*dto.StreamToken
	err     error
	usage   *dto.Usage
	tools   map[int]*googleInteractionToolStream
}

func (a *GoogleAdaptor) interactionStream(ctx context.Context, cfg *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	payload, err := googleInteractionPayload(request, true)
	if err != nil {
		return nil, err
	}
	version, err := googleInteractionVersion(request)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	response, err := googleInteractionRequest(streamCtx, cfg, http.MethodPost, googleInteractionEndpoint(cfg, version), payload)
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = bodyWithContext(streamCtx, response.Body)
	stream := &googleInteractionStream{body: response.Body, decoder: utils.NewSSEDecoder(response.Body), tools: make(map[int]*googleInteractionToolStream)}
	return newCancellableStream(stream, cancel), nil
}

func (s *googleInteractionStream) Close() error {
	return s.body.Close()
}

func (s *googleInteractionStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for len(s.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.err != nil {
			err := s.err
			s.err = io.EOF
			return nil, err
		}
		if !s.decoder.Next() {
			_ = s.Close()
			s.err = io.EOF
			if err := s.decoder.Err(); err != nil {
				return nil, err
			}
			return nil, io.ErrUnexpectedEOF
		}
		raw := s.decoder.Event()
		if len(raw.Data) == 0 {
			continue
		}
		var event googleInteractionEvent
		if err := json.Unmarshal(raw.Data, &event); err != nil {
			_ = s.Close()
			s.err = io.EOF
			return nil, fmt.Errorf("decode google interaction event: %w", err)
		}
		event.Raw = append(json.RawMessage(nil), raw.Data...)
		if event.Type == "" {
			event.Type = raw.Type
		}
		s.consume(event)
	}
	token := s.pending[0]
	s.pending = s.pending[1:]
	return token, nil
}

func (s *googleInteractionStream) consume(event googleInteractionEvent) {
	terminal := false
	first := len(s.pending)
	switch event.Type {
	case "interaction.created", "interaction.status_update":
		s.pending = append(s.pending, &dto.StreamToken{Type: "progress", Text: firstNonEmptyString(event.Status, event.Interaction.Status), Metadata: map[string]interface{}{"interaction_id": firstNonEmptyString(event.ID, event.Interaction.ID)}})
	case "step.start":
		if event.Step.Type == "function_call" {
			state := &googleInteractionToolStream{id: event.Step.ID, name: event.Step.Name}
			s.tools[event.Index] = state
			arguments := ""
			if len(event.Step.Arguments) > 0 && string(event.Step.Arguments) != "{}" && string(event.Step.Arguments) != "null" {
				arguments, state.arguments = string(event.Step.Arguments), true
			}
			s.pending = append(s.pending, interactionToolToken(event.Index, state.id, state.name, arguments))
		} else if event.Step.Type == "model_output" {
			for _, content := range event.Step.Content {
				s.contentToken(event.Index, content)
			}
		}
	case "step.delta":
		if event.Delta.Type == "arguments_delta" {
			var arguments string
			if err := json.Unmarshal(event.Delta.Arguments, &arguments); err != nil {
				s.err, terminal = fmt.Errorf("decode google function arguments delta: %w", err), true
				break
			}
			state := s.tools[event.Index]
			if state == nil {
				state = &googleInteractionToolStream{}
				s.tools[event.Index] = state
			}
			state.arguments = true
			s.pending = append(s.pending, interactionToolToken(event.Index, "", "", arguments))
		} else {
			s.contentToken(event.Index, event.Delta.googleInteractionContent)
		}
	case "step.stop":
		if state := s.tools[event.Index]; state != nil && !state.arguments {
			state.arguments = true
			s.pending = append(s.pending, interactionToolToken(event.Index, "", "", "{}"))
		}
	case "interaction.completed", "interaction.failed", "interaction.cancelled":
		terminal = true
		s.err = googleInteractionError(event.Interaction)
		if s.err == nil && event.Type != "interaction.completed" {
			s.err = fmt.Errorf("google %s", event.Type)
		}
		if s.err == nil {
			s.err = io.EOF
		}
	case "error":
		terminal = true
		s.err = fmt.Errorf("google interaction error (%v): %s", event.Error.Code, event.Error.Message)
	}
	rawUsage := event.Usage
	if len(event.Metadata.TotalUsage) > 0 {
		rawUsage = event.Metadata.TotalUsage
	}
	if len(event.Interaction.Usage) > 0 {
		rawUsage = event.Interaction.Usage
	}
	usage := usagePointer(googleInteractionUsage(rawUsage))
	if usage != nil {
		s.usage = usage
	}
	if terminal && usage == nil {
		usage = s.usage
	}
	if usage != nil {
		s.pending = append(s.pending, &dto.StreamToken{Type: "usage", Usage: usage})
	}
	if terminal && s.err == io.EOF {
		finish := firstNonEmptyString(event.Interaction.Status, strings.TrimPrefix(event.Type, "interaction."))
		s.pending = append(s.pending, &dto.StreamToken{Type: "finish", FinishReason: finish, Metadata: map[string]interface{}{"interaction_id": event.Interaction.ID}})
	}
	if len(event.Raw) > 0 {
		if len(s.pending) == first {
			s.pending = append(s.pending, &dto.StreamToken{Type: "event", Metadata: map[string]interface{}{"step_index": event.Index}})
		}
		token := s.pending[first]
		if token.Metadata == nil {
			token.Metadata = make(map[string]interface{})
		}
		token.Metadata["event"] = event.Raw
	}
	if terminal {
		_ = s.Close()
	}
}

func (s *googleInteractionStream) contentToken(index int, content googleInteractionContent) {
	if content.Type != "text" && content.Type != "image" && content.Type != "audio" && content.Type != "video" && content.Type != "document" {
		return
	}
	token := &dto.StreamToken{Type: content.Type, Text: content.Text, URL: content.URI, Metadata: map[string]interface{}{"step_index": index}}
	if content.Type != "text" {
		token.Index = index
	}
	if content.Data != "" {
		token.Text = content.Data
	}
	if content.MIMEType != "" {
		token.Metadata["mime_type"] = content.MIMEType
	}
	s.pending = append(s.pending, token)
}

func interactionToolToken(index int, id, name, arguments string) *dto.StreamToken {
	return &dto.StreamToken{Type: "function_call", ToolCalls: []dto.ToolCallDelta{{Index: index, ID: id, Type: "function", Function: dto.ToolCallFunctionDelta{Name: name, Arguments: arguments}}}}
}
