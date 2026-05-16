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
