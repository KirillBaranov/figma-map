package llm

import (
	"errors"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limit", &openai.APIError{HTTPStatusCode: 429}, true},
		{"server error", &openai.APIError{HTTPStatusCode: 503}, true},
		{"timeout", &openai.APIError{HTTPStatusCode: 408}, true},
		{"bad request", &openai.APIError{HTTPStatusCode: 400}, false},
		{"unauthorized", &openai.APIError{HTTPStatusCode: 401}, false},
		{"network", &openai.RequestError{HTTPStatusCode: 0, Err: errors.New("dial tcp")}, true},
		{"plain error", errors.New("boom"), false},
	}
	for _, c := range cases {
		if got := retryable(c.err); got != c.want {
			t.Errorf("%s: retryable=%v, want %v", c.name, got, c.want)
		}
	}
}
