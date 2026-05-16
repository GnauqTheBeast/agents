# Multi-agent Upgrade — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-file Gemini agent with a multi-agent system (orchestrator + researcher + writer) using the sub-agent-as-tool pattern, while preserving the learning-project feel (small, readable, well-commented).

**Architecture:** One generic `Agent` struct (`agent.go`). Roles are instances built by factories in `roles.go` with role-specific system prompts, tool sets, and dispatch maps. Sub-agents are exposed to their parent as ordinary tools — the parent's dispatch map calls the child's `Run` method. Leaf tools (`search`, `calculator`, `get_weather`) live in `tools.go`. Entry point `main.go` is ~30 lines of wiring. The Gemini `GenerateContent` call is abstracted behind a `generateFunc` field on `Agent` so unit tests can script LLM responses without hitting the network.

**Tech Stack:** Go 1.23, `google.golang.org/genai` v0.7.0, standard testing (`testing` package — no external test framework).

**Reference spec:** `docs/superpowers/specs/2026-05-16-multi-agent-design.md`

**Repo state:** Existing repo has no commits yet. First task creates the initial commit baseline.

---

## File Map

| File | Status | Responsibility |
|---|---|---|
| `agent.go` | Create | `Agent` struct, `generateFunc` type, `Run` method, `geminiGenerate` adapter, `runOrErr` helper |
| `tools.go` | Create | Leaf tool functions (`getWeather`, `calculator`, `search`) + their `*genai.Tool` schemas as package vars |
| `roles.go` | Create | Factories: `NewOrchestrator`, `NewResearcher`, `NewWriter`; orchestrator delegation tool schemas live here |
| `main.go` | Replace | Entry point: read env, build orchestrator, run query, print result |
| `agent_test.go` | Create | Loop control-flow tests using scripted fake `generateFunc` |
| `tools_test.go` | Create | Unit tests for `calculator` + `search` |
| `README.md` | Create | Reader-facing "what's an agent + how to read this repo" doc |
| `.gitignore` | Modify | Keep existing `.env` rule; no changes expected |

---

## Task 0: Baseline commit ✅ already done

The repo already has two commits:
- `3f0c849 first commit` — original single-file `main.go`, `go.mod`, `go.sum`, `.gitignore`
- `06a6d0a docs: add multi-agent upgrade spec and implementation plan` — this design spec + plan

Skip this task and start at Task 1.

---

## Task 1: Create `agent.go` skeleton with types

**Files:**
- Create: `minimal-agent-go/agent.go`

- [ ] **Step 1: Write `agent.go` with the types and a stub `Run`**

```go
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
```

- [ ] **Step 2: Verify it compiles**

Run: `cd minimal-agent-go && go build ./...`
Expected: build succeeds. The new symbols (`Agent`, `generateFunc`, `geminiGenerate`, `runOrErr`, `logf`) don't clash with the existing `main.go` (whose `getWeather`, `calculator`, `dispatchTool`, `runAgent`, `toolSchemas` are all different names). Conflicts only arise in Task 6 when `tools.go` redefines `getWeather` and `calculator` — they're handled there.

- [ ] **Step 3: Commit (work-in-progress checkpoint)**

```bash
cd minimal-agent-go
git add agent.go
git commit -m "wip: agent struct + generateFunc shape"
```

---

## Task 2: TDD — `Run` terminates on text-only response

**Files:**
- Create: `minimal-agent-go/agent_test.go`
- Modify: `minimal-agent-go/agent.go` (implement minimal text-termination)

- [ ] **Step 1: Write the failing test**

Add `agent_test.go`:

```go
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
```

- [ ] **Step 2: Run the test — verify it fails**

Run: `cd minimal-agent-go && go test -run TestRunTerminatesOnText -v`
Expected: FAIL — `Run` returns the stub error `"not implemented"`.

- [ ] **Step 3: Implement minimal `Run` to pass the test**

Replace the stub body of `Run` in `agent.go`:

