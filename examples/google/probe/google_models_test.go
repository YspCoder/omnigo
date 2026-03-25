package probe

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
)

func TestGoogleModelsLive(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		t.Fatalf("create genai client: %v", err)
	}

	models, err := googleProbeModels(ctx, client)
	if err != nil {
		t.Fatalf("discover models: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("no models configured")
	}

	for _, model := range models {
		model := model
		t.Run(model, func(t *testing.T) {
			llm, err := omnigo.NewLLM(
				omnigo.SetProvider("google"),
				omnigo.SetModel(model),
				omnigo.SetAPIKey(apiKey),
			)
			if err != nil {
				t.Fatalf("create llm: %v", err)
			}

			reqCtx, reqCancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer reqCancel()

			switch googleProbeMode(model) {
			case "image":
				req := &dto.MediaRequest{
					Type:  dto.MediaTypeImage,
					Model: model,
					Messages: []dto.Message{{
						Role:    "user",
						Content: "A clean product-style illustration of a small robot holding a paper note, soft lighting",
					}},
					N:    1,
					Size: "1:1",
				}

				resp, err := llm.Media(reqCtx, req)
				if err != nil {
					t.Fatalf("media request failed: %v", err)
				}
				paths, err := googleSaveImages(resp, model)
				if err != nil {
					t.Fatalf("save images failed: %v", err)
				}
				t.Logf("status=%q task_id=%q images=%d text=%q url=%q files=%v", resp.Status, resp.TaskID, len(resp.Data), truncate(resp.Text, 80), truncate(resp.URL, 80), paths)
				if len(resp.Data) == 0 && resp.URL == "" && resp.TaskID == "" {
					t.Fatalf("expected image output, task id, or url, got empty response: %#v", resp)
				}
			case "video":
				duration := googleVideoDuration(model)
				req := &dto.MediaRequest{
					Type:  dto.MediaTypeVideo,
					Model: model,
					Messages: []dto.Message{{
						Role:    "user",
						Content: "A cinematic drone shot flying over a quiet lake at sunrise, soft mist, realistic style",
					}},
					Size:     "16:9",
					Duration: duration,
				}

				resp, err := llm.Media(reqCtx, req)
				if err != nil {
					t.Fatalf("video request failed: %v", err)
				}
				t.Logf("status=%q task_id=%q duration=%d video_url=%q", resp.Status, resp.TaskID, duration, truncate(resp.Video.URL, 80))
				if resp.Video.URL != "" || resp.URL != "" {
					return
				}
				if resp.TaskID == "" {
					t.Fatalf("expected task id or video url, got empty response: %#v", resp)
				}
				status, err := googleWaitForVideoTask(llm, resp.TaskID, googleVideoTimeout())
				if err != nil {
					t.Fatalf("task status failed: %v", err)
				}
				t.Logf("task_status=%q video_url=%q code=%q message=%q", status.Output.TaskStatus, truncate(status.Output.VideoURL, 80), status.Output.Code, truncate(status.Output.Message, 120))
			default:
				resp, err := llm.Generate(reqCtx, omnigo.NewPrompt("Reply with exactly: ok"))
				if err != nil {
					t.Fatalf("generate request failed: %v", err)
				}
				t.Logf("response=%q", truncate(resp, 120))
				if strings.TrimSpace(resp) == "" {
					t.Fatal("expected non-empty text response")
				}
			}
		})
	}
}

func TestGoogleListModelsLive(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		t.Fatalf("create genai client: %v", err)
	}

	page, err := client.Models.List(ctx, &genai.ListModelsConfig{
		PageSize: 100,
	})
	if err != nil {
		t.Fatalf("list models failed: %v", err)
	}

	filter := strings.ToLower(strings.TrimSpace(os.Getenv("GOOGLE_MODEL_FILTER")))
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		if item == nil {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(name), filter) {
			continue
		}
		names = append(names, name)
	}

	slices.Sort(names)
	t.Logf("listed=%d next_page_token=%q filter=%q", len(names), page.NextPageToken, filter)
	for _, name := range names {
		t.Log(name)
	}

	if len(names) == 0 {
		if filter == "" {
			t.Fatal("list models returned no model names")
		}
		t.Fatalf("no model names matched filter %q", filter)
	}
}

