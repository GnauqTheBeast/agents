package main

import (
	"context"
	"errors"
	"fmt"
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
	if got != "hello world" {
		t.Fatalf("expected final text %q, got %q", "hello world", got)
	}
}

func funcCallResp(name string, args map[string]any) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{Name: name, Args: args},
				}},
			},
		}},
	}
}

func TestRunLoopsOnFunctionCall(t *testing.T) {
	var captured map[string]any
	dispatch := func(_ context.Context, name string, args map[string]any) string {
		if name != "echo" {
			t.Fatalf("unexpected tool %q", name)
		}
		captured = args
		return "echoed:" + args["msg"].(string)
	}

	agent := &Agent{
		Name:          "test",
		Model:         "fake",
		MaxIterations: 5,
		Dispatch:      dispatch,
		Generate: scriptedGenerate([]*genai.GenerateContentResponse{
			funcCallResp("echo", map[string]any{"msg": "hi"}),
			textResp("done"),
		}),
	}

	got, err := agent.Run(context.Background(), "use echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "done" {
		t.Fatalf("expected final text %q, got %q", "done", got)
	}
	if captured["msg"] != "hi" {
		t.Fatalf("expected dispatch to receive msg=hi, got %v", captured)
	}
}

func TestRunErrorsAtIterationCap(t *testing.T) {
	// Every response calls a tool — the loop never sees a text-only reply.
	dispatch := func(_ context.Context, _ string, _ map[string]any) string {
		return "noop"
	}
	resps := make([]*genai.GenerateContentResponse, 5)
	for i := range resps {
		resps[i] = funcCallResp("echo", map[string]any{"msg": "x"})
	}

	agent := &Agent{
		Name:          "test",
		Model:         "fake",
		MaxIterations: 3,
		Dispatch:      dispatch,
		Generate:      scriptedGenerate(resps),
	}

	_, err := agent.Run(context.Background(), "loop forever")
	if err == nil {
		t.Fatalf("expected iteration-cap error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected error to mention 'exceeded', got %q", err.Error())
	}
}

func TestUnknownToolReturnsErrorString(t *testing.T) {
	// Dispatch only knows "echo". Model calls "ghost" — dispatch returns an
	// error string, the loop must continue rather than crash, and the next
	// model response (plain text) should terminate normally.
	dispatch := func(_ context.Context, name string, _ map[string]any) string {
		if name == "echo" {
			return "ok"
		}
		return fmt.Sprintf("error: unknown tool %q", name)
	}

	agent := &Agent{
		Name:          "test",
		Model:         "fake",
		MaxIterations: 5,
		Dispatch:      dispatch,
		Generate: scriptedGenerate([]*genai.GenerateContentResponse{
			funcCallResp("ghost", map[string]any{}),
			textResp("recovered"),
		}),
	}

	got, err := agent.Run(context.Background(), "trigger unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "recovered" {
		t.Fatalf("expected final text %q, got %q", "recovered", got)
	}
}