```go
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	history := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: userInput}}},
	}

	cfg := &genai.GenerateContentConfig{Tools: a.Tools}
	if a.SystemPrompt != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: a.SystemPrompt}},
		}
	}

	for i := 0; i < a.MaxIterations; i++ {
		a.logf("turn %d: calling Gemini", i+1)

		resp, err := a.Generate(ctx, a.Model, history, cfg)
		if err != nil {
			return "", fmt.Errorf("generate: %w", err)
		}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return "", fmt.Errorf("no candidate in response")
		}
		msg := resp.Candidates[0].Content
		history = append(history, msg)

		var finalText strings.Builder
		sawText := false
		sawCall := false

		for _, part := range msg.Parts {
			if part.Text != "" {
				sawText = true
				finalText.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				sawCall = true
			}
		}

		if sawText && !sawCall {
			a.logf("final: %s", finalText.String())
			return finalText.String(), nil
		}

		// Function calls — not yet handled. Will add in Task 3.
		return "", fmt.Errorf("function calls not yet supported")
	}
	return "", fmt.Errorf("agent %q exceeded %d iterations", a.Name, a.MaxIterations)
}
```

- [ ] **Step 4: Run the test — verify it passes**

Run: `cd minimal-agent-go && go test -run TestRunTerminatesOnText -v`
Expected: PASS. The package compiles because the original `main.go` and the new `agent.go`/`agent_test.go` have no name conflicts at this stage.

- [ ] **Step 5: Commit**

```bash
cd minimal-agent-go
git add agent.go agent_test.go
git commit -m "feat(agent): text-only response terminates the loop"
```

---

## Task 3: TDD — `Run` dispatches function calls and loops

**Files:**
- Modify: `minimal-agent-go/agent_test.go` (add test)
- Modify: `minimal-agent-go/agent.go` (handle function-call parts)

- [ ] **Step 1: Add the failing test**

Append to `agent_test.go`:

```go
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
```

- [ ] **Step 2: Run the test — verify it fails**

Run: `cd minimal-agent-go && go test -run TestRunLoopsOnFunctionCall -v`
Expected: FAIL with "function calls not yet supported".

- [ ] **Step 3: Implement function-call handling**

Replace the body of the `for i := 0; ...` loop in `Run` (in `agent.go`) so the function-call branch dispatches and feeds results back. Final loop body:

```go
		a.logf("turn %d: calling Gemini", i+1)

		resp, err := a.Generate(ctx, a.Model, history, cfg)
		if err != nil {
			return "", fmt.Errorf("generate: %w", err)
		}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return "", fmt.Errorf("no candidate in response")
		}
		msg := resp.Candidates[0].Content
		history = append(history, msg)

		var finalText strings.Builder
		var toolResults []*genai.Part
		sawText := false

		for _, part := range msg.Parts {
			switch {
			case part.FunctionCall != nil:
				fc := part.FunctionCall
				a.logf("  wants to call: %s(%v)", fc.Name, fc.Args)
				result := a.Dispatch(ctx, fc.Name, fc.Args)
				a.logf("  result: %s", result)
				toolResults = append(toolResults, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name:     fc.Name,
						Response: map[string]any{"result": result},
					},
				})
			case part.Text != "":
				sawText = true
				finalText.WriteString(part.Text)
			}
		}

		if len(toolResults) == 0 && sawText {
			a.logf("final: %s", finalText.String())
			return finalText.String(), nil
		}

		history = append(history, &genai.Content{Role: "user", Parts: toolResults})
```

Also delete the leftover `return "", fmt.Errorf("function calls not yet supported")` line.

- [ ] **Step 4: Run both tests — verify they pass**

