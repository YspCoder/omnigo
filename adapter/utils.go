package adapter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

func getStringExtra(extra map[string]interface{}, key string) string {
	if extra == nil {
		return ""
	}
	if value, ok := extra[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func getBoolExtra(extra map[string]interface{}, key string) (bool, bool) {
	if extra == nil {
		return false, false
	}
	value, ok := extra[key]
	if !ok {
		return false, false
	}
	typed, ok := value.(bool)
	return typed, ok
}

func getStringSliceExtra(extra map[string]interface{}, key string) []string {
	if extra == nil {
		return nil
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return nil
	}

	switch typed := value.(type) {
	case []string:
		if len(typed) == 0 {
			return nil
		}
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok || s == "" {
				continue
			}
			result = append(result, s)
		}
		if len(result) == 0 {
			return nil
		}
		return result
	default:
		return nil
	}
}

func extractPayloadMap(extra map[string]interface{}) map[string]interface{} {
	if extra == nil {
		return nil
	}
	raw, ok := extra["payload"]
	if !ok {
		return nil
	}
	payload, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return payload
}

func marshalPayloadWithFallback(payload map[string]interface{}, fallback interface{}) ([]byte, error) {
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			return b, nil
		}
	}
	return json.Marshal(fallback)
}

func hashSHA256(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func hmacSign(key []byte, content string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(content))
	return mac.Sum(nil)
}

func firstSystemMessage(messages []dto.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.Role != "system" {
			continue
		}
		if text := strings.TrimSpace(fmt.Sprint(message.Content)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func nonSystemMessages(messages []dto.Message) []dto.Message {
	result := make([]dto.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			continue
		}
		result = append(result, message)
	}
	return result
}

func mediaPromptWithSystem(request *dto.MediaRequest) string {
	if request == nil {
		return ""
	}

	prompt := firstUserMessage(request.Messages)
	systemPrompt := firstSystemMessage(request.Messages)
	if systemPrompt == "" {
		return prompt
	}
	if prompt == "" {
		return systemPrompt
	}
	return systemPrompt + "\n\n" + prompt
}

func firstUserMessage(messages []dto.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		if text := strings.TrimSpace(fmt.Sprint(message.Content)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}
