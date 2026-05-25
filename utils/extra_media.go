package utils

import "strings"

// ParseExtraImageInputs extracts image inputs from extra.image, extra.images, and extra.files.
func ParseExtraImageInputs(extra map[string]interface{}) []string {
	if extra == nil {
		return nil
	}

	out := make([]string, 0, 6)
	out = append(out, parseImageLike(extra["image"])...)
	out = append(out, parseImageLike(extra["images"])...)
	out = append(out, parseImageFiles(extra["files"])...)
	return compactStrings(out)
}

func parseImageFiles(v interface{}) []string {
	switch items := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if file, ok := item.(map[string]interface{}); ok {
				if strings.ToLower(strings.TrimSpace(stringFromAny(file["type"]))) != "image" {
					continue
				}
				if url := strings.TrimSpace(stringFromAny(file["url"])); url != "" {
					out = append(out, url)
				}
			}
		}
		return out
	case []map[string]interface{}:
		out := make([]string, 0, len(items))
		for _, file := range items {
			if strings.ToLower(strings.TrimSpace(stringFromAny(file["type"]))) != "image" {
				continue
			}
			if url := strings.TrimSpace(stringFromAny(file["url"])); url != "" {
				out = append(out, url)
			}
		}
		return out
	default:
		return nil
	}
}

func parseImageLike(v interface{}) []string {
	switch typed := v.(type) {
	case string:
		if s := strings.TrimSpace(typed); s != "" {
			return []string{s}
		}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(stringFromAny(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case map[string]interface{}:
		if s := strings.TrimSpace(stringFromAny(typed["url"])); s != "" {
			return []string{s}
		}
		if imageURL, ok := typed["image_url"].(map[string]interface{}); ok {
			if s := strings.TrimSpace(stringFromAny(imageURL["url"])); s != "" {
				return []string{s}
			}
		}
	}
	return nil
}

func stringFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
