// Package adapter provides Pai adaptor implementation.
package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

const (
	paiModeText            = "text-to-video"
	paiModeImage           = "image-to-video"
	paiModeTransition      = "transition"
	paiModeExtend          = "extend"
	paiModeSwap            = "swap"
	paiModeMultiTransition = "multi-transition"
	paiModeMimic           = "mimic"
	paiModeLipSync         = "lip-sync"
	paiModeMaskSelection   = "mask-selection"
	paiModeSoundEffect     = "sound-effect"
	paiModeRestyle         = "restyle"
	paiModeModify          = "modify"
)

var paiModeEndpoint = map[string]string{
	paiModeText:            "/openapi/v2/video/text/generate",
	paiModeImage:           "/openapi/v2/video/img/generate",
	paiModeTransition:      "/openapi/v2/video/transition/generate",
	paiModeExtend:          "/openapi/v2/video/extend/generate",
	paiModeSwap:            "/openapi/v2/video/swap/generate",
	paiModeMultiTransition: "/openapi/v2/video/multi_transition/generate",
	paiModeMimic:           "/openapi/v2/video/mimic/generate",
	paiModeLipSync:         "/openapi/v2/video/lip_sync/generate",
	paiModeMaskSelection:   "/openapi/v2/video/mask/selection",
	paiModeSoundEffect:     "/openapi/v2/video/sound_effect/generate",
	paiModeRestyle:         "/openapi/v2/video/restyle/generate",
	paiModeModify:          "/openapi/v2/video/modify/generate",
}

type PaiAdaptor struct{}

type paiEnvelope struct {
	ErrCode int             `json:"ErrCode"`
	ErrMsg  string          `json:"ErrMsg"`
	Resp    json.RawMessage `json:"Resp"`
}

type paiTaskCreateResponse struct {
	VideoID uint64 `json:"video_id"`
	Credits int    `json:"credits"`
}

type paiUploadResponse struct {
	ImgID  uint64 `json:"img_id"`
	ImgURL string `json:"img_url"`
}

type paiTaskResult struct {
	CreateTime      string `json:"create_time"`
	ID              uint64 `json:"id"`
	ModifyTime      string `json:"modify_time"`
	NegativePrompt  string `json:"negative_prompt"`
	OutputHeight    int    `json:"outputHeight"`
	OutputWidth     int    `json:"outputWidth"`
	Prompt          string `json:"prompt"`
	ResolutionRatio int    `json:"resolution_ratio"`
	Seed            int    `json:"seed"`
	Size            int    `json:"size"`
	Status          int    `json:"status"`
	Style           string `json:"style"`
	URL             string `json:"url"`
}

type paiMaskSelectionResponse struct {
	KeyframeID  int                    `json:"keyframe_id"`
	KeyframeURL string                 `json:"keyframe_url"`
	Credits     int                    `json:"credits"`
	MaskInfo    []paiMaskSelectionItem `json:"mask_info"`
}

type paiMaskSelectionItem struct {
	MaskID   string `json:"mask_id"`
	MaskName string `json:"mask_name"`
	MaskURL  string `json:"mask_url"`
}

type paiRestyleListItem struct {
	RestyleID     int    `json:"restyle_id"`
	RestyleName   string `json:"restyle_name"`
	RestylePrompt string `json:"restyle_prompt"`
	CoverURL      string `json:"cover_url"`
}

type paiPollingStream struct {
	adaptor *PaiAdaptor
	cfg     *ProviderConfig
	taskID  string
	last    string
}

func (a *PaiAdaptor) Chat(context.Context, *ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("pai adaptor does not support chat")
}

func (a *PaiAdaptor) Stream(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("pai adaptor does not support chat stream")
}

