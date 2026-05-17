# Multi-agent upgrade — design

**Date:** 2026-05-16
**Status:** approved (pending written-spec review)
**Repo:** `minimal-agent-go`

## Goal

Upgrade the current single-agent demo into a small multi-agent system that teaches the **sub-agent-as-tool** pattern: a parent agent that delegates work to specialist sub-agents through the same tool-calling interface it uses for ordinary functions. The orchestrator should be unable to tell, at the code level, the difference between calling a Go function and calling a nested agent loop.

The project remains a **learning-by-doing** demo. The bar is "readable in one sitting," not "production-ready."

## Non-goals

- Real web search, real APIs beyond Gemini, or persistent storage.
- Token budgeting, retry/backoff, or rate-limit handling.
- An interactive REPL or multi-turn user memory (deferred to a later upgrade).
- Concurrency primitives (parallel sub-agents). Single-threaded by design.
- Coverage-driven testing. Tests exist to prove the loop terminates correctly; they are not exhaustive.

## Architecture

A single `Agent` struct represents every role. Orchestrator, researcher, and writer are all instances of it, distinguished only by data (system prompt + tool set + dispatch map). The lesson: **roles are data, not code.**

```go
type Agent struct {
    Name          string
    Model         string
    SystemPrompt  string
    Tools         []*genai.Tool
    Dispatch      func(ctx context.Context, name string, args map[string]any) string
    MaxIterations int
    generate      generateFunc // injectable for tests; production wires to genai client
}

func (a *Agent) Run(ctx context.Context, userInput string) (string, error)
```

`Run` is the existing agent loop lifted into a method. Nesting is achieved by making the orchestrator's `Dispatch` map call `researcher.Run` and `writer.Run` — their return strings become tool-result strings for the orchestrator. There is no separate "sub-agent" type or "delegation" abstraction; it is the same function call shape as any leaf tool.

### File layout

| File | Purpose | Approx LOC |
|---|---|---|
| `agent.go` | `Agent` struct + `Run` loop + `generateFunc` type | ~120 |
| `tools.go` | Leaf tools (`search`, `get_weather`, `calculator`) + their `*genai.Tool` schemas | ~100 |
| `roles.go` | Factories: `NewOrchestrator`, `NewResearcher`, `NewWriter` (system prompts + dispatch wiring) | ~80 |
| `main.go` | Entry point: read API key, build orchestrator, run query, print result | ~30 |
| `agent_test.go` | Loop-termination tests using a scripted fake `generate` | ~80 |
| `tools_test.go` | Unit tests for leaf tools | ~40 |

Existing `main.go` is replaced; the heavy explanatory comments from it migrate to `agent.go` (top-level loop) and the `README.md` (conceptual sections).

## Components

### Researcher

- **Tools:** `search(query: string) → string`
- **Sub-agents:** none
- **MaxIterations:** 5
- **System prompt (intent):** "You are a research assistant. Use the `search` tool one or more times to gather facts about the topic. When you have enough, reply with a concise bullet list of findings. Do not write prose paragraphs — leave that to the writer."
- **`search` implementation:** a hardcoded `map[string]string` of canned snippets keyed by lowercased substring match, with a `"no results for '%s'"` fallback. Zero external setup, deterministic for learning.

### Writer

- **Tools:** none
- **Sub-agents:** none
- **MaxIterations:** 2 (one call expected; cap exists only as a safety net)
- **System prompt (intent):** "You are a technical writer. Given a set of research notes, produce a clear 2–3 paragraph answer for the user. Do not invent facts beyond the notes."

The writer having zero tools is itself a teaching point: not every agent needs tools, and forcing the writer to rely only on its input prevents it from second-guessing research.

### Orchestrator

- **Tools (presented to the LLM):**
  - `research(topic: string) → string` — "Delegate research on a topic to the researcher sub-agent."
  - `write(notes: string) → string` — "Delegate writing a final answer to the writer sub-agent, given research notes."
- **Sub-agents:** researcher, writer (invoked via the dispatch map)
- **MaxIterations:** 10 (higher than children because each delegation consumes one orchestrator turn)
- **System prompt (intent):** "You coordinate two specialists. For any user question: (1) call `research` with a focused topic, (2) call `write` with the research notes, (3) reply with the writer's output. Do not answer from your own knowledge."

The dispatch map is the heart of the demo. It looks identical for leaf tools and sub-agents:

```go
Dispatch: func(ctx context.Context, name string, args map[string]any) string {
    switch name {
    case "research":
        return runOrErr(researcher.Run(ctx, args["topic"].(string)))
    case "write":
        return runOrErr(writer.Run(ctx, args["notes"].(string)))
    }
    return fmt.Sprintf("error: unknown tool %q", name)
}
```