Run: `cd minimal-agent-go && go test -run 'TestRunTerminatesOnText|TestRunLoopsOnFunctionCall' -v`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
cd minimal-agent-go
git add agent.go agent_test.go
git commit -m "feat(agent): dispatch function calls and feed results back"
```

---

## Task 4: TDD — iteration cap returns error

**Files:**
- Modify: `minimal-agent-go/agent_test.go`

- [ ] **Step 1: Add the failing test**

Append to `agent_test.go`:

```go
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
```

- [ ] **Step 2: Run the test — verify it passes immediately**

Run: `cd minimal-agent-go && go test -run TestRunErrorsAtIterationCap -v`
Expected: PASS — the loop already exits with the `"exceeded N iterations"` error after `MaxIterations` turns.

(TDD note: writing the test first still has value — it proves the cap works as designed and locks in the error-message contract for callers/sub-agent error propagation. If the test fails, fix the loop's terminal `return` to match the assertion.)

- [ ] **Step 3: Commit**

```bash
cd minimal-agent-go
git add agent_test.go
git commit -m "test(agent): iteration cap returns descriptive error"
```

---

## Task 5: TDD — unknown tool from dispatch yields error string, loop continues

**Files:**
- Modify: `minimal-agent-go/agent_test.go`

- [ ] **Step 1: Add the failing test**

Append to `agent_test.go`:

```go
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
```

Add `"fmt"` to the imports of `agent_test.go` if not already present.

- [ ] **Step 2: Run the test — verify it passes**

Run: `cd minimal-agent-go && go test -run TestUnknownToolReturnsErrorString -v`
Expected: PASS. (The loop treats every dispatch return value as a tool result, regardless of content — so the "unknown tool" handling is entirely a *convention* enforced by the dispatch function. This test pins that contract.)

- [ ] **Step 3: Commit**

```bash
cd minimal-agent-go
git add agent_test.go
git commit -m "test(agent): unknown tool from dispatch flows back as text"
```

---

## Task 6: Create `tools.go` with leaf tools + TDD `search`

**Files:**
- Create: `minimal-agent-go/tools.go`
- Create: `minimal-agent-go/tools_test.go`
- Modify: `minimal-agent-go/main.go` (delete the old contents — we'll write the new entry point in Task 8). Leaving an empty main.go briefly is fine, but we instead **rewrite it to a minimal placeholder** to avoid breaking `go build`.

- [ ] **Step 1: Write the failing tool test first**

Create `tools_test.go`:

```go
package main

import "testing"

func TestCalculator(t *testing.T) {
	cases := []struct {
		op   string
		a, b float64
		want string
	}{
		{"+", 1, 2, "3"},
		{"-", 5, 3, "2"},
		{"*", 4, 6, "24"},
		{"/", 10, 2, "5"},
		{"/", 1, 0, "error: divide by zero"},
		{"%", 1, 1, "error: unknown operator (use +, -, *, /)"},
	}
	for _, c := range cases {
		got := calculator(c.op, c.a, c.b)
		if got != c.want {
			t.Errorf("calculator(%q, %v, %v) = %q, want %q", c.op, c.a, c.b, got, c.want)
		}
	}
}

func TestSearch(t *testing.T) {
	got := search("gemini api")
	if got == "" {
		t.Fatal("expected non-empty result for known query")
	}
	miss := search("nothing-matches-this-query-xyz")
	if miss == "" {
		t.Fatal("expected non-empty fallback for unknown query")
	}
}
```

- [ ] **Step 2: Run the tests — verify they fail**

Run: `cd minimal-agent-go && go test -run 'TestCalculator|TestSearch' -v`
Expected: build error — `calculator` and `search` undefined (the new ones haven't been written yet, and the old ones still live in `main.go`).

- [ ] **Step 3: Empty out `main.go` to remove duplicate definitions**

Replace the entire contents of `main.go` with a minimal placeholder so the package still builds:

```go
package main

func main() {}
```

The real entry point is rewritten in Task 8.

- [ ] **Step 4: Create `tools.go`**

```go
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
```

- [ ] **Step 5: Run tests — verify they pass**

Run: `cd minimal-agent-go && go test ./... -v`
Expected: all 6 tests pass (4 agent tests + 2 tool tests). Build succeeds.

- [ ] **Step 6: Commit**

```bash
cd minimal-agent-go
git add tools.go tools_test.go main.go
git commit -m "feat(tools): extract leaf tools + schemas, add fake search"
```

---

## Task 7: Create `roles.go` with the three factories

**Files:**
- Create: `minimal-agent-go/roles.go`

- [ ] **Step 1: Write `roles.go`**

```go
package main

import (
	"context"

	"google.golang.org/genai"
)

// -----------------------------------------------------------------------------
// Role factories. Each returns an *Agent wired with a role-specific system
// prompt, tool set, and dispatch map. This is where you'd add new roles.
// -----------------------------------------------------------------------------

