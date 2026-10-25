package adapter

import (
	"reflect"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestAliNativeVideoMedia(t *testing.T) {
	raw := []map[string]interface{}{
		{"type": "first_clip", "url": "https://example.com/clip.mp4", "custom": "retained"},
		{"type": "last_frame", "url": "https://example.com/last.png"},
	}
	r := &dto.MediaRequest{Model: "wan2.7-i2v", Extra: map[string]interface{}{"media": raw}}
	_, payload, err := (&AliAdaptor{}).buildVideoRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got := payload["input"].(map[string]interface{})["media"]; !reflect.DeepEqual(got, raw) {
		t.Fatalf("media=%v", got)
	}
	r.Extra = map[string]interface{}{"image": "https://example.com/first.png", "driving_audio": "https://example.com/audio.mp3"}
	_, payload, err = (&AliAdaptor{}).buildVideoRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	media := payload["input"].(map[string]interface{})["media"].([]map[string]interface{})
	if len(media) != 2 || media[1]["type"] != "driving_audio" {
		t.Fatalf("media=%v", media)
	}
	r.Model = "wan2.6-i2v"
	if _, _, err := (&AliAdaptor{}).buildVideoRequest(r); err == nil {
		t.Fatal("legacy model accepted native media")
	}
	r.Extra = map[string]interface{}{"image": "https://example.com/first.png", "audio_url": "https://example.com/audio.mp3"}
	_, payload, err = (&AliAdaptor{}).buildVideoRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	input := payload["input"].(map[string]interface{})
	if input["img_url"] == nil || input["audio_url"] == nil || input["media"] != nil {
		t.Fatalf("legacy input=%v", input)
	}
}
