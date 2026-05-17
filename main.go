// Minimal multi-agent demo in Go using the Gemini API.
//
// See README.md for the conceptual walk-through. The short version: this
// program builds an orchestrator agent that delegates work to a researcher
// and a writer. Each sub-agent is just another *Agent — exposed to its
// parent as a tool.
//
// Run with:
//
//	export GEMINI_API_KEY=...
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/genai"
)

func main() {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("set GEMINI_API_KEY")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	orchestrator := NewOrchestrator(geminiGenerate(client))

	question := "What is the Gemini API and when would you use it?"
	fmt.Printf("user: %s\n", question)

	answer, err := orchestrator.Run(ctx, question)
	if err != nil {
		log.Fatalf("agent error: %v", err)
	}

	fmt.Printf("\n=== final answer ===\n%s\n", answer)
}