func (a *PaiAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	if r == nil {
		return nil, fmt.Errorf("pai request is required")
	}

	mode, endpoint, err := a.resolveMode(r)
	if err != nil {
		return nil, err
	}

	payload, err := a.buildPayload(ctx, cfg, mode, r)
	if err != nil {
		return nil, err
	}

	var out paiTaskCreateResponse
	if mode == paiModeMaskSelection {
		var maskOut paiMaskSelectionResponse
		if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, &maskOut); err != nil {
			return nil, err
		}
		resp := &dto.MediaResponse{
			Status: "success",
			Text:   uint64ToString(uint64(maskOut.KeyframeID)),
		}
		if len(maskOut.MaskInfo) > 0 {
			raw, _ := json.Marshal(maskOut)
			resp.Text = string(raw)
		}
		if maskOut.KeyframeURL != "" {
			resp.URL = maskOut.KeyframeURL
		}
		return resp, nil
	}
	if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, &out); err != nil {
		return nil, err
	}

	taskID := uint64ToString(out.VideoID)
	return &dto.MediaResponse{
		TaskID: taskID,
		Status: paiStatusText(5),
		Model:  r.Model,
		Text:   strings.TrimSpace(utils.MediaPromptWithSystem(r)),
	}, nil
}

func (a *PaiAdaptor) StreamMedia(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	resp, err := a.Media(ctx, cfg, r)
	if err != nil {
		return nil, err
	}
	if resp.TaskID == "" {
		return nil, fmt.Errorf("pai did not return a task id")
	}
	return &paiPollingStream{adaptor: a, cfg: cfg, taskID: resp.TaskID, last: resp.Status}, nil
}

func (a *PaiAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("task id is required")
	}

	var out paiTaskResult
	if err := a.doJSON(ctx, cfg, http.MethodGet, "/openapi/v2/video/result/"+url.PathEscape(taskID), nil, &out); err != nil {
		return nil, err
	}

	ratio := ""
	if out.OutputWidth > 0 && out.OutputHeight > 0 {
		ratio = fmt.Sprintf("%d:%d", out.OutputWidth, out.OutputHeight)
	}

	return &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:       firstNonEmptyString(uint64ToString(out.ID), taskID),
			TaskStatus:   paiStatusText(out.Status),
			SubmitTime:   out.CreateTime,
			EndTime:      out.ModifyTime,
			URL:          out.URL,
			VideoURL:     out.URL,
			Ratio:        ratio,
			Seed:         out.Seed,
			OrigPrompt:   out.Prompt,
			ActualPrompt: out.Prompt,
			Code:         strconv.Itoa(out.Status),
			Message:      out.NegativePrompt,
		},
	}, nil
}

func (a *PaiAdaptor) ListTasks(ctx context.Context, cfg *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	mode := normalizePaiMode(query["mode"])
	if mode == "" {
		mode = normalizePaiMode(query["task_type"])
	}
	switch mode {
	case paiModeRestyle:
		var items []paiRestyleListItem
		if err := a.doJSON(ctx, cfg, http.MethodGet, "/openapi/v2/video/restyle/list", nil, &items); err != nil {
			return nil, err
		}
		out := make([]dto.TaskListItem, 0, len(items))
		for _, item := range items {
			payload := map[string]interface{}{
				"restyle_prompt": item.RestylePrompt,
				"cover_url":      item.CoverURL,
			}
			out = append(out, dto.TaskListItem{
				ID:      strconv.Itoa(item.RestyleID),
				TaskID:  strconv.Itoa(item.RestyleID),
				State:   "available",
				Model:   item.RestyleName,
				Payload: payload,
			})
		}
		return &dto.TaskListResponse{
			Items: out,
			Tasks: out,
			Total: len(out),
		}, nil
	default:
		return nil, fmt.Errorf("task list not supported by pai for mode: %s", mode)
	}
}