// NewResearcher: gathers facts via the `search` tool, returns a bullet list.
// No sub-agents.
//
// Why this prompt: we forbid prose so the writer gets clean notes. We tell
// the researcher *how many* searches roughly to do so it doesn't bail after one.
func NewResearcher(gen generateFunc, depth int) *Agent {
	return &Agent{
		Name:  "researcher",
		Model: "gemini-2.5-flash",
		SystemPrompt: "You are a research assistant. Use the `search` tool one or " +
			"more times (typically 2-3 searches with different keywords) to gather " +
			"facts about the topic. When you have enough, reply with a concise " +
			"bullet list of findings. Do not write prose paragraphs — leave that " +
			"to the writer.",
		Tools: []*genai.Tool{{
			FunctionDeclarations: []*genai.FunctionDeclaration{searchSchema},
		}},
		MaxIterations: 5,
		Depth:         depth,
		Generate:      gen,
		Dispatch: func(_ context.Context, name string, args map[string]any) string {
			if name == "search" {
				return search(args["query"].(string))
			}
			return "error: unknown tool " + name
		},
	}
}

// NewWriter: no tools. Turns research notes into a 2-3 paragraph answer.
//
// Why no tools: the writer should not be tempted to second-guess research.
// Giving it zero tools forces it to work from its input.
func NewWriter(gen generateFunc, depth int) *Agent {
	return &Agent{
		Name:  "writer",
		Model: "gemini-2.5-flash",
		SystemPrompt: "You are a technical writer. Given a set of research notes, " +
			"produce a clear 2-3 paragraph answer for the user. Do not invent " +
			"facts beyond the notes.",
		MaxIterations: 2,
		Depth:         depth,
		Generate:      gen,
		Dispatch: func(_ context.Context, name string, _ map[string]any) string {
			return "error: writer has no tools, but model called " + name
		},
	}
}

// NewOrchestrator: coordinates researcher + writer. Each sub-agent is exposed
// as a tool. The dispatch map is the heart of the demo: notice that calling
// `research` (which runs a whole nested agent loop) looks identical to
// calling any plain Go function.
func NewOrchestrator(gen generateFunc) *Agent {
	researcher := NewResearcher(gen, 1)
	writer := NewWriter(gen, 1)

	researchSchema := &genai.FunctionDeclaration{
		Name:        "research",
		Description: "Delegate research on a topic to the researcher sub-agent. Returns a bullet list of findings.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"topic": {Type: genai.TypeString, Description: "What to research. Be specific."},
			},
			Required: []string{"topic"},
		},
	}

	writeSchema := &genai.FunctionDeclaration{
		Name:        "write",
		Description: "Delegate writing the final user-facing answer to the writer sub-agent. Pass the research notes as input.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"notes": {Type: genai.TypeString, Description: "The research notes from a prior research() call."},
			},
			Required: []string{"notes"},
		},
	}

	return &Agent{
		Name:  "orchestrator",
		Model: "gemini-2.5-flash",
		SystemPrompt: "You coordinate two specialists. For any user question: " +
			"(1) call `research` with a focused topic, " +
			"(2) call `write` with the research notes from step 1, " +
			"(3) reply with the writer's output as your final answer. " +
			"Do not answer from your own knowledge. Do not skip steps.",
		Tools: []*genai.Tool{{
			FunctionDeclarations: []*genai.FunctionDeclaration{researchSchema, writeSchema},
		}},
		MaxIterations: 10,
		Depth:         0,
		Generate:      gen,
		Dispatch: func(ctx context.Context, name string, args map[string]any) string {
			switch name {
			case "research":
				return runOrErr(researcher.Run(ctx, args["topic"].(string)))
			case "write":
				return runOrErr(writer.Run(ctx, args["notes"].(string)))
			}
			return "error: unknown tool " + name
		},
	}
}
```

- [ ] **Step 2: Verify the package builds**

Run: `cd minimal-agent-go && go build ./...`
Expected: build succeeds. No new tests yet — factories are configuration, exercised end-to-end by the smoke test in Task 9.

- [ ] **Step 3: Make sure existing tests still pass**

Run: `cd minimal-agent-go && go test ./... -v`
Expected: 6/6 pass.

- [ ] **Step 4: Commit**

```bash
cd minimal-agent-go
git add roles.go
git commit -m "feat(roles): orchestrator + researcher + writer factories"
```

---

## Task 8: Rewrite `main.go` as the entry point

**Files:**
- Modify: `minimal-agent-go/main.go`

- [ ] **Step 1: Replace `main.go` with the real entry point**

```go
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
```

- [ ] **Step 2: Verify the package builds**

Run: `cd minimal-agent-go && go build ./...`
Expected: build succeeds.

- [ ] **Step 3: Verify tests still pass**

Run: `cd minimal-agent-go && go test ./... -v`
Expected: 6/6 pass.

- [ ] **Step 4: Commit**

```bash
cd minimal-agent-go
git add main.go
git commit -m "feat(main): wire orchestrator entry point"
```

---

## Task 9: End-to-end smoke run

**Files:** none modified.

This task is a manual verification, not code. Confirm the multi-agent loop works against the real Gemini API.

- [ ] **Step 1: Ensure `GEMINI_API_KEY` is set**

Run: `cd minimal-agent-go && grep -q GEMINI_API_KEY .env && echo "have key in .env" || echo "no key in .env"`

If missing, either populate `.env` or export `GEMINI_API_KEY` in the shell before running.

- [ ] **Step 2: Run the program**

Run (with the API key available in env):

```bash
cd minimal-agent-go
export GEMINI_API_KEY="$(grep GEMINI_API_KEY .env | cut -d= -f2-)" 2>/dev/null || true
go run .
```

Expected output structure (exact wording will vary — Gemini responses are non-deterministic):

```
user: What is the Gemini API and when would you use it?
[orchestrator] turn 1: calling Gemini
[orchestrator]   wants to call: research(topic="...")
  [researcher] turn 1: calling Gemini
  [researcher]   wants to call: search(query="...")
  [researcher]   result: The Gemini API is Google's...
  [researcher] turn 2: calling Gemini
  ...
  [researcher] final: • ...
