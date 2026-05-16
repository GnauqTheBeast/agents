// Package main — see README.md for the conceptual overview.
//
// agent.go defines the generic agent loop. Every role in this program
// (orchestrator, researcher, writer) is an *Agent with a different
// system prompt, tool set, and dispatch map.
package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// generateFunc is the one Gemini call we make per turn, abstracted so tests
// can script responses without hitting the network. geminiGenerate adapts
// a real *genai.Client to this shape.
type generateFunc func(
	ctx context.Context,
	model string,
	history []*genai.Content,
	cfg *genai.GenerateContentConfig,
) (*genai.GenerateContentResponse, error)

// Agent is the universal shape. Roles are data: different prompt + tools +
// dispatch = different agent.
type Agent struct {
	Name          string
	Model         string
	SystemPrompt  string
	Tools         []*genai.Tool
	Dispatch      func(ctx context.Context, name string, args map[string]any) string
	MaxIterations int
	Depth         int // for indented logging; 0 = top-level

	Generate generateFunc
}

// Run executes the agent loop until the model replies with plain text (no
// tool calls) or MaxIterations is exceeded. Returns the model's final text.
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

// geminiGenerate adapts a *genai.Client into a generateFunc.
func geminiGenerate(client *genai.Client) generateFunc {
	return func(
		ctx context.Context,
		model string,
		history []*genai.Content,
		cfg *genai.GenerateContentConfig,
	) (*genai.GenerateContentResponse, error) {
		return client.Models.GenerateContent(ctx, model, history, cfg)
	}
}

// runOrErr collapses (string, error) into the single string the LLM expects
// as a tool result. On error it returns "error: <err>" so the parent agent
// sees the failure as text and can decide how to react.
func runOrErr(s string, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return s
}

// logf prints with a name prefix and indent matching nesting depth.
func (a *Agent) logf(format string, args ...any) {
	indent := strings.Repeat("  ", a.Depth)
	fmt.Printf("%s[%s] %s\n", indent, a.Name, fmt.Sprintf(format, args...))
}
