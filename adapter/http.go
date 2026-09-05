package adapter

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func providerHTTPClient(cfg *ProviderConfig) (*http.Client, error) {
	client := &http.Client{}
	if cfg.HTTPClient != nil {
		*client = *cfg.HTTPClient
	}
	if cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	if cfg.Proxy == "" {
		return client, nil
	}
	proxy := cfg.Proxy
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil || proxyURL.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL")
	}
	switch proxyURL.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	transport, ok := baseTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("proxy requires an *http.Transport; configure the proxy on the custom transport instead")
	}
	cloned := transport.Clone()
	cloned.Proxy = http.ProxyURL(proxyURL)
	client.Transport = cloned
	return client, nil
}

// Keep retry policy outside the SDK so raw and SDK API requests use the same settings.
type retryHTTPClient struct {
	client     *http.Client
	maxRetries int
	retryDelay time.Duration
}

func (c *retryHTTPClient) Do(request *http.Request) (*http.Response, error) {
	current := request
	for attempt := 0; ; attempt++ {
		response, err := c.client.Do(current)
		if request.Context().Err() != nil {
			if response != nil {
				response.Body.Close()
			}
			return nil, request.Context().Err()
		}
		if attempt >= c.maxRetries || (request.Body != nil && request.GetBody == nil) || !retryableResponse(response, err) {
			return response, err
		}
		delay := c.delay(attempt, response)
		if response != nil {
			io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			response.Body.Close()
		}
		timer := time.NewTimer(delay)
		select {
		case <-request.Context().Done():
			timer.Stop()
			return nil, request.Context().Err()
		case <-timer.C:
		}
		current = request.Clone(request.Context())
		if request.Body != nil {
			current.Body, err = request.GetBody()
			if err != nil {
				return nil, err
			}
		}
	}
}

func retryableResponse(response *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if response == nil {
		return false
	}
	switch response.Header.Get("X-Should-Retry") {
	case "true":
		return true
	case "false":
		return false
	}
	return response.StatusCode == 408 || response.StatusCode == 409 || response.StatusCode == 429 || response.StatusCode >= 500
}

func (c *retryHTTPClient) delay(attempt int, response *http.Response) time.Duration {
	if response != nil {
		if milliseconds, err := strconv.ParseFloat(response.Header.Get("Retry-After-Ms"), 64); err == nil && milliseconds >= 0 && milliseconds <= 60000 {
			return time.Duration(milliseconds * float64(time.Millisecond))
		}
		value := response.Header.Get("Retry-After")
		if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 && seconds <= 60 {
			return time.Duration(seconds * float64(time.Second))
		}
		if deadline, err := http.ParseTime(value); err == nil {
			if delay := time.Until(deadline); delay >= 0 && delay <= time.Minute {
				return delay
			}
		}
	}
	delay := c.retryDelay
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}
	for i := 0; i < attempt && delay < 30*time.Second; i++ {
		delay *= 2
	}
	return min(delay, 30*time.Second)
}
