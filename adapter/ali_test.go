package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestAliChatUsesCompatibleModeBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatible-mode/v1/chat/completions" {
			t.Fatalf("path = %q, want /compatible-mode/v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat-1","created":1,"model":"qwen-plus","choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeText,
		Model: "qwen-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat error = %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("text = %q, want hello", resp.Text)
	}
}

func TestAliMediaSyncImageMapsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != aliImageSyncEndpoint {
			t.Fatalf("path = %q, want %q", r.URL.Path, aliImageSyncEndpoint)
		}
		if got := r.Header.Get("X-DashScope-Async"); got != "" {
			t.Fatalf("X-DashScope-Async = %q, want empty", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "qwen-image-max" {
			t.Fatalf("model = %#v, want qwen-image-max", body["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-img","output":{"choices":[{"message":{"content":[{"image":"https://cdn.example.com/image.png"}]}}]}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "qwen-image-max",
		Messages: []dto.Message{
			{Role: "user", Content: "一只猫"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.URL != "https://cdn.example.com/image.png" {
		t.Fatalf("url = %q, want parsed image url", resp.URL)
	}
}

func TestAliMediaVideoUsesAsyncTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != aliVideoEndpoint {
			t.Fatalf("path = %q, want %q", r.URL.Path, aliVideoEndpoint)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q, want Bearer secret", got)
		}
		if got := r.Header.Get("X-DashScope-Async"); got != "enable" {
			t.Fatalf("X-DashScope-Async = %q, want enable", got)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		input := body["input"].(map[string]interface{})
		if _, ok := input["first_frame_url"]; ok {
			t.Fatalf("first_frame_url = %#v, want media protocol only", input["first_frame_url"])
		}
		media, ok := input["media"].([]interface{})
		if !ok || len(media) != 2 {
			t.Fatalf("media = %#v, want first/last frame media", input["media"])
		}
		first, _ := media[0].(map[string]interface{})
		last, _ := media[1].(map[string]interface{})
		if first["type"] != "first_frame" || first["url"] != "https://example.com/start.png" {
			t.Fatalf("media[0] = %#v, want first frame", first)
		}
		if last["type"] != "last_frame" || last["url"] != "https://example.com/end.png" {
			t.Fatalf("media[1] = %#v, want last frame", last)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-vid","output":{"task_id":"task-123","task_status":"PENDING"}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-kf2v-flash",
		Messages: []dto.Message{
			{Role: "user", Content: "镜头推进", ImageURL: "https://example.com/start.png"},
			{Role: "user", ImageURL: "https://example.com/end.png"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.TaskID != "task-123" {
		t.Fatalf("task id = %q, want task-123", resp.TaskID)
	}
	if resp.Status != "PENDING" {
		t.Fatalf("status = %q, want PENDING", resp.Status)
	}
}

func TestAliBuildVideoRequestUsesModelToInferReferenceMode(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-r2v-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "保持主体一致", ImageURL: "https://example.com/ref-1.png"},
			{Role: "user", ImageURL: "https://example.com/ref-2.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["reference_urls"]; ok {
		t.Fatalf("reference_urls = %#v, want media protocol only", input["reference_urls"])
	}
	if _, ok := input["first_frame_url"]; ok {
		t.Fatalf("first_frame_url = %#v, want omitted for reference mode", input["first_frame_url"])
	}
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 2 {
		t.Fatalf("media = %#v, want reference image media", input["media"])
	}
	for i, item := range media {
		if item["type"] != "reference_image" {
			t.Fatalf("media[%d] = %#v, want reference_image", i, item)
		}
	}
}

func TestAliBuildWan27VideoRequestUsesMediaAndRatio(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "wan2.7-r2v",
		Messages: []dto.Message{{Role: "user", Content: "保持主体一致"}},
		Size:     "16:9",
		Extra: map[string]interface{}{
			"image":           "https://example.com/ref.png",
			"video":           "https://example.com/source.mp4",
			"reference_voice": "https://example.com/voice.mp3",
			"prompt_extend":   false,
			"watermark":       true,
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 2 {
		t.Fatalf("media = %#v, want two media items", input["media"])
	}
	if media[0]["type"] != "reference_image" || media[0]["url"] != "https://example.com/ref.png" {
		t.Fatalf("media[0] = %#v, want reference image", media[0])
	}
	if media[1]["type"] != "reference_video" || media[1]["url"] != "https://example.com/source.mp4" {
		t.Fatalf("media[1] = %#v, want reference video", media[1])
	}
	if input["reference_voice"] != "https://example.com/voice.mp3" {
		t.Fatalf("reference_voice = %#v, want input reference_voice", input["reference_voice"])
	}
	params := payload["parameters"].(map[string]interface{})
	if params["ratio"] != "16:9" {
		t.Fatalf("ratio = %#v, want 16:9", params["ratio"])
	}
	if _, ok := params["size"]; ok {
		t.Fatalf("size = %#v, want omitted", params["size"])
	}
	if params["prompt_extend"] != false || params["watermark"] != true {
		t.Fatalf("parameters = %#v, want prompt_extend false and watermark true", params)
	}
}

func TestAliBuildWan27ReferenceVideoUsesImageAndVideoInputs(t *testing.T) {
	adaptor := &AliAdaptor{}
	_, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:       dto.MediaTypeVideo,
		Model:      "wan2.7-r2v",
		Messages:   []dto.Message{{Role: "user", Content: "参考角色生成视频"}},
		Size:       "16:9",
		Resolution: "720P",
		Duration:   10,
		Extra: map[string]interface{}{
			"images": []interface{}{
				"https://example.com/girl.jpg",
			},
			"videos": []interface{}{
				"https://example.com/boy.mp4",
			},
			"prompt_extend": false,
			"watermark":     true,
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	input := payload["input"].(map[string]interface{})
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 2 {
		t.Fatalf("media = %#v, want image/video media", input["media"])
	}
	if media[0]["type"] != "reference_image" || media[0]["url"] != "https://example.com/girl.jpg" {
		t.Fatalf("media[0] = %#v, want reference image", media[0])
	}
	if media[1]["type"] != "reference_video" || media[1]["url"] != "https://example.com/boy.mp4" {
		t.Fatalf("media[1] = %#v, want reference video", media[1])
	}
	params := payload["parameters"].(map[string]interface{})
	if params["resolution"] != "720P" || params["ratio"] != "16:9" || params["duration"] != 10 || params["prompt_extend"] != false || params["watermark"] != true {
		t.Fatalf("parameters = %#v, want wan2.7-r2v parameters", params)
	}
}

func TestAliBuildVideoRequestKeepsModelAndPassesParameters(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "wan2.5-t2v-preview",
		Messages: []dto.Message{{Role: "user", Content: "城市夜景延时摄影"}},
		Duration: 5,
		Extra: map[string]interface{}{
			"prompt_extend": true,
			"watermark":     false,
			"shot_type":     "dolly",
			"audio":         true,
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	if payload["model"] != "wan2.5-t2v-preview" {
		t.Fatalf("model = %#v, want request model", payload["model"])
	}
	if _, ok := payload["watermark"]; ok {
		t.Fatalf("top-level watermark = %#v, want omitted", payload["watermark"])
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["audio_url"]; ok {
		t.Fatalf("audio_url = %#v, want omitted when not provided", input["audio_url"])
	}
	params := payload["parameters"].(map[string]interface{})
	if params["duration"] != 5 || params["prompt_extend"] != true || params["watermark"] != false || params["shot_type"] != "dolly" || params["audio"] != true {
		t.Fatalf("parameters = %#v, want video options passed through", params)
	}
}

func TestAliBuildWanTextVideoPassesAudioURLInInput(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "wan2.5-t2v-preview",
		Messages: []dto.Message{{Role: "user", Content: "城市夜景延时摄影"}},
		Extra: map[string]interface{}{
			"audio_url": "https://example.com/music.mp3",
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if input["audio_url"] != "https://example.com/music.mp3" {
		t.Fatalf("audio_url = %#v, want input audio_url", input["audio_url"])
	}
	if _, ok := input["media"]; ok {
		t.Fatalf("media = %#v, want omitted for t2v", input["media"])
	}
}

func TestAliBuildVideoRequestDoesNotDefaultModel(t *testing.T) {
	adaptor := &AliAdaptor{}
	_, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Messages: []dto.Message{{Role: "user", Content: "城市夜景延时摄影"}},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if payload["model"] != "" {
		t.Fatalf("model = %#v, want empty request model preserved", payload["model"])
	}
}

func TestAliBuildWan27ImageVideoUsesVideoEndpointAndMedia(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.7-i2v",
		Messages: []dto.Message{
			{Role: "user", Content: "镜头推进", ImageURL: "https://example.com/start.png"},
			{Role: "user", ImageURL: "https://example.com/end.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["first_frame_url"]; ok {
		t.Fatalf("first_frame_url = %#v, want media protocol only", input["first_frame_url"])
	}
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 2 {
		t.Fatalf("media = %#v, want first/last frame media", input["media"])
	}
	if media[0]["type"] != "first_frame" || media[1]["type"] != "last_frame" {
		t.Fatalf("media = %#v, want first_frame and last_frame", media)
	}
}

func TestAliBuildWan27ImageVideoConvertsImagesToMedia(t *testing.T) {
	adaptor := &AliAdaptor{}
	_, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.7-i2v",
		Messages: []dto.Message{
			{Role: "user", Content: "镜头推进"},
		},
		Extra: map[string]interface{}{
			"images": []interface{}{
				"https://example.com/start.png",
				"https://example.com/end.png",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["first_frame_url"]; ok {
		t.Fatalf("first_frame_url = %#v, want omitted for media protocol", input["first_frame_url"])
	}
	if _, ok := input["last_frame_url"]; ok {
		t.Fatalf("last_frame_url = %#v, want omitted for media protocol", input["last_frame_url"])
	}
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 2 {
		t.Fatalf("media = %#v, want frame media", input["media"])
	}
	if media[0]["type"] != "first_frame" || media[1]["type"] != "last_frame" {
		t.Fatalf("media = %#v, want first_frame and last_frame", media)
	}
}

func TestAliBuildHappyHorseImageVideoUsesFirstFrameMedia(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "happyhorse-1.0-i2v",
		Messages: []dto.Message{{Role: "user", Content: "镜头推进"}},
		Extra: map[string]interface{}{
			"image": "https://example.com/start.png",
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["first_frame_url"]; ok {
		t.Fatalf("first_frame_url = %#v, want media protocol only", input["first_frame_url"])
	}
	if _, ok := input["img_url"]; ok {
		t.Fatalf("img_url = %#v, want media protocol only", input["img_url"])
	}
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 1 {
		t.Fatalf("media = %#v, want one media item", input["media"])
	}
	if media[0]["type"] != "first_frame" || media[0]["url"] != "https://example.com/start.png" {
		t.Fatalf("media[0] = %#v, want first_frame media", media[0])
	}
}

func TestAliBuildHappyHorseImageVideoKeepsOnlyOneFirstFrame(t *testing.T) {
	adaptor := &AliAdaptor{}
	_, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "happyhorse-1.0-i2v",
		Messages: []dto.Message{{Role: "user", Content: "镜头推进"}},
		Extra: map[string]interface{}{
			"images": []interface{}{
				"https://example.com/start.png",
				"https://example.com/end.png",
			},
			"videos": []interface{}{
				"https://example.com/source.mp4",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	input := payload["input"].(map[string]interface{})
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 1 {
		t.Fatalf("media = %#v, want one first_frame item", input["media"])
	}
	if media[0]["type"] != "first_frame" || media[0]["url"] != "https://example.com/start.png" {
		t.Fatalf("media[0] = %#v, want only first_frame image", media[0])
	}
}

func TestAliBuildHappyHorseReferenceVideoUsesReferenceImageMedia(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "happyhorse-1.0-r2v",
		Messages: []dto.Message{{Role: "user", Content: "保持主体一致"}},
		Extra: map[string]interface{}{
			"images": []interface{}{
				"https://example.com/girl.jpg",
				"https://example.com/fan.jpg",
				"https://example.com/earrings.jpg",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["reference_urls"]; ok {
		t.Fatalf("reference_urls = %#v, want media protocol only", input["reference_urls"])
	}
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 3 {
		t.Fatalf("media = %#v, want three reference images", input["media"])
	}
	for i, item := range media {
		if item["type"] != "reference_image" {
			t.Fatalf("media[%d] = %#v, want reference_image", i, item)
		}
	}
}

func TestAliBuildHappyHorseReferenceVideoSkipsNonImageMedia(t *testing.T) {
	adaptor := &AliAdaptor{}
	_, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "happyhorse-1.0-r2v",
		Messages: []dto.Message{{Role: "user", Content: "保持主体一致"}},
		Extra: map[string]interface{}{
			"image": "https://example.com/ref.jpg",
			"videos": []interface{}{
				"https://example.com/ref.mp4",
				"https://example.com/source.mp4",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	input := payload["input"].(map[string]interface{})
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 1 {
		t.Fatalf("media = %#v, want one reference_image item", input["media"])
	}
	if media[0]["type"] != "reference_image" || media[0]["url"] != "https://example.com/ref.jpg" {
		t.Fatalf("media[0] = %#v, want only reference_image", media[0])
	}
}

func TestAliBuildAnimateMixVideoRequest(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-animate-mix",
		Messages: []dto.Message{
			{Role: "user", ImageURL: "https://example.com/person.png", VideoURL: "https://example.com/source.mp4"},
		},
		Extra: map[string]interface{}{
			"mode":      "wan-pro",
			"watermark": true,
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliImageToVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliImageToVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if input["image_url"] != "https://example.com/person.png" || input["video_url"] != "https://example.com/source.mp4" || input["watermark"] != true {
		t.Fatalf("input = %#v, want animate mix image/video/watermark", input)
	}
	params := payload["parameters"].(map[string]interface{})
	if params["mode"] != "wan-pro" {
		t.Fatalf("mode = %#v, want wan-pro", params["mode"])
	}
}

func TestAliBuildAvatarVideoRequest(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-s2v",
		Messages: []dto.Message{
			{Role: "user", ImageURL: "https://example.com/avatar.png"},
		},
		Extra: map[string]interface{}{
			"audio_url":  "https://example.com/audio.wav",
			"resolution": "720P",
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliImageToVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliImageToVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if input["image_url"] != "https://example.com/avatar.png" || input["audio_url"] != "https://example.com/audio.wav" {
		t.Fatalf("input = %#v, want avatar image/audio", input)
	}
	params := payload["parameters"].(map[string]interface{})
	if params["resolution"] != "720P" {
		t.Fatalf("resolution = %#v, want 720P", params["resolution"])
	}
}

func TestAliBuildVideoRequestUsesModelToInferImageMode(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-i2v-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "镜头推进", ImageURL: "https://example.com/input.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["first_frame_url"]; ok {
		t.Fatalf("first_frame_url = %#v, want media protocol only", input["first_frame_url"])
	}
	media, ok := input["media"].([]map[string]interface{})
	if !ok || len(media) != 1 {
		t.Fatalf("media = %#v, want first frame media", input["media"])
	}
	if media[0]["type"] != "first_frame" || media[0]["url"] != "https://example.com/input.png" {
		t.Fatalf("media[0] = %#v, want first_frame input image", media[0])
	}
}

func TestAliBuildImageRequestUsesModelToInferAsyncImageGeneration(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, _, async, err := adaptor.buildImageRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "wan2.2-t2i-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "一只猫"},
		},
	})
	if err != nil {
		t.Fatalf("buildImageRequest error = %v", err)
	}
	if endpoint != aliImageNewAsyncEndpoint && endpoint != aliImageAsyncEndpoint {
		t.Fatalf("endpoint = %q, want an async image endpoint", endpoint)
	}
	if !async {
		t.Fatalf("async = false, want true")
	}
}

func TestAliResolveVideoModePrefersModelRouteTable(t *testing.T) {
	mode := aliResolveVideoMode(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wanx2.1-kf2v-plus",
	})
	if mode != aliVideoModeKeyframe {
		t.Fatalf("mode = %q, want %q", mode, aliVideoModeKeyframe)
	}
}

func TestAliTaskStatusMapsResultURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/task-123" {
			t.Fatalf("path = %q, want /api/v1/tasks/task-123", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-task","output":{"task_id":"task-123","task_status":"SUCCEEDED","results":[{"video_url":"https://cdn.example.com/video.mp4"}]}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, "task-123")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskStatus != "SUCCEEDED" {
		t.Fatalf("task status = %q, want SUCCEEDED", resp.Output.TaskStatus)
	}
	if resp.Output.VideoURL != "https://cdn.example.com/video.mp4" {
		t.Fatalf("video url = %q, want parsed url", resp.Output.VideoURL)
	}
}

func TestAliListTasksBuildsQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/" {
			t.Fatalf("path = %q, want /api/v1/tasks/", r.URL.Path)
		}
		if r.URL.Query().Get("page_size") != "10" || r.URL.Query().Get("status") != "SUCCEEDED" {
			t.Fatalf("query = %v, want page_size/status", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"task_id":"task-1","task_status":"SUCCEEDED","model":"wan2.2-t2v","submit_time":"2025-03-20T10:00:00Z"}],"total":1,"page_num":1,"page_size":10}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.ListTasks(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, map[string]string{
		"page_size": "10",
		"status":    "SUCCEEDED",
	})
	if err != nil {
		t.Fatalf("ListTasks error = %v", err)
	}
	if resp.Total != 1 || resp.PageSize != 10 {
		t.Fatalf("resp = %#v, want parsed pagination", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].TaskID != "task-1" {
		t.Fatalf("items = %#v, want one task item", resp.Items)
	}
}
