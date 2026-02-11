// Package adapter provides Volcengine Jimeng adaptor implementation.
package adapter

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

// JimengAdaptor converts requests and responses for Jimeng APIs.
type JimengAdaptor struct {
	BaseURL string
}

func (a *JimengAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	return nil, fmt.Errorf("chat mode not supported for Jimeng")
}

func (a *JimengAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming not supported for Jimeng")
}

func (a *JimengAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	url := "https://visual.volcengineapi.com/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31"
	if config.BaseURL != "" {
		url = strings.TrimRight(config.BaseURL, "/") + "/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31"
	}

	payload := map[string]interface{}{
		"req_key": config.Model,
		"prompt":  request.Prompt,
	}
	if request.Duration > 0 {
		payload["frames"] = 24*request.Duration + 1
	}
	if request.Size != "" {
		payload["aspect_ratio"] = request.Size
	}
	for k, v := range request.Extra {
		payload[k] = v
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	
	if err := a.signV4(req, config.AccessKey, config.SecretKey, "cn-north-1", "cv", body); err != nil {
		return nil, err
	}

	resp, err := config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var res struct {
		Code int `json:"code"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}
	if res.Code != 10000 {
		return nil, fmt.Errorf("jimeng error: %s", res.Message)
	}

	return &dto.MediaResponse{
		TaskID: res.Data.TaskID,
		Status: "submitted",
	}, nil
}

func (a *JimengAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	url := "https://visual.volcengineapi.com/?Action=CVSync2AsyncGetResult&Version=2022-08-31"
	if config.BaseURL != "" {
		url = strings.TrimRight(config.BaseURL, "/") + "/?Action=CVSync2AsyncGetResult&Version=2022-08-31"
	}

	payload := map[string]interface{}{
		"req_key": config.Model,
		"task_id": taskID,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	
	if err := a.signV4(req, config.AccessKey, config.SecretKey, "cn-north-1", "cv", body); err != nil {
		return nil, err
	}

	resp, err := config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var res struct {
		Code int `json:"code"`
		Data struct {
			Status   string `json:"status"`
			VideoURL string `json:"video_url"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}
	if res.Code != 10000 {
		return nil, fmt.Errorf("jimeng error: %s", res.Message)
	}

	return &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     taskID,
			TaskStatus: res.Data.Status,
			VideoURL:   res.Data.VideoURL,
		},
	}, nil
}

func (a *JimengAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Jimeng adaptor")
}

func (a *JimengAdaptor) signV4(req *http.Request, ak, sk, region, service string, body []byte) error {
	now := time.Now().UTC()
	date := now.Format("20060102T150405Z")
	authDate := date[:8]
	req.Header.Set("X-Date", date)
	req.Header.Set("Content-Type", "application/json")

	payloadHash := hex.EncodeToString(hashSHA256(body))
	req.Header.Set("X-Content-Sha256", payloadHash)

	queryString := strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
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

	hashedCanonicalString := hex.EncodeToString(hashSHA256([]byte(canonicalString)))
	credentialScope := authDate + "/" + region + "/" + service + "/request"
	signString := strings.Join([]string{"HMAC-SHA256", date, credentialScope, hashedCanonicalString}, "\n")

	kDate := hmacSign([]byte(sk), authDate)
	kRegion := hmacSign(kDate, region)
	kService := hmacSign(kRegion, service)
	kSigning := hmacSign(kService, "request")
	signature := hex.EncodeToString(hmacSign(kSigning, signString))

	authorization := "HMAC-SHA256" + " Credential=" + ak + "/" + credentialScope + ", SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + signature
	req.Header.Set("Authorization", authorization)
	return nil
}
