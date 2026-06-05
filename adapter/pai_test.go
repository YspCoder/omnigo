package adapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestPaiBuildPayloadTextMode(t *testing.T) {
	adaptor := &PaiAdaptor{}
	req := &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "v6",
		Size:     "16:9",
		Duration: 5,
		Seed:     7,
		Messages: []dto.Message{{Role: "user", Content: "a fox running through snow"}},
		Extra: map[string]interface{}{
			"mode":       "text-to-video",
			"quality":    "540p",
			"water_mark": false,
		},
	}

	mode, _, err := adaptor.resolveMode(req)
	if err != nil {
		t.Fatalf("resolveMode error = %v", err)
	}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{}, mode, req)
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if payload["model"] != "v6" || payload["aspect_ratio"] != "16:9" || payload["quality"] != "540p" {
		t.Fatalf("payload = %#v, want common fields", payload)
	}
}

func TestPaiMediaUploadsImageAndCreatesTask(t *testing.T) {
	var uploadCalled, generateCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/v2/image/upload":
			uploadCalled = true
			if got := r.Header.Get("API-KEY"); got != "secret" {
				t.Fatalf("API-KEY = %q, want secret", got)
			}
			if got := r.Header.Get("Ai-Trace-Id"); got == "" {
				t.Fatal("Ai-Trace-Id should not be empty")
			}
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Fatalf("ParseMultipartForm error = %v", err)
			}
			file, _, err := r.FormFile("image")
			if err != nil {
				t.Fatalf("FormFile error = %v", err)
			}
			defer file.Close()
			body, _ := io.ReadAll(file)
			if string(body) != "fake-image" {
				t.Fatalf("uploaded body = %q, want fake-image", string(body))
			}
			_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"success","Resp":{"img_id":123,"img_url":"https://cdn.example.com/in.png"}}`))
		case "/openapi/v2/video/img/generate":
			generateCalled = true
			body, _ := io.ReadAll(r.Body)
			text := string(body)
			if !strings.Contains(text, `"img_id":123`) {
				t.Fatalf("generate payload = %s, want img_id 123", text)
			}
			_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"success","Resp":{"video_id":456}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "v6",
		Messages: []dto.Message{{Role: "user", Content: "turn this into a flying dragon"}},
		Extra: map[string]interface{}{
			"image": "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-image")),
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if !uploadCalled || !generateCalled {
		t.Fatalf("uploadCalled=%v generateCalled=%v, want both true", uploadCalled, generateCalled)
	}
	if resp.TaskID != "456" {
		t.Fatalf("task id = %q, want 456", resp.TaskID)
	}
}

func TestPaiTaskStatusMapsSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2/video/result/456" {
			t.Fatalf("path = %q, want task status path", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"success","Resp":{"id":456,"prompt":"hello","status":1,"url":"https://cdn.example.com/video.mp4","outputWidth":1280,"outputHeight":720,"seed":9}}`))
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, "456")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskStatus != "success" {
		t.Fatalf("status = %q, want success", resp.Output.TaskStatus)
	}
	if resp.Output.VideoURL != "https://cdn.example.com/video.mp4" {
		t.Fatalf("video url = %q, want mapped url", resp.Output.VideoURL)
	}
	if resp.Output.Ratio != "1280:720" {
		t.Fatalf("ratio = %q, want 1280:720", resp.Output.Ratio)
	}
}

func TestPaiBuildPayloadExtendUsesSourceVideoID(t *testing.T) {
	adaptor := &PaiAdaptor{}
	req := &dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "v5",
		Extra: map[string]interface{}{
			"mode":            "extend",
			"source_video_id": 321,
			"quality":         "540p",
			"motion_mode":     "normal",
		},
		Messages: []dto.Message{{Role: "user", Content: "continue the motion into the stars"}},
	}

	mode, _, err := adaptor.resolveMode(req)
	if err != nil {
		t.Fatalf("resolveMode error = %v", err)
	}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{}, mode, req)
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if payload["source_video_id"] != uint64(321) {
		t.Fatalf("payload = %#v, want source_video_id 321", payload)
	}
}

