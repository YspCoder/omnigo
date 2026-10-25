package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

type openAIEventConverter func([]byte, string) ([]*dto.StreamToken, bool, error)

// openAIEventStream delivers terminal metadata before a terminal error, and
// distinguishes an interrupted stream from a completed generation.
type openAIEventStream struct {
	decoder    *utils.SSEDecoder
	body       io.ReadCloser
	convert    openAIEventConverter
	pending    []*dto.StreamToken
	pendingErr error
	done       bool
	closed     bool
	finished   bool
}

func openAIHTTPStream(ctx context.Context, response *http.Response, cancel context.CancelFunc, convert openAIEventConverter) (dto.TokenStream, error) {
	response.Body = bodyWithContext(ctx, response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer cancel()
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("openai stream error: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return newCancellableStream(&openAIEventStream{decoder: utils.NewSSEDecoder(response.Body), body: response.Body, convert: convert}, cancel), nil
}

func (s *openAIEventStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for len(s.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.pendingErr != nil {
			err := s.pendingErr
			s.pendingErr = nil
			return nil, err
		}
		if s.done {
			return nil, io.EOF
		}
		if !s.decoder.Next() {
			s.done = true
			_ = s.closeSource()
			if err := s.decoder.Err(); err != nil {
				return nil, err
			}
			if !s.finished {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, io.EOF
		}
		event := s.decoder.Event()
		if bytes.Equal(bytes.TrimSpace(event.Data), []byte("[DONE]")) {
			s.done = true
			_ = s.closeSource()
			if !s.finished {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, io.EOF
		}
		tokens, done, err := s.convert(event.Data, event.Type)
		s.pending = append(s.pending, tokens...)
		for _, token := range tokens {
			if token.FinishReason != "" {
				s.finished = true
			}
		}
		if done || err != nil {
			s.done, s.pendingErr = true, err
			_ = s.closeSource()
		}
	}
	token := s.pending[0]
	s.pending[0] = nil
	s.pending = s.pending[1:]
	return token, nil
}

func (s *openAIEventStream) closeSource() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

func (s *openAIEventStream) Close() error {
	s.done, s.pending, s.pendingErr = true, nil, nil
	return s.closeSource()
}

func (a *OpenAIAdaptor) streamResponses(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	body, err := openAIResponsesBody(request, true)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	response, err := a.doOpenAIRequest(streamCtx, config, "/responses", "application/json", body)
	if err != nil {
		cancel()
		return nil, err
	}
	events := &openAIResponsesEvents{calls: make(map[int]*openAIResponseCall), text: make(map[string]string)}
	return openAIHTTPStream(streamCtx, response, cancel, events.convert)
}

type openAIResponseCall struct {
	id, name, arguments string
}

type openAIResponsesEvents struct {
	calls map[int]*openAIResponseCall
	text  map[string]string
}

type openAIResponseEvent struct {
	Type         string                    `json:"type"`
	Delta        string                    `json:"delta"`
	Text         string                    `json:"text"`
	Refusal      string                    `json:"refusal"`
	Arguments    string                    `json:"arguments"`
	ItemID       string                    `json:"item_id"`
	OutputIndex  int                       `json:"output_index"`
	ContentIndex int                       `json:"content_index"`
	Item         openAIResponsesOutputItem `json:"item"`
	Response     *openAIResponsesResponse  `json:"response"`
	Error        *openAIResponsesError     `json:"error"`
	Code         string                    `json:"code"`
	Message      string                    `json:"message"`
	Usage        json.RawMessage           `json:"usage"`
}

func (s *openAIResponsesEvents) convert(raw []byte, eventType string) ([]*dto.StreamToken, bool, error) {
	var event openAIResponseEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, true, err
	}
	event.Type = firstNonEmptyString(event.Type, eventType)
	if event.Error != nil {
		event.Type = "error"
	}
	metadata := map[string]interface{}{"event_type": event.Type, "item_id": event.ItemID, "content_index": event.ContentIndex}
	switch event.Type {
	case "response.output_text.delta", "response.refusal.delta":
		return s.textEvent(event.OutputIndex, event.ContentIndex, event.Type, event.Delta, false, metadata)
	case "response.output_text.done", "response.refusal.done":
		return s.textEvent(event.OutputIndex, event.ContentIndex, event.Type, firstNonEmptyString(event.Text, event.Refusal), true, metadata)
	case "response.output_item.added", "response.output_item.done":
		if event.Item.Type == "function_call" {
			token, err := s.callEvent(event.OutputIndex, event.Item.CallID, event.Item.Name, event.Item.Arguments, true)
			return openAITokenSlice(token), false, err
		}
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		full := event.Type == "response.function_call_arguments.done"
		arguments := event.Delta
		if full {
			arguments = event.Arguments
		}
		token, err := s.callEvent(event.OutputIndex, "", "", arguments, full)
		return openAITokenSlice(token), false, err
	case "response.completed", "response.incomplete", "response.failed":
		if event.Response == nil {
			return nil, true, fmt.Errorf("openai %s event has no response", event.Type)
		}
		if event.Response.Status == "" {
			event.Response.Status = strings.TrimPrefix(event.Type, "response.")
		}
		result, resultErr := openAIResponsesResult(event.Response)
		var tokens []*dto.StreamToken
		for index, item := range event.Response.Output {
			if item.Type == "function_call" {
				token, err := s.callEvent(index, item.CallID, item.Name, item.Arguments, true)
				tokens = append(tokens, openAITokenSlice(token)...)
				if err != nil && resultErr == nil {
					resultErr = err
				}
			}
			for contentIndex, content := range item.Content {
				if content.Type != "output_text" && content.Type != "refusal" {
					continue
				}
				text := firstNonEmptyString(content.Text, content.Refusal)
				fragment, _, err := s.textEvent(index, contentIndex, "response."+content.Type+".done", text, true, metadata)
				tokens = append(tokens, fragment...)
				if err != nil && resultErr == nil {
					resultErr = err
				}
			}
		}
		if len(event.Response.Output) == 0 && event.Response.OutputText != "" {
			fragment, _, err := s.textEvent(0, 0, "response.output_text.done", event.Response.OutputText, true, metadata)
			tokens = append(tokens, fragment...)
			if err != nil && resultErr == nil {
				resultErr = err
			}
		}
		token := &dto.StreamToken{Type: "finish", FinishReason: result.Choices[0].FinishReason, Usage: usagePointer(result.Usage), Metadata: map[string]interface{}{
			"response_id": result.ID, "status": result.Status, "error_code": result.ErrorCode,
			"error_message": result.ErrorMessage, "event_type": event.Type, "event": json.RawMessage(append([]byte(nil), raw...)),
		}}
		if resultErr != nil {
			token.Type = "error"
		}
		return append(tokens, token), true, resultErr
	case "error":
		message, code := event.Message, event.Code
		if event.Error != nil {
			message, code = event.Error.Message, event.Error.Code
		}
		metadata["code"] = code
		message = firstNonEmptyString(message, "response stream failed")
		return []*dto.StreamToken{{Type: "error", Text: message, Usage: usagePointer(usageFromJSON(event.Usage)), Metadata: metadata}}, true, fmt.Errorf("openai responses stream: %s", message)
	}
	metadata["event"] = json.RawMessage(append([]byte(nil), raw...))
	return []*dto.StreamToken{{Type: "progress", Metadata: metadata}}, false, nil
}

func (s *openAIResponsesEvents) callEvent(index int, id, name, arguments string, full bool) (*dto.StreamToken, error) {
	call := s.calls[index]
	if call == nil {
		call = &openAIResponseCall{}
		s.calls[index] = call
	}
	delta := dto.ToolCallDelta{Index: index, Type: "function"}
	if call.id == "" && id != "" {
		delta.ID, call.id = id, id
	}
	if call.name == "" && name != "" {
		delta.Function.Name, call.name = name, name
	}
	if full {
		if !strings.HasPrefix(arguments, call.arguments) {
			return nil, fmt.Errorf("openai function call %d final arguments disagree with streamed arguments", index)
		}
		arguments = strings.TrimPrefix(arguments, call.arguments)
	}
	call.arguments += arguments
	delta.Function.Arguments = arguments
	if delta.ID == "" && delta.Function.Name == "" && arguments == "" {
		return nil, nil
	}
	return &dto.StreamToken{Type: "function_call", ToolCalls: []dto.ToolCallDelta{delta}, Metadata: map[string]interface{}{"output_index": index}}, nil
}

func (s *openAIResponsesEvents) textEvent(index, contentIndex int, eventType, text string, full bool, metadata map[string]interface{}) ([]*dto.StreamToken, bool, error) {
	kind := strings.TrimSuffix(strings.TrimSuffix(eventType, ".delta"), ".done")
	key := fmt.Sprintf("%d:%d:%s", index, contentIndex, kind)
	if full {
		if !strings.HasPrefix(text, s.text[key]) {
			return nil, false, fmt.Errorf("openai output %d final text disagrees with streamed text", index)
		}
		text = strings.TrimPrefix(text, s.text[key])
	}
	s.text[key] += text
	if text == "" {
		return nil, false, nil
	}
	metadata = maps.Clone(metadata)
	metadata["output_index"], metadata["content_index"] = index, contentIndex
	return []*dto.StreamToken{{Type: "text", Text: text, Metadata: metadata}}, false, nil
}

func openAITokenSlice(token *dto.StreamToken) []*dto.StreamToken {
	if token == nil {
		return nil
	}
	return []*dto.StreamToken{token}
}
