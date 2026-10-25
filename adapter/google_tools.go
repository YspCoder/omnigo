package adapter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
)

func googleApplyTools(cfg *genai.GenerateContentConfig, r *dto.MediaRequest) error {
	var declarations []*genai.FunctionDeclaration
	for _, tool := range r.Tools {
		if tool.Type != "" && tool.Type != "function" || strings.TrimSpace(tool.Function.Name) == "" {
			return fmt.Errorf("google tools require a named function")
		}
		declarations = append(declarations, &genai.FunctionDeclaration{Name: tool.Function.Name, Description: tool.Function.Description, ParametersJsonSchema: tool.Function.Parameters})
	}
	if len(declarations) > 0 {
		cfg.Tools = []*genai.Tool{{FunctionDeclarations: declarations}}
	}
	if r.ToolChoice == nil {
		return nil
	}
	choice := &genai.FunctionCallingConfig{}
	if mode, ok := openAIToolChoiceMode(r.ToolChoice); ok {
		switch mode {
		case "auto":
			choice.Mode = genai.FunctionCallingConfigModeAuto
		case "none":
			choice.Mode = genai.FunctionCallingConfigModeNone
		case "required":
			choice.Mode = genai.FunctionCallingConfigModeAny
		}
	} else if named, ok := openAIResponsesToolChoice(r.ToolChoice).(map[string]interface{}); ok {
		if len(named) == 0 {
			return nil
		}
		name, _ := named["name"].(string)
		if named["type"] != "function" || strings.TrimSpace(name) == "" {
			return fmt.Errorf("google function tool choice requires a name")
		}
		choice.Mode, choice.AllowedFunctionNames = genai.FunctionCallingConfigModeAny, []string{name}
	} else {
		return fmt.Errorf("unsupported google tool choice")
	}
	cfg.ToolConfig = &genai.ToolConfig{FunctionCallingConfig: choice}
	return nil
}

func googleFunctionParts(m dto.Message, names map[string]string) ([]*genai.Part, error) {
	var parts []*genai.Part
	for _, call := range m.ToolCalls {
		args := map[string]interface{}{}
		if len(call.Function.Arguments) > 0 {
			if err := json.Unmarshal(call.Function.Arguments, &args); err != nil || args == nil {
				return nil, fmt.Errorf("google function %q requires JSON object arguments", call.Function.Name)
			}
		}
		part := &genai.Part{FunctionCall: &genai.FunctionCall{ID: call.ID, Name: call.Function.Name, Args: args}}
		if signature, ok := call.Metadata["thought_signature"].(string); ok {
			var err error
			part.ThoughtSignature, err = base64.StdEncoding.DecodeString(signature)
			if err != nil {
				return nil, fmt.Errorf("decode google thought signature: %w", err)
			}
		}
		parts = append(parts, part)
		names[call.ID] = call.Function.Name
	}
	if m.Role == "tool" {
		name := firstNonEmptyString(m.Name, names[m.ToolCallID])
		if name == "" {
			return nil, fmt.Errorf("google tool result requires a name or matching tool_call_id")
		}
		var output interface{} = m.Content
		if text, ok := output.(string); ok {
			var decoded interface{}
			if json.Unmarshal([]byte(text), &decoded) == nil {
				output = decoded
			}
		}
		result, ok := output.(map[string]interface{})
		if !ok {
			result = map[string]interface{}{"output": output}
		}
		parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{ID: m.ToolCallID, Name: name, Response: result}})
	}
	return parts, nil
}

func googleToolCall(part *genai.Part, candidate, index int) dto.ToolCall {
	call := part.FunctionCall
	args, _ := json.Marshal(call.Args)
	if call.Args == nil {
		args = []byte("{}")
	}
	result := dto.ToolCall{ID: call.ID, Type: "function", Function: dto.ToolCallFunction{Name: call.Name, Arguments: args}}
	if result.ID == "" {
		result.ID = fmt.Sprintf("google_call_%d_%d", candidate, index)
	}
	if len(part.ThoughtSignature) > 0 {
		result.Metadata = map[string]interface{}{"thought_signature": base64.StdEncoding.EncodeToString(part.ThoughtSignature)}
	}
	return result
}
