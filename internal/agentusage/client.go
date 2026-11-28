package agentusage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agentflow/internal/domain"
)

const ingestPath = "/api/insights/v1/ingest"

// IngestResult is the platform's answer for one accepted payload.
type IngestResult string

const (
	ResultAccepted  IngestResult = "accepted"
	ResultDuplicate IngestResult = "duplicate"
)

// APIError is a refused report. StatusCode tells the caller whether the run is
// doomed (bad credential, bad payload) or can continue (a changed session).
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("server refused the report with %d: %s", e.StatusCode, e.Message)
}

// Fatal reports whether retrying the same run cannot succeed.
func (e *APIError) Fatal() bool {
	return e.StatusCode == http.StatusBadRequest ||
		e.StatusCode == http.StatusUnauthorized ||
		e.StatusCode == http.StatusForbidden
}

// Conflict reports an execution the platform already has with other content.
func (e *APIError) Conflict() bool { return e.StatusCode == http.StatusConflict }

// Client posts executions to the platform's ingest endpoint.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient builds an ingest client. baseURL is the platform root, for example
// http://127.0.0.1:8090.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Ingest posts one execution. A 200 means the platform already counted it.
func (c *Client) Ingest(ctx context.Context, execution domain.InsightExecution) (IngestResult, error) {
	body, err := json.Marshal(execution)
	if err != nil {
		return "", fmt.Errorf("encode execution: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+ingestPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build ingest request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("post execution: %w", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	switch response.StatusCode {
	case http.StatusCreated:
		return ResultAccepted, nil
	case http.StatusOK:
		return ResultDuplicate, nil
	}
	var reply struct {
		Error string `json:"error"`
	}
	message := strings.TrimSpace(string(payload))
	if err := json.Unmarshal(payload, &reply); err == nil && strings.TrimSpace(reply.Error) != "" {
		message = reply.Error
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return "", &APIError{StatusCode: response.StatusCode, Message: message}
}