func (a *PaiAdaptor) resolveMode(r *dto.MediaRequest) (string, string, error) {
	if mode := normalizePaiMode(utils.GetStringExtra(r.Extra, "mode")); mode != "" {
		return mode, paiModeEndpoint[mode], nil
	}
	if mode := normalizePaiMode(utils.GetStringExtra(r.Extra, "task_type")); mode != "" {
		return mode, paiModeEndpoint[mode], nil
	}

	switch r.Type {
	case dto.MediaTypeVideo:
		if a.looksLikeLipSync(r) {
			return paiModeLipSync, paiModeEndpoint[paiModeLipSync], nil
		}
		if a.looksLikeSoundEffect(r) {
			return paiModeSoundEffect, paiModeEndpoint[paiModeSoundEffect], nil
		}
		if a.looksLikeRestyle(r) {
			return paiModeRestyle, paiModeEndpoint[paiModeRestyle], nil
		}
		if a.looksLikeModify(r) {
			return paiModeModify, paiModeEndpoint[paiModeModify], nil
		}
		if a.looksLikeMaskSelection(r) {
			return paiModeMaskSelection, paiModeEndpoint[paiModeMaskSelection], nil
		}
		if a.looksLikeSwap(r) {
			return paiModeSwap, paiModeEndpoint[paiModeSwap], nil
		}
		if a.looksLikeMultiTransition(r) {
			return paiModeMultiTransition, paiModeEndpoint[paiModeMultiTransition], nil
		}
		if a.looksLikeMimic(r) {
			return paiModeMimic, paiModeEndpoint[paiModeMimic], nil
		}
		if a.looksLikeExtend(r) {
			return paiModeExtend, paiModeEndpoint[paiModeExtend], nil
		}
		images := paiImageInputs(r)
		if utils.GetStringExtra(r.Extra, "first_frame_img") != "" || utils.GetStringExtra(r.Extra, "last_frame_img") != "" {
			return paiModeTransition, paiModeEndpoint[paiModeTransition], nil
		}
		if len(images) >= 2 {
			return paiModeTransition, paiModeEndpoint[paiModeTransition], nil
		}
		if len(images) == 1 {
			return paiModeImage, paiModeEndpoint[paiModeImage], nil
		}
		return paiModeText, paiModeEndpoint[paiModeText], nil
	default:
		return "", "", fmt.Errorf("pai only supports video generation")
	}
}

func normalizePaiMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "text", "text-to-video", "text2video":
		return paiModeText
	case "image", "image-to-video", "img-to-video", "img2video":
		return paiModeImage
	case "transition", "start-end", "start_end", "first-last":
		return paiModeTransition
	case "extend":
		return paiModeExtend
	case "swap":
		return paiModeSwap
	case "multi-transition", "multi_transition":
		return paiModeMultiTransition
	case "mimic":
		return paiModeMimic
	case "lip-sync", "lip_sync", "lipsync":
		return paiModeLipSync
	case "mask-selection", "mask_selection":
		return paiModeMaskSelection
	case "sound-effect", "sound_effect":
		return paiModeSoundEffect
	case "restyle":
		return paiModeRestyle
	case "modify":
		return paiModeModify
	default:
		return ""
	}
}

