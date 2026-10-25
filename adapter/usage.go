package adapter

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"

	"github.com/YspCoder/omnigo/dto"
)

// usageFromJSON normalizes common token aliases while retaining the source
// payload. Provider-specific accounting (for example Claude caches) stays in
// the adaptor so included subtotals are never counted twice.
func usageFromJSON(raw json.RawMessage) dto.Usage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
		return dto.Usage{}
	}
	var details dto.UsageDetails
	_ = json.Unmarshal(raw, &details)
	details.CaptureRawUsage(raw)
	u := dto.Usage{UsageDetails: details}
	u.PromptTokens, _ = usageInt(fields, "prompt_tokens", "input_tokens", "promptTokenCount")
	u.CompletionTokens, _ = usageInt(fields, "completion_tokens", "output_tokens", "candidatesTokenCount")
	var totalReported bool
	u.TotalTokens, totalReported = usageInt(fields, "total_tokens", "totalTokenCount")
	if !totalReported {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	u.PromptTokensDetails = usageObject(fields, "prompt_tokens_details", "input_tokens_details")
	u.CompletionTokensDetails = usageObject(fields, "completion_tokens_details", "output_tokens_details")
	for _, mapping := range []struct{ source, target string }{
		{"cache_read_input_tokens", "cached_tokens"}, {"cachedContentTokenCount", "cached_tokens"},
		{"cache_creation_input_tokens", "cache_creation_tokens"}, {"cache_creation", "cache_creation"},
		{"promptTokensDetails", "modalities"}, {"cacheTokensDetails", "cache_modalities"},
		{"toolUsePromptTokenCount", "tool_tokens"}, {"toolUsePromptTokensDetails", "tool_modalities"},
	} {
		copyUsageField(&u.PromptTokensDetails, fields, mapping.source, mapping.target)
	}
	copyUsageField(&u.CompletionTokensDetails, fields, "thoughtsTokenCount", "reasoning_tokens")
	copyUsageField(&u.CompletionTokensDetails, fields, "candidatesTokensDetails", "modalities")
	for _, key := range []string{"credits", "final_unit_deduction", "final_balance_deduction",
		"duration", "video_duration", "input_video_duration", "output_video_duration",
		"video_count", "image_count", "generated_images", "SR", "ratio", "size", "characters"} {
		copyUsageField(&u.Billing, fields, key, key)
	}
	return u
}

func usageFromValue(value any) dto.Usage {
	raw, err := json.Marshal(value)
	if err != nil {
		return dto.Usage{}
	}
	// A non-nil SDK usage struct can encode to {} when every count is zero.
	if string(raw) == "{}" {
		return dto.Usage{UsageDetails: dto.UsageDetails{RawUsage: raw}}
	}
	return usageFromJSON(raw)
}

func creditsUsage(credits json.RawMessage) dto.Usage {
	if len(credits) == 0 || bytes.Equal(bytes.TrimSpace(credits), []byte("null")) {
		return dto.Usage{}
	}
	return usageFromValue(map[string]json.RawMessage{"credits": credits})
}

func usageInt(fields map[string]json.RawMessage, keys ...string) (int, bool) {
	for _, key := range keys {
		var number json.Number
		if raw := fields[key]; len(raw) > 0 && string(raw) != "null" && json.Unmarshal(raw, &number) == nil {
			if value, err := number.Int64(); err == nil {
				return int(value), true
			}
		}
	}
	return 0, false
}

func usageObject(fields map[string]json.RawMessage, keys ...string) map[string]json.RawMessage {
	for _, key := range keys {
		var value map[string]json.RawMessage
		if json.Unmarshal(fields[key], &value) == nil && len(value) > 0 {
			return value
		}
	}
	return nil
}

func copyUsageField(dst *map[string]json.RawMessage, src map[string]json.RawMessage, from, to string) {
	if raw := src[from]; len(raw) > 0 && string(raw) != "null" {
		if *dst == nil {
			*dst = make(map[string]json.RawMessage)
		}
		(*dst)[to] = raw
	}
}

func usagePointer(usage dto.Usage) *dto.Usage {
	if !usage.HasUsage() {
		return nil
	}
	return &usage
}

func taskUsage(usage dto.Usage) *dto.TaskStatusUsage {
	if !usage.HasUsage() {
		return nil
	}
	duration := 0
	for _, key := range []string{"output_video_duration", "duration", "video_duration"} {
		var value *float64
		if json.Unmarshal(usage.Billing[key], &value) == nil && value != nil {
			duration = int(*value) // Legacy integer field; Billing retains fractional seconds.
			break
		}
	}
	count, _ := usageInt(usage.Billing, "video_count")
	sr, _ := usageInt(usage.Billing, "SR")
	return &dto.TaskStatusUsage{
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
		VideoDuration: duration, VideoCount: count, SR: sr, UsageDetails: usage.UsageDetails,
	}
}

func streamUsage(usage *dto.TaskStatusUsage) *dto.Usage {
	if usage == nil {
		return nil
	}
	converted := dto.Usage{
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		TotalTokens: usage.TotalTokens, UsageDetails: usage.UsageDetails,
	}
	converted.Billing = maps.Clone(usage.Billing)
	for key, value := range map[string]int{"video_duration": usage.VideoDuration, "video_count": usage.VideoCount, "SR": usage.SR} {
		if value != 0 && len(converted.Billing[key]) == 0 {
			if converted.Billing == nil {
				converted.Billing = make(map[string]json.RawMessage)
			}
			converted.Billing[key], _ = json.Marshal(value)
		}
	}
	return usagePointer(converted)
}

// updateStreamUsage retains the last reported snapshot when a later poll omits it.
func updateStreamUsage(latest **dto.Usage, current *dto.Usage) bool {
	if current == nil || !current.HasUsage() {
		return false
	}
	changed := !reflect.DeepEqual(*latest, current)
	*latest = current
	return changed
}
