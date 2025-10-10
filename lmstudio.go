package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/pkg/robusthttp"
)

type LMStudioClient struct {
	host   string
	httpc  *http.Client
	logger *slog.Logger
}
type ResponseSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *JSONSchemaWrap `json:"json_schema,omitempty"`
}

type JSONSchemaWrap struct {
	Name   string         `json:"name"`
	Schema ResponseSchema `json:"schema"`
	Strict bool           `json:"strict"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

var (
	schema = ResponseSchema{
		Type: "object",
		Properties: map[string]Property{
			"bad_faith": {
				Type:        "boolean",
				Description: "Whether the reply to the parent is bad faith or not.",
			},
		},
		Required: []string{"bad_faith"},
	}
)

func NewLMStudioClient(host string, logger *slog.Logger) *LMStudioClient {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "lmstudio")
	httpc := robusthttp.NewClient()
	return &LMStudioClient{
		host:   host,
		httpc:  httpc,
		logger: logger,
	}
}

func (c *LMStudioClient) sendChatRequest(request ChatRequest) (*ChatResponse, error) {
	url := fmt.Sprintf("%s/v1/chat/completions", c.host)

	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d - %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &chatResp, nil
}

func (c *LMStudioClient) GetIsBadFaith(ctx context.Context, parent, reply string) (bool, error) {
	request := ChatRequest{
		Model: "google/gemma-3-27b",
		Messages: []Message{
			{
				Role:    "system",
				Content: "You are an observer of posts on a microblogging website. You determine if the second message provided by the user is a bad faith reply to the second message provided to you. Opposing viewpoints are good, and should be appreciated. However, things that are toxic, trollish, or offer no good value to the conversation are considered bad faith.",
			},
			{
				Role:    "user",
				Content: parent,
			},
			{
				Role:    "user",
				Content: reply,
			},
		},
		Temperature: 0.7,
		MaxTokens:   50,
		ResponseFormat: &ResponseFormat{
			Type: "json_schema",
			JSONSchema: &JSONSchemaWrap{
				Name:   "message_classification",
				Schema: schema,
				Strict: true,
			},
		},
	}
	response, err := c.sendChatRequest(request)
	if err != nil {
		return false, fmt.Errorf("failed to get chat response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &result); err != nil {
		return false, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	badFaith, ok := result["bad_faith"].(bool)
	if !ok {
		return false, fmt.Errorf("model gave bad response, not structured")
	}

	return badFaith, nil
}