func (a *PaiAdaptor) buildPayload(ctx context.Context, cfg *ProviderConfig, mode string, r *dto.MediaRequest) (map[string]interface{}, error) {
	payload := make(map[string]interface{})
	consumed := map[string]struct{}{
		"mode":                    {},
		"task_type":               {},
		"image":                   {},
		"images":                  {},
		"first_frame_img":         {},
		"last_frame_img":          {},
		"first_frame_image":       {},
		"last_frame_image":        {},
		"watermark":               {},
		"aspect_ratio":            {},
		"quality":                 {},
		"duration":                {},
		"seed":                    {},
		"prompt":                  {},
		"negative_prompt":         {},
		"template_id":             {},
		"sound_effect_switch":     {},
		"generate_audio_switch":   {},
		"source_video_id":         {},
		"video_media_id":          {},
		"audio_media_id":          {},
		"source_media_id":         {},
		"mask_id":                 {},
		"keyframe_id":             {},
		"multi_transition":        {},
		"img_id":                  {},
		"lip_sync_tts_content":    {},
		"lip_sync_tts_conent":     {},
		"lip_sync_tts_speaker_id": {},
		"restyle_id":              {},
		"restyle_prompt":          {},
		"original_sound_switch":   {},
		"img_ids":                 {},
		"mask_ids":                {},
		"keyframe_ids":            {},
	}

	prompt := strings.TrimSpace(utils.MediaPromptWithSystem(r))
	if explicitPrompt := utils.GetStringExtra(r.Extra, "prompt"); explicitPrompt != "" {
		prompt = explicitPrompt
	}
	if prompt != "" {
		payload["prompt"] = prompt
	}
	if r.Model != "" {
		payload["model"] = r.Model
	}
	if r.Duration != nil {
		payload["duration"] = r.Duration
	}
	if r.Seed != 0 {
		payload["seed"] = r.Seed
	}
	if r.Size != "" {
		payload["aspect_ratio"] = r.Size
	}
	if quality := firstNonEmptyString(r.Resolution, utils.GetStringExtra(r.Extra, "quality")); quality != "" {
		payload["quality"] = quality
	}
	if negative := utils.GetStringExtra(r.Extra, "negative_prompt"); negative != "" {
		payload["negative_prompt"] = negative
	}
	if templateID, ok := intValue(r.Extra["template_id"]); ok {
		payload["template_id"] = templateID
	}
	if watermark, ok := utils.GetBoolExtra(r.Extra, "water_mark"); ok {
		payload["water_mark"] = watermark
	}
	if watermark, ok := utils.GetBoolExtra(r.Extra, "watermark"); ok {
		payload["water_mark"] = watermark
	}
	for _, key := range []string{"motion_mode", "style", "resolution", "camera_movement", "sound_effect_switch", "generate_audio_switch"} {
		if value := utils.GetStringExtra(r.Extra, key); value != "" {
			payload[key] = value
			consumed[key] = struct{}{}
		}
	}
	for _, key := range []string{"sound_effect_switch", "generate_audio_switch"} {
		if value, ok := utils.GetBoolExtra(r.Extra, key); ok {
			payload[key] = value
			consumed[key] = struct{}{}
		}
	}

	switch mode {
	case paiModeText:
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeImage:
		imgID, err := a.resolveImageID(ctx, cfg, firstNonEmptyString(
			utils.GetStringExtra(r.Extra, "img_id"),
			utils.GetStringExtra(r.Extra, "image_id"),
		), paiImageInputs(r))
		if err != nil {
			return nil, err
		}
		payload["img_id"] = imgID
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeTransition:
		firstID, lastID, err := a.resolveTransitionImageIDs(ctx, cfg, r)
		if err != nil {
			return nil, err
		}
		payload["first_frame_img"] = firstID
		payload["last_frame_img"] = lastID
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeExtend:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeSwap:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		if _, ok := r.Extra["auto_mask_selection"]; ok && !hasAnyExtraKey(r.Extra, "mask_id") {
			maskResp, err := a.maskSelection(ctx, cfg, r.Extra)
			if err != nil {
				return nil, err
			}
			if payload["keyframe_id"] == nil && maskResp.KeyframeID > 0 {
				payload["keyframe_id"] = maskResp.KeyframeID
			}
			if payload["mask_id"] == nil && len(maskResp.MaskInfo) > 0 {
				payload["mask_id"] = maskResp.MaskInfo[0].MaskID
			}
		}
		imgID, err := a.resolveImageID(ctx, cfg, firstNonEmptyString(
			utils.GetStringExtra(r.Extra, "img_id"),
			utils.GetStringExtra(r.Extra, "image_id"),
		), paiImageInputs(r))
		if err != nil {
			return nil, err
		}
		payload["img_id"] = imgID
		if maskID := utils.GetStringExtra(r.Extra, "mask_id"); maskID != "" {
			payload["mask_id"] = maskID
		}
		if keyframeID, ok := intValue(r.Extra["keyframe_id"]); ok {
			payload["keyframe_id"] = keyframeID
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeMultiTransition:
		segments, err := a.resolveMultiTransition(ctx, cfg, r)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"multi_transition": segments}, nil
	case paiModeMimic:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		imgID, err := a.resolveImageID(ctx, cfg, firstNonEmptyString(
			utils.GetStringExtra(r.Extra, "img_id"),
			utils.GetStringExtra(r.Extra, "image_id"),
		), paiImageInputs(r))
		if err != nil {
			return nil, err
		}
		payload["img_id"] = imgID
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeLipSync:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		if audioID, ok := uint64FromAny(r.Extra["audio_media_id"]); ok {
			payload["audio_media_id"] = audioID
		}
		if audioID, ok := uint64FromAny(r.Extra["source_media_id"]); ok {
			payload["audio_media_id"] = audioID
		}
		if speakerID := utils.GetStringExtra(r.Extra, "lip_sync_tts_speaker_id"); speakerID != "" {
			payload["lip_sync_tts_speaker_id"] = speakerID
		}
		if ttsContent := firstNonEmptyString(
			utils.GetStringExtra(r.Extra, "lip_sync_tts_content"),
			utils.GetStringExtra(r.Extra, "lip_sync_tts_conent"),
		); ttsContent != "" {
			payload["lip_sync_tts_content"] = ttsContent
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeMaskSelection:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		if keyframeID, ok := intValue(r.Extra["keyframe_id"]); ok {
			payload["keyframe_id"] = keyframeID
		}
		return payload, nil
	case paiModeSoundEffect:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		if original, ok := utils.GetBoolExtra(r.Extra, "original_sound_switch"); ok {
			payload["original_sound_switch"] = original
		}
		if content := utils.GetStringExtra(r.Extra, "sound_effect_content"); content != "" {
			payload["sound_effect_content"] = content
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeRestyle:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		if restyleID, ok := intValue(r.Extra["restyle_id"]); ok {
			payload["restyle_id"] = restyleID
		}
		if restylePrompt := utils.GetStringExtra(r.Extra, "restyle_prompt"); restylePrompt != "" {
			payload["restyle_prompt"] = restylePrompt
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case paiModeModify:
		videoID, sourceKey, err := paiResolveVideoReference(r.Extra)
		if err != nil {
			return nil, err
		}
		payload[sourceKey] = videoID
		if prompt != "" {
			payload["prompt"] = prompt
		}
		if quality := firstNonEmptyString(r.Resolution, utils.GetStringExtra(r.Extra, "quality")); quality != "" {
			payload["quality"] = quality
		}
		if keyframeIDs := paiIntSlice(r.Extra["keyframe_ids"]); len(keyframeIDs) > 0 {
			payload["keyframe_ids"] = keyframeIDs
		} else if keyframeID, ok := intValue(r.Extra["keyframe_id"]); ok {
			payload["keyframe_ids"] = keyframeID
		}
		if maskIDs := paiStringSlice(r.Extra["mask_ids"]); len(maskIDs) > 0 {
			payload["mask_ids"] = maskIDs
		}
		imgIDs, err := a.resolveImageIDs(ctx, cfg, r)
		if err != nil {
			return nil, err
		}
		if len(imgIDs) > 0 {
			payload["img_ids"] = imgIDs
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	default:
		return nil, fmt.Errorf("unsupported pai mode: %s", mode)
	}
}

func (a *PaiAdaptor) looksLikeExtend(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "source_video_id", "video_media_id") && !hasAnyExtraKey(r.Extra, "img_id", "mask_id", "audio_media_id", "source_media_id", "lip_sync_tts_content", "lip_sync_tts_conent", "lip_sync_tts_speaker_id", "multi_transition")
}

func (a *PaiAdaptor) looksLikeSwap(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "mask_id", "keyframe_id")
}

func (a *PaiAdaptor) looksLikeMultiTransition(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	_, ok := r.Extra["multi_transition"]
	return ok
}

func (a *PaiAdaptor) looksLikeMimic(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "source_video_id", "video_media_id") && (len(paiImageInputs(r)) > 0 || hasAnyExtraKey(r.Extra, "img_id", "image_id"))
}

func (a *PaiAdaptor) looksLikeLipSync(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "audio_media_id", "source_media_id", "lip_sync_tts_content", "lip_sync_tts_conent", "lip_sync_tts_speaker_id")
}