func googleProbeModels(ctx context.Context, client *genai.Client) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv("GOOGLE_MODELS"))
	if raw == "" {
		return googleDiscoverModels(ctx, client)
	}

	parts := strings.Split(raw, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model != "" {
			models = append(models, model)
		}
	}
	return models, nil
}

func googleDiscoverModels(ctx context.Context, client *genai.Client) ([]string, error) {
	filter := strings.ToLower(strings.TrimSpace(os.Getenv("GOOGLE_MODEL_FILTER")))
	models := make([]string, 0)
	for item, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, err
		}
		if item == nil {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(item.Name), "models/")
		if name == "" {
			continue
		}
		if !googleShouldProbeModel(name, item.SupportedActions) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(name), filter) {
			continue
		}
		models = append(models, name)
	}
	slices.Sort(models)
	return slices.Compact(models), nil
}

func googleShouldProbeModel(name string, actions []string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "veo") || strings.Contains(lower, "video") {
		return true
	}
	if strings.HasPrefix(lower, "imagen-") {
		return true
	}
	if strings.Contains(lower, "image") {
		return true
	}
	for _, action := range actions {
		action = strings.ToLower(strings.TrimSpace(action))
		if action == "generatecontent" && strings.Contains(lower, "gemini") {
			return true
		}
	}
	return false
}

func googleProbeMode(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(name, "veo") || strings.Contains(name, "video") {
		return "video"
	}
	if strings.Contains(name, "image") || strings.HasPrefix(name, "imagen-") {
		return "image"
	}
	return "text"
}

func googleVideoDuration(model string) int {
	name := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(name, "veo-3.") {
		return 8
	}
	return 5
}

func googleWaitForVideoTask(llm omnigo.LLM, taskID string, timeout time.Duration) (*dto.TaskStatusResponse, error) {
	deadline := time.Now().Add(timeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		status, err := llm.TaskStatus(ctx, taskID)
		cancel()
		if err != nil {
			return nil, err
		}
		switch strings.ToUpper(strings.TrimSpace(status.Output.TaskStatus)) {
		case "SUCCEEDED":
			if strings.TrimSpace(status.Output.VideoURL) == "" {
				return nil, fmt.Errorf("task %s succeeded but returned empty video url", taskID)
			}
			return status, nil
		case "FAILED":
			return nil, fmt.Errorf("task %s failed: code=%s message=%s", taskID, status.Output.Code, status.Output.Message)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("task %s timed out after %s; last status=%s message=%s", taskID, timeout, status.Output.TaskStatus, status.Output.Message)
		}
		time.Sleep(5 * time.Second)
	}
}

func googleVideoTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("GOOGLE_VIDEO_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 {
			return seconds
		}
	}
	return 10 * time.Minute
}

func googleSaveImages(resp *dto.MediaResponse, model string) ([]string, error) {
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}

	dir := strings.TrimSpace(os.Getenv("GOOGLE_OUTPUT_DIR"))
	if dir == "" {
		dir = filepath.Join("tmp", "google_probe")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	prefix := sanitizeFileName(model)
	saved := make([]string, 0, len(resp.Data))
	for i, item := range resp.Data {
		if strings.TrimSpace(item.B64JSON) == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return saved, err
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%d.png", prefix, i+1))
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return saved, err
		}
		saved = append(saved, path)
	}
	return saved, nil
}

func sanitizeFileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "image"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", ".", "-")
	return replacer.Replace(value)
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return fmt.Sprintf("%s...", value[:max])
}
