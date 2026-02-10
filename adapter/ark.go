// Package adapter provides Volcengine Ark (火山方舟) adaptor implementation.
package adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

// ArkAdaptor converts requests and responses for Volcengine Ark APIs.
// It follows the patterns seen in github.com/volcengine/volcengine-go-sdk/service/arkruntime.
type ArkAdaptor struct {
	BaseURL string
	Region  string
}

// GetURL returns the Ark endpoint for the given mode and optional taskID.
func (a *ArkAdaptor) GetURL(mode string, config *ProviderConfig, taskID string) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		// Default to beijing region if not specified
		base = "https://ark.cn-beijing.volces.com/api/v3"
	}

	switch mode {
	case ModeChat:
		return base + "/chat/completions", nil
	case ModeImage:
		return base + "/image/generations", nil
	case ModeVideo:
		return base + "/video/generations", nil
	case ModeTask:
		if taskID == "" {
			return "", errors.New("task_id is required for task query")
		}
		return base + "/tasks/" + taskID, nil
	default:
		return "", fmt.Errorf("unsupported mode for Ark: %s", mode)
	}
}

// SetupHeaders sets Ark headers with Bearer token or V4 signing.
func (a *ArkAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string, body []byte) error {
	req.Header.Set("Content-Type", "application/json")

	if config.AccessKey != "" && config.SecretKey != "" {
		region := a.Region
		if region == "" {
			region = "cn-beijing" // default
		}
		// Attempt to extract region from BaseURL if possible
		if config.BaseURL != "" {
			if strings.Contains(config.BaseURL, "ark.cn-") {
				parts := strings.Split(config.BaseURL, ".")
				for _, p := range parts {
					if strings.HasPrefix(p, "cn-") {
						region = p
						break
					}
				}
			}
		}

		return a.signV4(req, config.AccessKey, config.SecretKey, region, "air", body)
	} else if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}

	return nil
}

func (a *ArkAdaptor) signV4(req *http.Request, ak, sk, region, service string, body []byte) error {
	now := time.Now().UTC()
	date := now.Format("20060102T150405Z")
	authDate := date[:8]

	req.Header.Set("X-Date", date)

	// Hash body
	payloadHash := hex.EncodeToString(arkHashSHA256(body))
	req.Header.Set("X-Content-Sha256", payloadHash)

	// Canonical Query String
	queryString := strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")

	// Signed Headers
	signedHeaders := []string{"content-type", "host", "x-content-sha256", "x-date"}
	var headerList []string
	for _, hdr := range signedHeaders {
		if hdr == "host" {
			headerList = append(headerList, hdr+":"+req.Host)
		} else {
			headerList = append(headerList, hdr+":"+strings.TrimSpace(req.Header.Get(hdr)))
		}
	}
	headerString := strings.Join(headerList, "\n")

	canonicalString := strings.Join([]string{
		req.Method,
		req.URL.Path,
		queryString,
		headerString + "\n",
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")

	hashedCanonicalString := hex.EncodeToString(arkHashSHA256([]byte(canonicalString)))

	credentialScope := authDate + "/" + region + "/" + service + "/request"

	signString := strings.Join([]string{
		"HMAC-SHA256",
		date,
		credentialScope,
		hashedCanonicalString,
	}, "\n")

	// Signing Key
	kDate := arkHMACSign([]byte(sk), authDate)
	kRegion := arkHMACSign(kDate, region)
	kService := arkHMACSign(kRegion, service)
	kSigning := arkHMACSign(kService, "request")

	// Final Signature
	signature := hex.EncodeToString(arkHMACSign(kSigning, signString))

	authorization := "HMAC-SHA256" + " Credential=" + ak + "/" + credentialScope + ", SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + signature
	req.Header.Set("Authorization", authorization)

	return nil
}

func arkHashSHA256(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func arkHMACSign(key []byte, content string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(content))
	return mac.Sum(nil)
}

// ConvertChatRequest handles chat completion requests (OpenAI compatible).
func (a *ArkAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return (&OpenAIAdaptor{}).ConvertChatRequest(ctx, config, request)
}

// ConvertChatResponse handles chat completion responses (OpenAI compatible).
func (a *ArkAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	return (&OpenAIAdaptor{}).ConvertChatResponse(ctx, config, body)
}

// ConvertMediaRequest handles image/video generation requests (OpenAI compatible style).
func (a *ArkAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	return (&OpenAIAdaptor{}).ConvertMediaRequest(ctx, config, mode, request)
}

// ConvertMediaResponse handles image/video generation responses.
func (a *ArkAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	var response struct {
		ID      string `json:"id"`
		TaskID  string `json:"task_id"`
		Created int64  `json:"created"`
		Data    []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Error.Message,
			Provider: "ark",
		}
	}

	res := &dto.MediaResponse{
		Created: response.Created,
		TaskID:  response.TaskID,
	}

	if res.TaskID == "" {
		res.TaskID = response.ID
	}

	for _, item := range response.Data {
		res.Data = append(res.Data, dto.ImageData{
			URL:     item.URL,
			B64JSON: item.B64JSON,
		})
	}

	if len(res.Data) > 0 {
		res.URL = res.Data[0].URL
	}

	return res, nil
}

// ConvertTaskStatusResponse handles the async task query response.
func (a *ArkAdaptor) ConvertTaskStatusResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.TaskStatusResponse, error) {
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Result struct {
			Video struct {
				URL string `json:"url"`
			} `json:"video"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"result"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	res := &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     response.ID,
			TaskStatus: response.Status,
		},
	}

	if response.Error != nil {
		res.Output.Code = response.Error.Code
		res.Output.Message = response.Error.Message
	}

	if response.Result.Video.URL != "" {
		res.Output.VideoURL = response.Result.Video.URL
	} else if len(response.Result.Images) > 0 {
		res.Output.VideoURL = response.Result.Images[0].URL
	}

	return res, nil
}

// PrepareStreamRequest handles streaming setup (OpenAI compatible).
func (a *ArkAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return (&OpenAIAdaptor{}).PrepareStreamRequest(ctx, config, request)
}

// ParseStreamResponse handles streaming chunks (OpenAI compatible).
func (a *ArkAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	return (&OpenAIAdaptor{}).ParseStreamResponse(chunk)
}
