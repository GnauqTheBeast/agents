package main

import (
	"context"

	"google.golang.org/genai"
)

// -----------------------------------------------------------------------------
// Role factories. Each returns an *Agent wired with a role-specific system
// prompt, tool set, and dispatch map. This is where you'd add new roles.
// -----------------------------------------------------------------------------

// NewResearcher gathers facts via the `search` tool, returns a bullet list.
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

// NewWriter no tools. Turns research notes into a 2-3 paragraph answer.
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

// NewOrchestrator coordinates researcher + writer. Each sub-agent is exposed
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
