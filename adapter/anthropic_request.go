package adapter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/liushuangls/go-anthropic/v2"
)

func anthropicRequest(request *dto.MediaRequest) (anthropic.MessagesRequest, error) {
	params := anthropic.MessagesRequest{}
	if request == nil {
		return params, fmt.Errorf("anthropic request is nil")
	}
	// Decode supported native options once for both ordinary and streaming calls.
	options := make(map[string]any)
	for _, key := range []string{"top_p", "top_k", "stop_sequences", "thinking", "cache_control", "metadata"} {
		if value, ok := request.Options[key]; ok {
			options[key] = value
		}
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return params, fmt.Errorf("anthropic options: %w", err)
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("anthropic options: %w", err)
	}
	params.Model, params.MaxTokens = anthropic.Model(request.Model), request.MaxTokens
	if temperature, ok := requestTemperature(request); ok {
		params.SetTemperature(float32(temperature))
		// New Claude models accept one sampling control, while the shared
		// client supplies both defaults. Preserve the existing temperature choice.
		params.TopP = nil
	}
	if params.Thinking != nil && params.Thinking.Type != anthropic.ThinkingTypeDisabled {
		params.Temperature, params.TopP, params.TopK = nil, nil, nil
	}
	if request.Schema != nil {
		raw, err := json.Marshal(request.Schema)
		if err != nil {
			return params, fmt.Errorf("anthropic schema: %w", err)
		}
		params.OutputConfig = &anthropic.OutputConfig{Format: &anthropic.OutputFormat{
			Type: anthropic.OutputFormatJsonSchema, Schema: json.RawMessage(raw),
		}}
	}
	var systems []string
	for _, message := range request.Messages {
		content, err := anthropicMessageContent(message)
		if err != nil {
			return params, err
		}
		if message.Role == "system" || message.Role == "developer" {
			for _, block := range content {
				if block.Type != anthropic.MessagesContentTypeText {
					return params, fmt.Errorf("anthropic system messages require text blocks")
				}
				params.MultiSystem = append(params.MultiSystem, anthropic.MessageSystemPart{
					Type: "text", Text: block.GetText(), CacheControl: block.CacheControl,
				})
				systems = append(systems, block.GetText())
			}
			continue
		}
		role := anthropic.ChatRole(message.Role)
		if message.Role == "tool" {
			if message.ToolCallID == "" {
				return params, fmt.Errorf("anthropic tool result requires tool_call_id")
			}
			role = anthropic.RoleUser
			id := message.ToolCallID
			content = []anthropic.MessageContent{{Type: anthropic.MessagesContentTypeToolResult,
				MessageContentToolResult: &anthropic.MessageContentToolResult{ToolUseID: &id, Content: content},
			}}
		}
		if role != anthropic.RoleUser && role != anthropic.RoleAssistant {
			return params, fmt.Errorf("anthropic does not support message role %q", message.Role)
		}
		if n := len(params.Messages); n > 0 && params.Messages[n-1].Role == role {
			params.Messages[n-1].Content = append(params.Messages[n-1].Content, content...)
		} else {
			params.Messages = append(params.Messages, anthropic.Message{Role: role, Content: content})
		}
	}
	// Keep the existing string system representation unless cache controls need blocks.
	cacheSystem := false
	for _, part := range params.MultiSystem {
		cacheSystem = cacheSystem || part.CacheControl != nil
	}
	if !cacheSystem {
		params.System, params.MultiSystem = strings.Join(systems, "\n\n"), nil
	}
	if len(params.Messages) == 0 && request.Prompt != "" {
		params.Messages = []anthropic.Message{anthropic.NewUserTextMessage(request.Prompt)}
	}
	for _, tool := range request.Tools {
		if tool.Type != "" && tool.Type != "function" {
			return params, fmt.Errorf("anthropic unified tools require type function, got %q", tool.Type)
		}
		if tool.Function.Name == "" {
			return params, fmt.Errorf("anthropic tool requires a function name")
		}
		schema := tool.Function.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		params.Tools = append(params.Tools, anthropic.ToolDefinition{
			Name: tool.Function.Name, Description: tool.Function.Description, InputSchema: schema,
		})
	}
	params.ToolChoice, err = anthropicToolChoice(request.ToolChoice)
	return params, err
}

func anthropicToolChoice(value any) (*anthropic.ToolChoice, error) {
	if value == nil {
		return nil, nil
	}
	choice := anthropic.ToolChoice{}
	if text, ok := value.(string); ok {
		choice.Type = text
	} else {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("anthropic tool_choice: %w", err)
		}
		var decoded struct {
			anthropic.ToolChoice
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("anthropic tool_choice: %w", err)
		}
		choice = decoded.ToolChoice
		if choice.Type == "function" {
			choice.Type, choice.Name = "tool", decoded.Function.Name
		}
	}
	if choice.Type == "required" {
		choice.Type = "any"
	}
	switch choice.Type {
	case "auto", "any", "none":
	case "tool":
		if choice.Name == "" {
			return nil, fmt.Errorf("anthropic tool_choice requires a name")
		}
	default:
		return nil, fmt.Errorf("unsupported anthropic tool_choice %q", choice.Type)
	}
	return &choice, nil
}