func (a *PaiAdaptor) looksLikeMaskSelection(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "auto_mask_selection") || (hasAnyExtraKey(r.Extra, "source_video_id", "video_media_id") && hasAnyExtraKey(r.Extra, "keyframe_id") && !hasAnyExtraKey(r.Extra, "img_id", "mask_id"))
}

func (a *PaiAdaptor) looksLikeSoundEffect(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "sound_effect_content", "original_sound_switch")
}

func (a *PaiAdaptor) looksLikeRestyle(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "restyle_id", "restyle_prompt")
}

func (a *PaiAdaptor) looksLikeModify(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	return hasAnyExtraKey(r.Extra, "img_ids", "mask_ids", "keyframe_ids")
}

func paiImageInputs(r *dto.MediaRequest) []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, 6)
	seen := make(map[string]struct{}, 6)
	appendURL := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	for _, message := range r.Messages {
		if message.ImageURL != "" {
			appendURL(message.ImageURL)
		}
	}
	for _, input := range utils.ParseExtraImageInputs(r.Extra) {
		appendURL(input)
	}
	return out
}

func (a *PaiAdaptor) resolveTransitionImageIDs(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (uint64, uint64, error) {
	if first, ok := uint64FromAny(r.Extra["first_frame_img"]); ok {
		if last, ok := uint64FromAny(r.Extra["last_frame_img"]); ok {
			return first, last, nil
		}
	}
	inputs := paiImageInputs(r)
	if len(inputs) < 2 {
		return 0, 0, fmt.Errorf("pai transition requires two images")
	}
	firstID, err := a.resolveImageID(ctx, cfg, "", []string{inputs[0]})
	if err != nil {
		return 0, 0, err
	}
	lastID, err := a.resolveImageID(ctx, cfg, "", []string{inputs[1]})
	if err != nil {
		return 0, 0, err
	}
	return firstID, lastID, nil
}

func (a *PaiAdaptor) resolveMultiTransition(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) ([]map[string]interface{}, error) {
	raw, ok := r.Extra["multi_transition"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("pai multi-transition requires multi_transition")
	}

	items, ok := raw.([]interface{})
	if !ok {
		if typed, ok := raw.([]map[string]interface{}); ok {
			items = make([]interface{}, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		} else {
			return nil, fmt.Errorf("pai multi-transition must be a slice")
		}
	}

	segments := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		segment, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out := make(map[string]interface{})
		if imgID, ok := uint64FromAny(segment["img_id"]); ok {
			out["img_id"] = imgID
		} else {
			input := firstNonEmptyString(
				paiStringFromAny(segment["image"]),
				paiStringFromAny(segment["img"]),
				paiStringFromAny(segment["img_url"]),
				paiStringFromAny(segment["url"]),
			)
			if input == "" {
				return nil, fmt.Errorf("pai multi-transition segment requires img_id or image input")
			}
			imgID, err := a.uploadImage(ctx, cfg, input)
			if err != nil {
				return nil, err
			}
			out["img_id"] = imgID
		}
		if duration, ok := intValue(segment["duration"]); ok {
			out["duration"] = duration
		}
		if prompt := strings.TrimSpace(paiStringFromAny(segment["prompt"])); prompt != "" {
			out["prompt"] = prompt
		}
		segments = append(segments, out)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("pai multi-transition requires at least one segment")
	}
	return segments, nil
}

func (a *PaiAdaptor) maskSelection(ctx context.Context, cfg *ProviderConfig, extra map[string]interface{}) (*paiMaskSelectionResponse, error) {
	videoID, sourceKey, err := paiResolveVideoReference(extra)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		sourceKey: videoID,
	}
	if keyframeID, ok := intValue(extra["keyframe_id"]); ok {
		payload["keyframe_id"] = keyframeID
	}
	var out paiMaskSelectionResponse
	if err := a.doJSON(ctx, cfg, http.MethodPost, paiModeEndpoint[paiModeMaskSelection], payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *PaiAdaptor) resolveImageID(ctx context.Context, cfg *ProviderConfig, explicit string, inputs []string) (uint64, error) {
	if id, ok := uint64FromAny(explicit); ok {
		return id, nil
	}
	if len(inputs) == 0 {
		return 0, fmt.Errorf("pai image mode requires image input")
	}
	return a.uploadImage(ctx, cfg, inputs[0])
}

func (a *PaiAdaptor) resolveImageIDs(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) ([]uint64, error) {
	if r == nil {
		return nil, nil
	}
	if ids := paiUint64Slice(r.Extra["img_ids"]); len(ids) > 0 {
		return ids, nil
	}
	inputs := paiImageInputs(r)
	if len(inputs) == 0 {
		return nil, nil
	}
	out := make([]uint64, 0, len(inputs))
	for _, input := range inputs {
		id, err := a.uploadImage(ctx, cfg, input)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (a *PaiAdaptor) uploadImage(ctx context.Context, cfg *ProviderConfig, input string) (uint64, error) {
	filename, contentType, data, err := paiReadImageInput(ctx, paiHTTPClient(cfg), input)
	if err != nil {
		return 0, err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		return 0, err
	}
	if _, err := part.Write(data); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(paiBaseURL(cfg), "/")+"/openapi/v2/image/upload", body)
	if err != nil {
		return 0, err
	}
	a.applyHeaders(req, cfg)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		req.Header.Set("X-Upload-Content-Type", contentType)
	}

	resp, err := paiHTTPClient(cfg).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("pai upload error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var env paiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, err
	}
	if env.ErrCode != 0 {
		return 0, fmt.Errorf("pai upload error: code=%d message=%s", env.ErrCode, env.ErrMsg)
	}
	var out paiUploadResponse
	if err := json.Unmarshal(env.Resp, &out); err != nil {
		return 0, err
	}
	if out.ImgID == 0 {
		return 0, fmt.Errorf("pai upload returned empty img_id")
	}
	return out.ImgID, nil
}

func (a *PaiAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, method, path string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(paiBaseURL(cfg), "/")+path, body)
	if err != nil {
		return err
	}
	a.applyHeaders(req, cfg)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := paiHTTPClient(cfg).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pai api error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var env paiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if env.ErrCode != 0 {
		return fmt.Errorf("pai api error: code=%d message=%s", env.ErrCode, env.ErrMsg)
	}
	if out == nil || len(env.Resp) == 0 {
		return nil
	}
	return json.Unmarshal(env.Resp, out)
}

