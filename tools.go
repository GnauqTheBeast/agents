package main

import (
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// -----------------------------------------------------------------------------
// Leaf tools. Each is a plain Go function. The orchestrator's children call
// them. The schemas at the bottom describe each tool to the LLM.
// -----------------------------------------------------------------------------

func getWeather(location string) string {
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

// search is a fake "internet". It does substring matching on lowercased
// queries against a small canned table. Swap in a real API later (Tavily,
// Brave, DuckDuckGo) without changing anything else.
var fakeSearchIndex = map[string]string{
	"gemini api": "The Gemini API is Google's interface to the Gemini family of multimodal LLMs (text, vision, audio). It exposes generateContent, streaming, and function-calling.",
	"agent":      "An AI agent is a loop: LLM proposes an action (tool call or final answer), code runs the tool, result is fed back. Repeat until the LLM produces a final answer.",
	"go":         "Go is a statically typed, compiled language by Google designed for simplicity, concurrency (goroutines, channels), and fast builds.",
	"hanoi":      "Hanoi is the capital of Vietnam, in the Red River delta. Known for Old Quarter, pho, egg coffee.",
}

func search(query string) string {
	q := strings.ToLower(query)
	for key, val := range fakeSearchIndex {
		if strings.Contains(q, key) {
			return val
		}
	}
	return fmt.Sprintf("no results for %q (try: gemini api, agent, go, hanoi)", query)
}

// -----------------------------------------------------------------------------
// Tool schemas as package vars so any role can pick which leaves it exposes.
// -----------------------------------------------------------------------------

var searchSchema = &genai.FunctionDeclaration{
	Name:        "search",
	Description: "Search a small in-memory knowledge base. Returns a short snippet for the query.",
	Parameters: &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"query": {Type: genai.TypeString, Description: "Search query. Short keywords work best."},
		},
		Required: []string{"query"},
	},
}

var weatherSchema = &genai.FunctionDeclaration{
	Name:        "get_weather",
	Description: "Get the current weather for a given location.",
	Parameters: &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"location": {Type: genai.TypeString, Description: "City name, e.g. 'Hanoi' or 'Tokyo'."},
		},
		Required: []string{"location"},
	},
}

var calculatorSchema = &genai.FunctionDeclaration{
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
}
