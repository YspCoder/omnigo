package utils

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

func GetStringExtra(extra map[string]interface{}, key string) string {
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

func GetBoolExtra(extra map[string]interface{}, key string) (bool, bool) {
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

func GetIntExtra(extra map[string]interface{}, key string) (int, bool) {
	if extra == nil {
		return 0, false
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return 0, false
	}

	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func GetStringSliceExtra(extra map[string]interface{}, key string) []string {
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

func ExtractPayloadMap(extra map[string]interface{}) map[string]interface{} {
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

func FirstSystemMessage(messages []dto.Message) string {
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

func NonSystemMessages(messages []dto.Message) []dto.Message {
	result := make([]dto.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			continue
		}
		result = append(result, message)
	}
	return result
}

func MediaPromptWithSystem(request *dto.MediaRequest) string {
	if request == nil {
		return ""
	}

	prompt := FirstUserMessage(request.Messages)
	systemPrompt := FirstSystemMessage(request.Messages)
	if systemPrompt == "" {
		return prompt
	}
	if prompt == "" {
		return systemPrompt
	}
	return systemPrompt + "\n\n" + prompt
}

func FirstUserMessage(messages []dto.Message) string {
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

func ContentImageURLs(v interface{}) []string {
	switch arr := v.(type) {
	case []string:
		var out []string
		for _, item := range arr {
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []map[string]interface{}:
		var out []string
		for _, item := range arr {
			if url, ok := ContentImageURL(item); ok {
				out = append(out, url)
			}
		}
		return out
	case []map[string]string:
		var out []string
		for _, item := range arr {
			if url, ok := ContentImageURL(item); ok {
				out = append(out, url)
			}
		}
		return out
	case []interface{}:
		var out []string
		for _, item := range arr {
			if url, ok := ContentImageURL(item); ok {
				out = append(out, url)
			}
		}
		return out
	default:
		if url, ok := ContentImageURL(v); ok {
			return []string{url}
		}
	}
	return nil
}

func ContentImageURL(v interface{}) (string, bool) {
	switch item := v.(type) {
	case string:
		if item == "" {
			return "", false
		}
		return item, true
	case map[string]interface{}:
		if url, ok := item["url"].(string); ok && url != "" {
			return url, true
		}
		if imageURL, ok := item["image_url"].(map[string]interface{}); ok {
			if url, ok := imageURL["url"].(string); ok && url != "" {
				return url, true
			}
		}
	case map[string]string:
		if url, ok := item["url"]; ok && url != "" {
			return url, true
		}
	}
	return "", false
}
