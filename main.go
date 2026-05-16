// Minimal agentic loop in Go using the Gemini API.
//
// What is an "agent"? At its core, an agent is a LOOP:
//
//  1. Send the conversation-so-far + tool schemas to the LLM.
//  2. The LLM either replies with TEXT (we're done) or asks to CALL A TOOL.
//  3. If it called a tool, run the tool, append the result to the
//     conversation, and loop back to step 1.
//
// That's it. Every "AI agent" you've heard of is built on this loop.
// This file implements it in ~150 lines so you can read every step.
//
// Run with:
//
//	export GEMINI_API_KEY=...
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/genai"
)

// -----------------------------------------------------------------------------
// 1. THE TOOLS the agent can use.
//
// Each tool is just a regular Go function. We also describe its shape to the
// LLM via a "function declaration" (the schemas below) so the LLM knows
// (a) what tools exist and (b) what arguments each tool takes.
// -----------------------------------------------------------------------------

func getWeather(location string) string {
	// In a real agent this would call a weather API. For learning, we fake it.
	return fmt.Sprintf("%s: 32°C, sunny, light breeze", location)
}

func calculator(op string, a, b float64) string {
	switch op {
	case "+":
		return fmt.Sprintf("%v", a+b)
	case "-":
		return fmt.Sprintf("%v", a-b)
	case "*":
		return fmt.Sprintf("%v", a*b)
	case "/":
		if b == 0 {
			return "error: divide by zero"
		}
		return fmt.Sprintf("%v", a/b)
	default:
		return "error: unknown operator (use +, -, *, /)"
	}
}

// toolSchemas describes the tools to the LLM. The LLM uses these schemas to
// decide WHICH tool to call and WHAT arguments to pass.
var toolSchemas = []*genai.Tool{{
	FunctionDeclarations: []*genai.FunctionDeclaration{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a given location.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"location": {
						Type:        genai.TypeString,
						Description: "City name, e.g. 'Hanoi' or 'Tokyo'.",
					},
				},
				Required: []string{"location"},
			},
		},
		{
			Name:        "calculator",
			Description: "Perform arithmetic on two numbers.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"op": {Type: genai.TypeString, Description: "Operator: + - * /"},
					"a":  {Type: genai.TypeNumber, Description: "First number."},
					"b":  {Type: genai.TypeNumber, Description: "Second number."},
				},
				Required: []string{"op", "a", "b"},
			},
		},
	},
}}

// dispatchTool is the bridge between the LLM's "intent" (a function-call
// request) and actual code execution. Given a tool name + args, run it.
func dispatchTool(name string, args map[string]any) string {
	switch name {
	case "get_weather":
		return getWeather(args["location"].(string))
	case "calculator":
		// JSON numbers always arrive in Go as float64.
		return calculator(args["op"].(string), args["a"].(float64), args["b"].(float64))
	default:
		return fmt.Sprintf("error: unknown tool %q", name)
	}
}

// -----------------------------------------------------------------------------
// 2. THE AGENT LOOP.
//
// Two key things to watch:
//   - `history` is the running conversation. Every turn APPENDS to it; we
//     never throw anything away. The model needs full context each call.
//   - The loop ends when the model replies with plain text and no tool call.
// -----------------------------------------------------------------------------

func runAgent(ctx context.Context, client *genai.Client, userQuestion string) error {
	// Start the conversation with the user's question.
	history := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: userQuestion}}},
	}

	// SAFETY CAP: never let an agent loop forever. Real agents always bound
	// themselves — by iterations, tokens, wall time, or dollars spent.
	const maxIterations = 5

	for i := 0; i < maxIterations; i++ {
		fmt.Printf("\n--- turn %d: calling Gemini ---\n", i+1)

		// (a) Ask the model what to do next, given the conversation so far.
		resp, err := client.Models.GenerateContent(
			ctx,
			"gemini-2.5-flash",
			history,
			&genai.GenerateContentConfig{Tools: toolSchemas},
		)
		if err != nil {
			return fmt.Errorf("gemini call failed: %w", err)
		}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return fmt.Errorf("no response from model")
		}
		modelMessage := resp.Candidates[0].Content

		// Append the model's reply to history. This is critical: the model
		// needs to "remember" what it just decided, so its NEXT call sees it.
		history = append(history, modelMessage)

		// (b) Inspect the reply. It contains "parts" — each part is either
		// plain text (the agent is talking to us) or a function call (the
		// agent wants to use a tool). One reply can contain MULTIPLE tool
		// calls — the model can ask for several tools in one go.
		var toolResults []*genai.Part
		var sawText bool

		for _, part := range modelMessage.Parts {
			switch {
			case part.FunctionCall != nil:
				fc := part.FunctionCall
				fmt.Printf("model wants to call: %s(%v)\n", fc.Name, fc.Args)

				// Actually run the tool.
				result := dispatchTool(fc.Name, fc.Args)
				fmt.Printf("tool result: %s\n", result)

				// Pack the result as a "function response" part that the
				// model will see on the next turn.
				toolResults = append(toolResults, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name:     fc.Name,
						Response: map[string]any{"result": result},
					},
				})

			case part.Text != "":
				sawText = true
				fmt.Printf("\nmodel: %s\n", part.Text)
			}
		}

		// (c) TERMINATION: if the model produced text and called no tools,
		// the agent is done — it has answered the user's question.
		if len(toolResults) == 0 && sawText {
			return nil
		}

		// Otherwise: feed tool results back as a new "user" turn and loop.
		// (Gemini's convention: tool responses are sent with role="user".)
		history = append(history, &genai.Content{
			Role:  "user",
			Parts: toolResults,
		})
	}

	return fmt.Errorf("agent did not finish within %d iterations", maxIterations)
}

// -----------------------------------------------------------------------------
// 3. ENTRY POINT — wires everything together.
// -----------------------------------------------------------------------------

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

	question := "What's the weather in Hanoi and BacNinh, and what is pi * 10?"
	fmt.Printf("user: %s\n", question)

	if err := runAgent(ctx, client, question); err != nil {
		log.Fatalf("agent error: %v", err)
	}
}
