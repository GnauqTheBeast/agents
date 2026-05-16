package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/genai"
)

// scriptedGenerate returns a generateFunc that yields the given responses
// in order, then errors if called again.
func scriptedGenerate(responses []*genai.GenerateContentResponse) generateFunc {
	i := 0
	return func(
		_ context.Context,
		_ string,
		_ []*genai.Content,
		_ *genai.GenerateContentConfig,
	) (*genai.GenerateContentResponse, error) {
		if i >= len(responses) {
			return nil, errors.New("scripted generate: out of responses")
		}
		r := responses[i]
		i++
		return r, nil
	}
}

func textResp(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: text}}},
		}},
	}
}

func TestRunTerminatesOnText(t *testing.T) {
	agent := &Agent{
		Name:          "test",
		Model:         "fake",
		MaxIterations: 3,
		Generate:      scriptedGenerate([]*genai.GenerateContentResponse{textResp("hello world")}),
	}

	got, err := agent.Run(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "hello world") {
		t.Fatalf("expected final text to contain %q, got %q", "hello world", got)
	}
}
