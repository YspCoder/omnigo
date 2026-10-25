package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
	"github.com/openai/openai-go/v3"
)

// imageRequest shares native parameters between ordinary and streaming images.
// Multipart extras are strings because the installed SDK's multipart encoder
// only writes string extras, and EditStreaming overwrites them with stream=true.
func (a *OpenAIAdaptor) imageRequest(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest, stream bool) (*http.Response, error) {
	if _, err := a.getClient(config); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{"model": request.Model, "prompt": utils.MediaPromptWithSystem(request)}
	if request.N > 0 {
		payload["n"] = request.N
	}
	for key, value := range map[string]string{"size": request.Size, "quality": request.Resolution, "response_format": request.ResponseFormat} {
		if value != "" {
			payload[key] = value
		}
	}
	for _, key := range []string{"background", "input_fidelity", "moderation", "output_compression", "output_format", "partial_images", "user", "safety_identifier"} {
		if value, ok := request.Extra[key]; ok {
			payload[key] = value
		}
	}
	for key, value := range request.Options {
		if key != "image" && key != "images" && key != "files" && key != "mask" {
			payload[key] = value
		}
	}
	if stream {
		payload["stream"] = true
	} else {
		delete(payload, "stream")
	}
	inputs := utils.ParseExtraImageInputs(request.Extra)
	if optionsInputs := utils.ParseExtraImageInputs(request.Options); len(optionsInputs) > 0 {
		inputs = optionsInputs
	}
	if len(inputs) == 0 {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return a.doOpenAIRequest(ctx, config, "/images/generations", "application/json", body)
	}
	images, err := openAIImageReferenceReaders(ctx, a.httpClient, inputs)
	if err != nil {
		return nil, err
	}
	prompt, ok := payload["prompt"].(string)
	if !ok {
		return nil, fmt.Errorf("openai image prompt must be a string")
	}
	params := openai.ImageEditParams{Prompt: prompt, Image: openAIImageEditInput(images)}
	maskValue := request.Extra["mask"]
	if value, ok := request.Options["mask"]; ok {
		maskValue = value
	}
	masks := utils.ContentImageURLs(maskValue)
	if len(masks) > 1 {
		return nil, fmt.Errorf("openai image edit accepts one mask")
	}
	if len(masks) == 1 {
		params.Mask, err = openAIImageReferenceReader(ctx, a.httpClient, masks[0], 0)
		if err != nil {
			return nil, err
		}
	}
	extras := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		if key == "prompt" || value == nil {
			continue
		}
		extras[key], err = customMultipartValue(value)
		if err != nil {
			return nil, fmt.Errorf("encode openai image parameter %q: %w", key, err)
		}
	}
	params.SetExtraFields(extras)
	body, contentType, err := params.MarshalMultipart()
	if err != nil {
		return nil, err
	}
	return a.doOpenAIRequest(ctx, config, "/images/edits", contentType, body)
}

func (a *OpenAIAdaptor) generateImage(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	response, err := a.imageRequest(ctx, config, request, false)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("openai images api error: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result openai.ImagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if async, _ := utils.GetBoolExtra(request.Extra, "async"); async {
		return openAIAsyncImageResponse(&result)
	}
	return openAIImageResponse(&result), nil
}

func (a *OpenAIAdaptor) streamImages(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	response, err := a.imageRequest(streamCtx, config, request, true)
	if err != nil {
		cancel()
		return nil, err
	}
	return openAIHTTPStream(streamCtx, response, cancel, openAIImageEvent)
}

func openAIImageEvent(raw []byte, eventType string) ([]*dto.StreamToken, bool, error) {
	var event struct {
		Type    string                `json:"type"`
		B64JSON string                `json:"b64_json"`
		URL     string                `json:"url"`
		Index   int                   `json:"partial_image_index"`
		Usage   json.RawMessage       `json:"usage"`
		Error   *openAIResponsesError `json:"error"`
		Code    string                `json:"code"`
		Message string                `json:"message"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, true, err
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, true, err
	}
	delete(metadata, "b64_json")
	delete(metadata, "usage")
	event.Type = firstNonEmptyString(event.Type, eventType)
	token := &dto.StreamToken{Type: event.Type, Text: event.B64JSON, URL: event.URL, Index: event.Index, Usage: usagePointer(usageFromJSON(event.Usage)), Metadata: metadata}
	if event.Type == "error" || event.Error != nil {
		message := event.Message
		if event.Error != nil {
			message = event.Error.Message
		}
		token.Type, token.Text = "error", firstNonEmptyString(message, "image stream failed")
		return []*dto.StreamToken{token}, true, fmt.Errorf("openai image stream: %s", token.Text)
	}
	if strings.HasSuffix(event.Type, ".completed") {
		token.FinishReason = "completed"
	}
	// Multiple requested images can have separate completion events. The HTTP
	// stream or [DONE] ends the request after all final images have been delivered.
	return []*dto.StreamToken{token}, false, nil
}

func openAIVideoRetiredError() error {
	return fmt.Errorf("OpenAI Videos API and Sora 2 were retired on 2026-09-24; use a supported provider or a separately configured gateway")
}