`ctx` flows from the parent's `Run` into `Dispatch` so cancellation/timeouts propagate to sub-agents. `runOrErr` collapses `(string, error)` into the single string the LLM expects: returns the string on success, or `"error: <err>"` on failure (see error handling).

## Data flow

A query like *"What is the Gemini API and when would you use it?"* produces:

```
main()
 └─ orchestrator.Run(question)
     │  turn 1 → model calls: research(topic="...")
     ├─ researcher.Run("...")
     │   ├─ turn 1: search(query="Gemini API") → snippet
     │   ├─ turn 2: search(query="when to use Gemini") → snippet
     │   └─ turn 3: text reply (bullet list) → returned to orchestrator
     │  turn 2 → model calls: write(notes="• ...")
     ├─ writer.Run("• ...")
     │   └─ turn 1: text reply → returned to orchestrator
     │  turn 3 → model replies with TEXT → returned to main
     └─ printed
```

Each sub-agent gets a fresh `[]*genai.Content` history. The orchestrator never sees the researcher's intermediate `search` calls — only the final summary. This is **context isolation**: each level keeps its own context focused, and the parent's token budget is spent on coordination, not on the child's working memory.

### Logging

Each agent prints lines prefixed by its name and indented by nesting depth, so a real run reads like the trace above. Format:

```
[orchestrator] turn 1: calling Gemini
[orchestrator]   wants to call: research(topic="...")
  [researcher] turn 1: calling Gemini
  [researcher]   wants to call: search(query="...")
  [researcher]   result: ...
  [researcher] final: ...
[orchestrator]   result: ...
```

Indent is passed via the `Agent` struct as a `depth int` field set by the factories.

## Error handling

Three modes handled explicitly:

1. **Sub-agent exceeds its iteration cap.** `Agent.Run` returns a Go error. The orchestrator's dispatch entry for `research`/`write` catches that error and returns it as a string to the LLM (e.g. `"error: researcher failed: exceeded 5 iterations"`). Errors flow back to the model as text, not up the call stack — the LLM decides how to react.
2. **Unknown tool name in dispatch.** Return `"error: unknown tool <name>"` as the tool result.
3. **Top-level iteration cap hit.** Only the orchestrator's `Run` error propagates to `main()` and exits.

Deliberately not handled (YAGNI for a learning project):

- Retries on transient Gemini API errors — propagate the first error.
- Token budgeting / counting.
- Concurrency-safe state — single-threaded.

## Testing

Goal: prove the loop's control flow is correct, without hitting the real Gemini API.

`Agent.Run` takes a `generate generateFunc` field instead of calling `client.Models.GenerateContent` directly:

```go
type generateFunc func(ctx context.Context, model string, history []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
```

Production wires this to `client.Models.GenerateContent`. Tests pass a closure over a scripted slice of responses.

### `agent_test.go`

- `TestRunTerminatesOnText` — scripted response with only a text part; `Run` returns that text and stops.
- `TestRunLoopsOnFunctionCall` — first response calls a tool, second responds with text. Verify dispatch is called with the right args; final return is the text.
- `TestRunErrorsAtIterationCap` — every scripted response calls a tool. `Run` errors after `MaxIterations`.
- `TestUnknownToolReturnsErrorString` — model calls a tool not in dispatch; dispatch returns `"error: unknown tool"`; loop continues; next response is text; verify final text.

### `tools_test.go`

- Unit tests for `calculator` (each operator, divide-by-zero).
- Unit test for `search` (known query, unknown query).
- Skip `get_weather` (literal string formatting, no logic).

No tests for the role factories themselves — they are configuration, not logic.

## Docs

- `README.md` in `minimal-agent-go/` root. Sections:
  - *What is an agent* — the loop in 6 lines.
  - *The two tool types* — leaf tools vs. sub-agents-as-tools, with the dispatch snippet.
  - *Why sub-agents* — context isolation, role specialization.
  - *Running the demo* — env var, `go run .`, sample output.
  - *Reading order* — `main.go` → `agent.go` → `roles.go` → `tools.go`.
  - Under 200 lines.
- Inline comments in `agent.go` and `roles.go` keep the current heavy-explanation style. Each role factory carries a short "why this prompt" note.
- This design doc itself lives under `docs/superpowers/specs/` as a record; it is not the reader-facing learning material.

## Out of scope (parking lot for later upgrades)

- Interactive REPL + persistent conversation.
- Parallel sub-agent fan-out (researchers in goroutines).
- Real search API (Tavily, DuckDuckGo).
- Token / cost tracking.
- A critic role (planner → worker → critic).
- Streaming responses.

Each is a clean follow-on: the `Agent` abstraction supports them without rewrites.