func (a *PaiAdaptor) applyHeaders(req *http.Request, cfg *ProviderConfig) {
	req.Header.Set("API-KEY", cfg.APIKey)
	req.Header.Set("Ai-Trace-Id", paiTraceID())
	for key, value := range cfg.Headers {
		req.Header.Set(key, value)
	}
}

func paiTraceID() string {
	return fmt.Sprintf("pai-%d", time.Now().UnixNano())
}

func paiBaseURL(cfg *ProviderConfig) string {
	if cfg != nil && cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return "https://app-api.pixverseai.cn"
}

func paiHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func paiStatusText(status int) string {
	switch status {
	case 1:
		return "success"
	case 5:
		return "processing"
	case 7:
		return "rejected"
	case 8:
		return "failed"
	default:
		return strconv.Itoa(status)
	}
}

func paiReadImageInput(ctx context.Context, client *http.Client, input string) (string, string, []byte, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", nil, fmt.Errorf("pai image input is empty")
	}
	if strings.HasPrefix(input, "data:") {
		return paiReadDataURL(input)
	}
	if strings.HasPrefix(strings.ToLower(input), "http://") || strings.HasPrefix(strings.ToLower(input), "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
		if err != nil {
			return "", "", nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", "", nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", "", nil, fmt.Errorf("pai image fetch failed: status=%d url=%s", resp.StatusCode, input)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", nil, err
		}
		contentType := resp.Header.Get("Content-Type")
		filename := filepath.Base(resp.Request.URL.Path)
		if filename == "" || filename == "." || filename == "/" {
			filename = paiImageFilename(contentType)
		}
		return filename, contentType, data, nil
	}
	if data, err := os.ReadFile(input); err == nil {
		contentType := http.DetectContentType(data)
		return filepath.Base(input), contentType, data, nil
	}
	if data, err := base64.StdEncoding.DecodeString(input); err == nil {
		contentType := http.DetectContentType(data)
		return paiImageFilename(contentType), contentType, data, nil
	}
	return "", "", nil, fmt.Errorf("pai image input must be a URL, data URL, base64 string, or readable file path")
}

