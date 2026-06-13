// Package llm wraps an OpenAI-compatible chat client for vision calls. Because
// the base URL is configurable, the same client works against OpenAI, the
// kb-labs gateway, or a local Ollama/llava server.
package llm

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Client is a thin vision-oriented wrapper over the OpenAI chat completions API.
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

// New constructs a Client. The model must be vision-capable.
func New(opts Options) *Client {
	cfg := openai.DefaultConfig(opts.APIKey)
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}
	return &Client{api: openai.NewClientWithConfig(cfg), model: opts.Model}
}

// Image is one image input to a vision call.
type Image struct {
	// Label is an optional text marker emitted just before the image so the
	// model can reference it (e.g. a candidate filename).
	Label string
	// PNG is the raw image data.
	PNG []byte
}

// Vision sends a prompt plus images and returns the model's text reply. All
// images use detail:"low" to keep token usage bounded, and temperature is 0
// for reproducibility.
func (c *Client) Vision(ctx context.Context, prompt string, images []Image) (string, error) {
	parts := []openai.ChatMessagePart{
		{Type: openai.ChatMessagePartTypeText, Text: prompt},
	}
	for _, img := range images {
		if img.Label != "" {
			parts = append(parts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeText,
				Text: img.Label,
			})
		}
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(img.PNG)
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL:    dataURL,
				Detail: openai.ImageURLDetailLow,
			},
		})
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	resp, err := c.api.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Temperature: 0,
		MaxTokens:   1024,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, MultiContent: parts},
		},
	})
	if err != nil {
		return "", fmt.Errorf("vision call: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("vision call: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
