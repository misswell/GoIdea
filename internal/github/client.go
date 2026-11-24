package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultGraphQLEndpoint = "https://api.github.com/graphql"
	defaultAPIBase         = "https://api.github.com"
	defaultPageSize        = 50
	defaultMaxPages        = 5
	maxResponseBytes       = 16 << 20
)

// ClientOptions configures transport and endpoints. The zero value uses
// github.com and a bounded 30-second HTTP client.
type ClientOptions struct {
	HTTPClient      *http.Client
	GraphQLEndpoint string
	APIBase         string
	PageSize        int
	MaxPages        int
	UserAgent       string
}

// Client talks to GitHub's GraphQL and REST APIs with a token read from the
// OS credential store for each request.
type Client struct {
	credentials     Credentials
	http            *http.Client
	graphqlEndpoint string
	apiBase         string
	pageSize        int
	maxPages        int
	userAgent       string
}

// NewClient returns a GitHub client. It panics only for a nil credential
// source, which is a programming error.
func NewClient(credentials Credentials, options ClientOptions) *Client {
	if credentials == nil {
		panic("github: nil credentials")
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	graphqlEndpoint := strings.TrimRight(options.GraphQLEndpoint, "/")
	if graphqlEndpoint == "" {
		graphqlEndpoint = defaultGraphQLEndpoint
	}
	apiBase := strings.TrimRight(options.APIBase, "/")
	if apiBase == "" {
		apiBase = defaultAPIBase
	}
	pageSize := options.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = defaultPageSize
	}
	maxPages := options.MaxPages
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	userAgent := strings.TrimSpace(options.UserAgent)
	if userAgent == "" {
		userAgent = "GoIDEA"
	}
	return &Client{
		credentials: credentials, http: httpClient,
		graphqlEndpoint: graphqlEndpoint, apiBase: apiBase,
		pageSize: pageSize, maxPages: maxPages, userAgent: userAgent,
	}
}

// APIError is a non-success GitHub response. Its message is bounded and has
// the current token removed before it reaches the UI.
type APIError struct {
	Method   string
	Endpoint string
	Status   int
	Message  string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("GitHub %s %s: %s", e.Method, e.Endpoint, e.Message)
	}
	return fmt.Sprintf("GitHub %s %s returned HTTP %d", e.Method, e.Endpoint, e.Status)
}

func (c *Client) GraphQL(ctx context.Context, query string, variables any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	body, token, err := c.do(ctx, http.MethodPost, c.graphqlEndpoint, payload)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("GitHub returned invalid JSON: %w", err)
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, item := range envelope.Errors {
			if message := strings.TrimSpace(item.Message); message != "" {
				messages = append(messages, message)
			}
		}
		if len(messages) == 0 {
			messages = append(messages, "unknown GraphQL error")
		}
		return &APIError{
			Method: http.MethodPost, Endpoint: "/graphql",
			Message: redact(strings.Join(messages, "\n"), token),
		}
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		return errors.New("GitHub returned no GraphQL data")
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("GitHub returned an unexpected GraphQL response: %w", err)
	}
	return nil
}

func (c *Client) REST(ctx context.Context, method, endpoint string, input, out any) error {
	var payload []byte
	var err error
	if input != nil {
		payload, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	body, _, err := c.do(ctx, method, c.apiBase+"/"+strings.TrimLeft(endpoint, "/"), payload)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("GitHub returned an unexpected REST response: %w", err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, payload []byte) ([]byte, string, error) {
	token, err := c.credentials.Token(ctx)
	if err != nil {
		return nil, "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, "", ErrNoToken
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("build GitHub request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("GitHub request failed: %w", err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, "", fmt.Errorf("read GitHub response: %w", readErr)
	}
	if len(body) > maxResponseBytes {
		return nil, "", errors.New("GitHub response exceeded 16 MiB")
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return body, token, nil
	}
	message := githubErrorMessage(body)
	message = redact(message, token)
	if message == "" {
		message = strings.TrimSpace(response.Status)
	}
	if len(message) > 600 {
		message = message[:600] + "…"
	}
	apiErr := &APIError{
		Method: method, Endpoint: requestURLPath(endpoint),
		Status: response.StatusCode, Message: message,
	}
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return nil, "", fmt.Errorf("%w: %v", ErrUnauthorized, apiErr)
	case http.StatusNotFound:
		return nil, "", fmt.Errorf("%w: %v", ErrNotFound, apiErr)
	default:
		return nil, "", apiErr
	}
}

func githubErrorMessage(body []byte) string {
	var value struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &value) == nil {
		lines := []string{}
		if strings.TrimSpace(value.Message) != "" {
			lines = append(lines, strings.TrimSpace(value.Message))
		}
		for _, item := range value.Errors {
			message := strings.TrimSpace(item.Message)
			if message == "" {
				message = strings.TrimSpace(item.Code)
			}
			if message != "" {
				lines = append(lines, message)
			}
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}
	return strings.Join(strings.Fields(string(body)), " ")
}

func redact(message, token string) string {
	if token == "" {
		return message
	}
	return strings.ReplaceAll(message, token, "[redacted token]")
}

func requestURLPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "/"
	}
	if u.Path == "" {
		return "/"
	}
	return u.Path
}