func paiReadDataURL(input string) (string, string, []byte, error) {
	parts := strings.SplitN(input, ",", 2)
	if len(parts) != 2 {
		return "", "", nil, fmt.Errorf("invalid data url")
	}
	header := parts[0]
	encoded := parts[1]
	contentType := strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", nil, fmt.Errorf("decode data url: %w", err)
	}
	return paiImageFilename(contentType), contentType, data, nil
}

func paiImageFilename(contentType string) string {
	exts, _ := mime.ExtensionsByType(contentType)
	if len(exts) > 0 {
		return "image" + exts[0]
	}
	return "image.png"
}

func uint64FromAny(v interface{}) (uint64, bool) {
	switch typed := v.(type) {
	case uint64:
		return typed, true
	case int:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int64:
		if typed >= 0 {
			return uint64(typed), true
		}
	case float64:
		if typed >= 0 {
			return uint64(typed), true
		}
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

func uint64ToString(v uint64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatUint(v, 10)
}

func paiResolveVideoReference(extra map[string]interface{}) (uint64, string, error) {
	if id, ok := uint64FromAny(extra["video_media_id"]); ok {
		return id, "video_media_id", nil
	}
	if id, ok := uint64FromAny(extra["source_video_id"]); ok {
		return id, "source_video_id", nil
	}
	return 0, "", fmt.Errorf("pai mode requires video_media_id or source_video_id")
}

func hasAnyExtraKey(extra map[string]interface{}, keys ...string) bool {
	if extra == nil {
		return false
	}
	for _, key := range keys {
		value, ok := extra[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func paiStringFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func paiStringSlice(v interface{}) []string {
	switch typed := v.(type) {
	case []string:
		if len(typed) == 0 {
			return nil
		}
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(paiStringFromAny(item)); s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func paiIntSlice(v interface{}) []int {
	switch typed := v.(type) {
	case []int:
		if len(typed) == 0 {
			return nil
		}
		return typed
	case []interface{}:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			if n, ok := intValue(item); ok {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func paiUint64Slice(v interface{}) []uint64 {
	switch typed := v.(type) {
	case []uint64:
		if len(typed) == 0 {
			return nil
		}
		return typed
	case []interface{}:
		out := make([]uint64, 0, len(typed))
		for _, item := range typed {
			if n, ok := uint64FromAny(item); ok {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func (w *paiPollingStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		status, err := w.adaptor.TaskStatus(ctx, w.cfg, w.taskID)
		if err != nil {
			return nil, err
		}
		state := status.Output.TaskStatus
		if state == w.last && state != "success" && state != "failed" && state != "rejected" {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		w.last = state
		token := &dto.StreamToken{Type: "progress", Text: state}
		switch state {
		case "success":
			token.Type = "url"
			token.URL = status.Output.VideoURL
			return token, io.EOF
		case "failed", "rejected":
			if status.Output.Message != "" {
				token.Text = status.Output.Message
			}
			return token, fmt.Errorf("pai task %s: %s", state, token.Text)
		default:
			return token, nil
		}
	}
}

func (w *paiPollingStream) Close() error { return nil }