func TestPaiBuildPayloadMultiTransitionUploadsImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2/image/upload" {
			t.Fatalf("path = %q, want upload path", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"success","Resp":{"img_id":777}}`))
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, paiModeMultiTransition, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"multi_transition": []interface{}{
				map[string]interface{}{
					"image":    "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("frame-a")),
					"duration": 3,
					"prompt":   "first scene",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	items, ok := payload["multi_transition"].([]map[string]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("payload = %#v, want one multi_transition item", payload)
	}
	if items[0]["img_id"] != uint64(777) {
		t.Fatalf("item = %#v, want uploaded img_id", items[0])
	}
}

func TestPaiBuildPayloadLipSyncUsesTTSFields(t *testing.T) {
	adaptor := &PaiAdaptor{}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{}, paiModeLipSync, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"video_media_id":          99,
			"lip_sync_tts_speaker_id": "auto",
			"lip_sync_tts_content":    "hello world",
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if payload["video_media_id"] != uint64(99) {
		t.Fatalf("payload = %#v, want video_media_id 99", payload)
	}
	if payload["lip_sync_tts_content"] != "hello world" {
		t.Fatalf("payload = %#v, want lip_sync_tts_content", payload)
	}
}

func TestPaiMaskSelectionReturnsStructuredText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2/video/mask/selection" {
			t.Fatalf("path = %q, want mask selection path", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"Success","Resp":{"keyframe_id":1,"keyframe_url":"https://cdn.example.com/frame.png","credits":2,"mask_info":[{"mask_id":"0","mask_name":"bear","mask_url":"https://cdn.example.com/mask.png"}]}}`))
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"mode":            "mask-selection",
			"source_video_id": 12,
			"keyframe_id":     1,
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.URL != "https://cdn.example.com/frame.png" {
		t.Fatalf("resp = %#v, want keyframe url", resp)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Text), &parsed); err != nil {
		t.Fatalf("resp.Text should be JSON, got error %v", err)
	}
}

func TestPaiBuildPayloadSwapUsesAutoMaskSelection(t *testing.T) {
	var maskSelectionCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/v2/video/mask/selection":
			maskSelectionCalled = true
			_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"Success","Resp":{"keyframe_id":5,"mask_info":[{"mask_id":"bear"}]}}`))
		case "/openapi/v2/image/upload":
			_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"success","Resp":{"img_id":321}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, paiModeSwap, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"source_video_id":     7,
			"auto_mask_selection": true,
			"image":               "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-image")),
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if !maskSelectionCalled {
		t.Fatal("expected auto mask selection to be called")
	}
	if payload["keyframe_id"] != 5 || payload["mask_id"] != "bear" {
		t.Fatalf("payload = %#v, want mask selection fields", payload)
	}
}

func TestPaiBuildPayloadSoundEffectUsesVideoReference(t *testing.T) {
	adaptor := &PaiAdaptor{}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{}, paiModeSoundEffect, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"source_video_id":       11,
			"original_sound_switch": true,
			"sound_effect_content":  "thunder and rain",
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if payload["source_video_id"] != uint64(11) || payload["sound_effect_content"] != "thunder and rain" {
		t.Fatalf("payload = %#v, want sound effect fields", payload)
	}
}

func TestPaiBuildPayloadRestyleUsesRestyleID(t *testing.T) {
	adaptor := &PaiAdaptor{}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{}, paiModeRestyle, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"source_video_id": 9,
			"restyle_id":      3,
			"seed":            8,
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if payload["source_video_id"] != uint64(9) || payload["restyle_id"] != 3 {
		t.Fatalf("payload = %#v, want restyle fields", payload)
	}
}

func TestPaiListTasksRestyleReturnsAvailableItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2/video/restyle/list" {
			t.Fatalf("path = %q, want restyle list path", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"Success","Resp":[{"restyle_id":1,"restyle_name":"anime","restyle_prompt":"anime style","cover_url":"https://cdn.example.com/anime.png"}]}`))
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	resp, err := adaptor.ListTasks(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, map[string]string{"mode": "restyle"})
	if err != nil {
		t.Fatalf("ListTasks error = %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "1" || resp.Items[0].State != "available" {
		t.Fatalf("resp = %#v, want one available restyle item", resp)
	}
}

func TestPaiBuildPayloadModifyUsesImageAndMaskArrays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2/image/upload" {
			t.Fatalf("path = %q, want upload path", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ErrCode":0,"ErrMsg":"success","Resp":{"img_id":654}}`))
	}))
	defer server.Close()

	adaptor := &PaiAdaptor{}
	payload, err := adaptor.buildPayload(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, paiModeModify, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Messages: []dto.Message{
			{Role: "user", Content: "@selection0 subject is swapped with @img0"},
		},
		Extra: map[string]interface{}{
			"video_media_id": 1234,
			"mask_ids":       []interface{}{"3847593904"},
			"keyframe_ids":   []interface{}{1},
			"quality":        "540p",
			"image":          "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-image")),
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	imgIDs, ok := payload["img_ids"].([]uint64)
	if !ok || len(imgIDs) != 1 || imgIDs[0] != 654 {
		t.Fatalf("payload = %#v, want uploaded img_ids", payload)
	}
}