[orchestrator]   result: • ...
[orchestrator] turn 2: calling Gemini
[orchestrator]   wants to call: write(notes="...")
  [writer] turn 1: calling Gemini
  [writer] final: The Gemini API is...
[orchestrator]   result: The Gemini API is...
[orchestrator] turn 3: calling Gemini
[orchestrator] final: The Gemini API is...

=== final answer ===
...
```

Verification checklist:
- [ ] Orchestrator calls `research` at least once.
- [ ] Researcher calls `search` at least once.
- [ ] Orchestrator calls `write` after `research`.
- [ ] Indented logging shows the nested hierarchy (researcher/writer lines indented under orchestrator).
- [ ] Final answer is printed.

If the orchestrator skips `research` or `write` (model can be lazy), tighten the system prompt — most fixes are wording adjustments in `roles.go`, then re-run.

- [ ] **Step 3: Commit (only if any tweaks were made)**

If the smoke run required prompt tweaks:

```bash
cd minimal-agent-go
git add roles.go
git commit -m "fix(roles): tighten orchestrator prompt after smoke run"
```

Otherwise skip.

---

## Task 10: Write `README.md`

**Files:**
- Create: `minimal-agent-go/README.md`

- [ ] **Step 1: Write the README**

```markdown
# minimal-agent-go

A tiny multi-agent demo in Go, built for learning. ~500 lines total. Read top-to-bottom and you understand every line.

## What is an agent

An agent is a **loop**:

1. Send the conversation so far + tool descriptions to an LLM.
2. The LLM either replies with TEXT (done) or asks to CALL A TOOL.
3. Run the tool, append the result, go to 1.

Every "AI agent" you've heard of is built on this loop. The whole thing is implemented in `agent.go`.

## Why sub-agents?

A single agent works fine for small tasks, but two problems show up fast:

- **Context bloat.** Every tool call lives forever in the agent's history. Twenty searches blow your token budget and confuse the model.
- **Mixed concerns.** One system prompt has to be researcher AND writer AND planner. The model does each role worse.

