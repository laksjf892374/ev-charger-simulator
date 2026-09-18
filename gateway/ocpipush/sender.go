package ocpipush

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"cposim/gateway/clock"
	"cposim/gateway/trace"
)

const (
	maxRecordedBodyBytes = 16 * 1024
	sendTimeout          = 5 * time.Second
)

type Push struct {
	Body    any
	Method  string
	Summary string
	URL     string
}

// Sender delivers one push. It is the seam for link-level behaviour: retries, or injected
// delivery faults (duplicate, delay, drop), would be decorators around the HTTP sender.
type Sender interface {
	Send(push Push) error
}

type httpSender struct {
	clockGateway clock.Gateway
	httpClient   *http.Client
	traceGateway trace.Gateway
}

func NewHTTPSender(
	clockGateway clock.Gateway,
	traceGateway trace.Gateway,
) Sender {
	return httpSender{
		clockGateway: clockGateway,
		httpClient:   &http.Client{Timeout: sendTimeout},
		traceGateway: traceGateway,
	}
}

func (s httpSender) Send(push Push) error {
	requestBody, err := json.Marshal(push.Body)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	entry := trace.Entry{
		Direction:   trace.DirectionOutbound,
		Method:      push.Method,
		RecordedAt:  s.clockGateway.Now(),
		RequestBody: string(requestBody),
		Summary:     push.Summary,
		URL:         push.URL,
	}

	statusCode, responseBody, err := s.do(push, requestBody)
	entry.ResponseBody = responseBody
	entry.StatusCode = statusCode
	if err != nil {
		entry.ResponseBody = err.Error()
	}

	s.traceGateway.Record(entry)

	if err != nil {
		return fmt.Errorf("do: %w", err)
	}

	return nil
}

func (s httpSender) do(push Push, requestBody []byte) (int, string, error) {
	// the URL of a command result comes from the eMSP's request, so it is not trusted
	parsedURL, err := url.Parse(push.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return 0, "", fmt.Errorf("push URL %q is not an http(s) URL", push.URL)
	}

	request, err := http.NewRequest(push.Method, push.URL, bytes.NewReader(requestBody))
	if err != nil {
		return 0, "", fmt.Errorf("http.NewRequest: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return 0, "", fmt.Errorf("httpClient.Do: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxRecordedBodyBytes))
	if err != nil {
		return response.StatusCode, "", fmt.Errorf("io.ReadAll: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return response.StatusCode, string(responseBody), fmt.Errorf("eMSP answered HTTP %d", response.StatusCode)
	}

	return response.StatusCode, string(responseBody), nil
}
