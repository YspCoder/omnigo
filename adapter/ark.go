// Package adapter provides Volcengine Ark (火山方舟) adaptor implementation.
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
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/utils"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
)

// ArkAdaptor converts requests and responses for Volcengine Ark APIs using the official SDK for Chat
// and manual signed requests for Image/Video generation.
type ArkAdaptor struct {
	client *arkruntime.Client
}

func (a *ArkAdaptor) getClient(config *ProviderConfig) *arkruntime.Client {
	if a.client != nil {
		return a.client
	}
	
	opts := []arkruntime.ConfigOption{
		arkruntime.WithRegion(config.Region),
	}
	if config.BaseURL != "" {
		opts = append(opts, arkruntime.WithBaseUrl(config.BaseURL))
	}
	if config.HTTPClient != nil {
		opts = append(opts, arkruntime.WithHTTPClient(config.HTTPClient))
	}
	
	a.client = arkruntime.NewClientWithAkSk(config.AccessKey, config.SecretKey, opts...)
	return a.client
}

func (a *ArkAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	client := a.getClient(config)

	req := model.ChatCompletionRequest{
		Model:    request.Model,
		Messages: make([]*model.ChatCompletionMessage, len(request.Messages)),
	}
	for i, m := range request.Messages {
		req.Messages[i] = &model.ChatCompletionMessage{
			Role:    m.Role,
			Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(fmt.Sprint(m.Content))},
		}
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	res := &dto.ChatResponse{
		Choices: make([]dto.ChatChoice, len(resp.Choices)),
		Usage: dto.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	for i, c := range resp.Choices {
		content := ""
		if c.Message.Content != nil && c.Message.Content.StringValue != nil {
			content = *c.Message.Content.StringValue
		}
		res.Choices[i] = dto.ChatChoice{
			Index: c.Index,
			Message: dto.Message{
				Role:    c.Message.Role,
				Content: content,
			},
			FinishReason: string(c.FinishReason),
		}
	}
	return res, nil
}

type arkStreamWrapper struct {
	reader *utils.ChatCompletionStreamReader
}

func (w *arkStreamWrapper) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.reader.Recv()
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, io.EOF
	}
	
	return &dto.StreamToken{
		Text:  resp.Choices[0].Delta.Content,
		Type:  "text",
		Index: resp.Choices[0].Index,
	}, nil
}

func (w *arkStreamWrapper) Close() error {
	return w.reader.Close()
}

func (a *ArkAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	client := a.getClient(config)

	req := model.ChatCompletionRequest{
		Model:    request.Model,
		Messages: make([]*model.ChatCompletionMessage, len(request.Messages)),
	}
	for i, m := range request.Messages {
		req.Messages[i] = &model.ChatCompletionMessage{
			Role:    m.Role,
			Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(fmt.Sprint(m.Content))},
		}
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	return &arkStreamWrapper{reader: stream}, nil
}

func (a *ArkAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	baseURL := "https://ark.cn-beijing.volces.com/api/v3"
	if config.BaseURL != "" {
		baseURL = strings.TrimRight(config.BaseURL, "/")
	}

	endpoint := baseURL + "/video/generations"
	if request.Type == dto.MediaTypeImage {
		endpoint = baseURL + "/image/generations"
	}

	payload := map[string]interface{}{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if request.Size != "" {
		payload["size"] = request.Size
	}
	for k, v := range request.Extra {
		payload[k] = v
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	
	region := config.Region
	if region == "" { region = "cn-beijing" }
	
	if err := a.signV4(req, config.AccessKey, config.SecretKey, region, "air", body); err != nil {
		return nil, err
	}

	resp, err := config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var res struct {
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
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, fmt.Errorf("ark error: %s (%s)", res.Error.Message, res.Error.Code)
	}

	mediaRes := &dto.MediaResponse{
		Created: res.Created,
		TaskID:  res.TaskID,
	}
	if mediaRes.TaskID == "" { mediaRes.TaskID = res.ID }
	for _, d := range res.Data {
		mediaRes.Data = append(mediaRes.Data, dto.ImageData{URL: d.URL, B64JSON: d.B64JSON})
	}
	if len(mediaRes.Data) > 0 { mediaRes.URL = mediaRes.Data[0].URL }

	return mediaRes, nil
}

func (a *ArkAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	baseURL := "https://ark.cn-beijing.volces.com/api/v3"
	if config.BaseURL != "" {
		baseURL = strings.TrimRight(config.BaseURL, "/")
	}
	url := baseURL + "/tasks/" + taskID

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	region := config.Region
	if region == "" { region = "cn-beijing" }
	
	if err := a.signV4(req, config.AccessKey, config.SecretKey, region, "air", nil); err != nil {
		return nil, err
	}

	resp, err := config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var res struct {
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
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}

	statusRes := &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     res.ID,
			TaskStatus: res.Status,
		},
	}
	if res.Error != nil {
		statusRes.Output.Code = res.Error.Code
		statusRes.Output.Message = res.Error.Message
	}
	if res.Result.Video.URL != "" {
		statusRes.Output.VideoURL = res.Result.Video.URL
	} else if len(res.Result.Images) > 0 {
		statusRes.Output.VideoURL = res.Result.Images[0].URL
	}

	return statusRes, nil
}

func (a *ArkAdaptor) signV4(req *http.Request, ak, sk, region, service string, body []byte) error {
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