The fix: **sub-agents as tools**. The parent agent sees `research(topic)` and `write(notes)` as ordinary tools. Internally each one runs a fresh agent loop with its own system prompt, its own tools, and its own context. The parent only sees the final result. Context isolation, role specialization, same dispatch contract.

The key code is in `roles.go`:

```go
Dispatch: func(ctx context.Context, name string, args map[string]any) string {
    switch name {
    case "research":
        return runOrErr(researcher.Run(ctx, args["topic"].(string)))
    case "write":
        return runOrErr(writer.Run(ctx, args["notes"].(string)))
    }
    return "error: unknown tool " + name
}
```

This is structurally identical to calling `calculator(...)` or `search(...)`. The orchestrator does not know — and cannot tell — that it's spawning whole agent loops. **Sub-agents are tools.**

## Running it

```bash
export GEMINI_API_KEY=...
go run .
```

You'll see indented logs as the orchestrator delegates to researcher → search, then writer, then prints the final answer.

## Reading order

1. **`main.go`** — 30 lines, just wiring. Read first to see the shape of the program.
2. **`agent.go`** — the `Agent` struct and the `Run` loop. This is THE agent loop; everything else is configuration.
3. **`roles.go`** — three factories. Notice that researcher, writer, and orchestrator are all the same struct, distinguished only by prompt + tools + dispatch.
4. **`tools.go`** — the leaf tools: `search` (fake), `calculator`, `get_weather`.
5. **`agent_test.go`** — how the loop is unit-tested without hitting the real Gemini API: inject a scripted `generateFunc`.

## Try changing things

- Tighten or loosen the orchestrator's system prompt in `roles.go` and watch how its delegations change.
- Add a `critic` sub-agent that reviews the writer's draft.
- Add a third leaf tool (a fake "calendar" or "stock price") and let the researcher use it.
- Swap the fake `search` in `tools.go` for a real API.
- Run researchers in parallel: have the orchestrator call `research(topic)` three times concurrently via goroutines, then `write(notes)` with the merged result.

Each of these is a small diff that compounds your mental model.

## What's deliberately missing

This is a learning repo. No retry/backoff, no token counting, no REPL, no streaming, no concurrency. The `Agent` abstraction supports adding any of them without rewrites — see the design spec under `docs/superpowers/specs/` if you want the long version.
```

- [ ] **Step 2: Sanity-check line count**

Run: `cd minimal-agent-go && wc -l README.md`
Expected: under 200 lines (target was set in the spec).

- [ ] **Step 3: Commit**

```bash
cd minimal-agent-go
git add README.md
git commit -m "docs: add learner-facing README"
```

---

## Self-review checklist

Before declaring done, walk through the spec one more time and verify:

- [ ] Generic `Agent` struct with `Name`, `Model`, `SystemPrompt`, `Tools`, `Dispatch`, `MaxIterations`, `Depth`, `Generate` — Task 1.
- [ ] `generateFunc` type, injectable for tests — Task 1.
- [ ] `geminiGenerate` adapter — Task 1.
- [ ] `runOrErr` helper used by orchestrator dispatch — Task 1 (definition) + Task 7 (use).
- [ ] Loop terminates on text, dispatches function calls, errors at cap, handles unknown tool — Tasks 2/3/4/5.
- [ ] Researcher with `search` tool, MaxIterations=5 — Task 7.
- [ ] Writer with no tools, MaxIterations=2 — Task 7.
- [ ] Orchestrator with `research`+`write` schemas, MaxIterations=10 — Task 7.
- [ ] Indented logging by `Depth` — Task 1 (`logf`) + Task 7 (factories set depth).
- [ ] Sub-agent errors flow back as tool-result strings — Task 1 (`runOrErr`) + Task 7 (orchestrator dispatch).
- [ ] Unit tests for `calculator` (including divide-by-zero, unknown op) and `search` — Task 6.
- [ ] README with concept doc embedded — Task 10.
- [ ] `main.go` thin entry — Task 8.
- [ ] No leftover code from the original single-file `main.go` — Task 6 (placeholder) + Task 8 (final).

If any item is missing, add a follow-up task before handing off to execution.
