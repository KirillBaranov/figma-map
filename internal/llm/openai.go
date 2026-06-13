// Package llm wraps an OpenAI-compatible chat client for vision calls that
// return structured JSON. Because the base URL is configurable, the same client
// works against OpenAI, the kb-labs gateway, or a local Ollama server.
package llm

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// Image is one image input to a vision call.
type Image struct {
	// Label is an optional text marker emitted just before the image so the
	// model can reference it (e.g. a candidate id).
	Label string
	// PNG is the raw image data.
	PNG []byte
}

// VisionModel is the seam the rest of the app depends on: send a prompt plus
// images and decode a schema-conforming JSON object into out. Implemented by
// *Client; mocked in tests.
type VisionModel interface {
	VisionJSON(ctx context.Context, prompt string, images []Image, schemaName string, out any) error
}

// Client is a vision-oriented wrapper over the OpenAI chat completions API.
type Client struct {
	api   *openai.Client
	model string
}

// Options configures a Client.
type Options struct {
	APIKey  string
	BaseURL string // empty = default OpenAI endpoint
	Model   string
}

// New constructs a Client. The model must be vision-capable and support
// structured outputs (e.g. gpt-4o-mini).
func New(opts Options) *Client {
	cfg := openai.DefaultConfig(opts.APIKey)
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}
	return &Client{api: openai.NewClientWithConfig(cfg), model: opts.Model}
}

// VisionJSON sends prompt+images and decodes the model's structured-output JSON
// into out (a pointer to a struct). The response is constrained to a JSON schema
// derived from out's type, so the model returns conforming JSON — no parsing of
// free-form text. Temperature is 0 for reproducibility; images use detail:low.
func (c *Client) VisionJSON(ctx context.Context, prompt string, images []Image, schemaName string, out any) error {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("VisionJSON: out must be a non-nil pointer, got %T", out)
	}
	schema, err := jsonschema.GenerateSchemaForType(rv.Elem().Interface())
	if err != nil {
		return fmt.Errorf("VisionJSON: build schema: %w", err)
	}

	parts := []openai.ChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: prompt}}
	for _, img := range images {
		if img.Label != "" {
			parts = append(parts, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: img.Label})
		}
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(img.PNG)
		parts = append(parts, openai.ChatMessagePart{
			Type:     openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{URL: dataURL, Detail: openai.ImageURLDetailLow},
		})
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:       c.model,
		Temperature: 0,
		MaxTokens:   1024,
		Messages:    []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, MultiContent: parts}},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   schemaName,
				Schema: schema,
				Strict: true,
			},
		},
	}

	resp, err := c.chatWithRetry(ctx, req)
	if err != nil {
		return fmt.Errorf("vision call: %w", err)
	}
	if len(resp.Choices) == 0 {
		return fmt.Errorf("vision call: empty response")
	}
	return schema.Unmarshal(resp.Choices[0].Message.Content, out)
}

// maxAttempts is the total number of chat attempts (1 try + retries).
const maxAttempts = 4

// chatWithRetry calls the API, retrying transient failures (rate limits, 5xx,
// network) with exponential backoff. Non-transient errors return immediately.
func (c *Client) chatWithRetry(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second // 1s, 2s, 4s
			select {
			case <-ctx.Done():
				return openai.ChatCompletionResponse{}, ctx.Err()
			case <-time.After(backoff):
			}
		}
		resp, err := c.api.CreateChatCompletion(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil || !retryable(err) {
			return openai.ChatCompletionResponse{}, err
		}
	}
	return openai.ChatCompletionResponse{}, lastErr
}

// retryable reports whether an API error is worth retrying.
func retryable(err error) bool {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 408, 429, 500, 502, 503, 504:
			return true
		}
		return false
	}
	// Network/transport errors (no HTTP status) are transient.
	var reqErr *openai.RequestError
	return errors.As(err, &reqErr)
}