func anthropicMessageContent(message dto.Message) ([]anthropic.MessageContent, error) {
	content, err := anthropicContent(message.Content)
	if err != nil {
		return nil, err
	}
	if message.VideoURL != "" {
		return nil, fmt.Errorf("anthropic does not support video input")
	}
	for _, media := range []struct{ kind, url string }{{"image", message.ImageURL}, {"document", message.FileURL}} {
		if media.url != "" {
			block, err := anthropicURLContent(media.kind, media.url)
			if err != nil {
				return nil, err
			}
			content = append(content, block)
		}
	}
	for _, call := range message.ToolCalls {
		if message.Role != "assistant" || call.ID == "" || call.Function.Name == "" || (call.Type != "" && call.Type != "function") {
			return nil, fmt.Errorf("anthropic tool calls require assistant role, id and function name")
		}
		found := false
		for _, block := range content {
			found = found || (block.MessageContentToolUse != nil && block.MessageContentToolUse.ID == call.ID)
		}
		if found { // A native response may carry the same call in Content and ToolCalls.
			continue
		}
		arguments := call.Function.Arguments
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(arguments, &object) != nil || object == nil {
			return nil, fmt.Errorf("anthropic tool %q arguments must be a JSON object", call.Function.Name)
		}
		content = append(content, anthropic.MessageContent{Type: anthropic.MessagesContentTypeToolUse,
			MessageContentToolUse: anthropic.NewMessageContentToolUse(call.ID, call.Function.Name, arguments),
		})
	}
	return content, nil
}

func anthropicContent(value any) ([]anthropic.MessageContent, error) {
	if value == nil {
		return nil, nil
	}
	if text, ok := value.(string); ok {
		if text == "" {
			return nil, nil
		}
		return []anthropic.MessageContent{anthropic.NewTextMessageContent(text)}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("anthropic content: %w", err)
	}
	var blocks []json.RawMessage
	if len(raw) > 0 && raw[0] == '{' {
		blocks = []json.RawMessage{raw}
	} else if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("anthropic content requires text or content blocks: %w", err)
	}
	content := make([]anthropic.MessageContent, 0, len(blocks))
	for _, rawBlock := range blocks {
		var block anthropic.MessageContent
		if err := json.Unmarshal(rawBlock, &block); err != nil {
			return nil, fmt.Errorf("anthropic content block: %w", err)
		}
		switch block.Type {
		case "image_url":
			var image struct {
				ImageURL json.RawMessage `json:"image_url"`
			}
			_ = json.Unmarshal(rawBlock, &image)
			var address string
			if json.Unmarshal(image.ImageURL, &address) != nil {
				var detail struct {
					URL string `json:"url"`
				}
				_ = json.Unmarshal(image.ImageURL, &detail)
				address = detail.URL
			}
			block, err = anthropicURLContent("image", address)
			if err != nil {
				return nil, err
			}
		case anthropic.MessagesContentTypeText, anthropic.MessagesContentTypeImage,
			anthropic.MessagesContentTypeDocument, anthropic.MessagesContentTypeToolUse,
			anthropic.MessagesContentTypeToolResult, anthropic.MessagesContentTypeThinking,
			anthropic.MessagesContentTypeRedactedThinking, anthropic.MessagesContentTypeServerToolUse,
			anthropic.MessagesContentTypeWebSearchToolResult:
		default:
			return nil, fmt.Errorf("unsupported anthropic content type %q", block.Type)
		}
		if block.Type == anthropic.MessagesContentTypeImage || block.Type == anthropic.MessagesContentTypeDocument {
			if block.Source == nil {
				return nil, fmt.Errorf("anthropic %s requires a source", block.Type)
			}
			switch block.Source.Type {
			case anthropic.MessagesContentSourceTypeBase64, anthropic.MessagesContentSourceTypeUrl,
				anthropic.MessagesContentSourceTypeText, anthropic.MessagesContentSourceTypeContent:
			default:
				return nil, fmt.Errorf("unsupported anthropic source type %q", block.Source.Type)
			}
		}
		content = append(content, block)
	}
	return content, nil
}

func anthropicURLContent(kind, address string) (anthropic.MessageContent, error) {
	block := anthropic.MessageContent{Type: anthropic.MessagesContentType(kind)}
	if strings.HasPrefix(address, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(address, "data:"), ",")
		mediaType, encoded := strings.CutSuffix(header, ";base64")
		if !ok || !encoded || mediaType == "" || data == "" {
			return block, fmt.Errorf("anthropic %s requires a base64 data URL", kind)
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return block, fmt.Errorf("anthropic %s data URL: %w", kind, err)
		}
		block.Source = &anthropic.MessageContentSource{Type: anthropic.MessagesContentSourceTypeBase64, MediaType: mediaType, Data: data}
	} else {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return block, fmt.Errorf("anthropic %s requires an HTTP(S) or base64 data URL", kind)
		}
		block.Source = &anthropic.MessageContentSource{Type: anthropic.MessagesContentSourceTypeUrl, Url: address}
	}
	return block, nil
}
