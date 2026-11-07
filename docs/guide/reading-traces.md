# Reading traces

Pocketfuse reads an agent's work at two levels, and links between them keep your
place.

## A session is a conversation

The session page shows one conversation from top to bottom. Each turn holds what
started it, its steps in order with their timing, and what came back. Plain
requests between turns collapse into one line, and routine polling stays hidden
until you ask for it. Clicking a step opens its input, output and tool calls beside
the conversation. **Full trace** opens the turn's whole structure.

A trace becomes a turn according to its spans:

| Kind | Recognized by |
|---|---|
| Turn | a span whose name ends in `.turn` |
| Wake turn | a span whose name ends in `.wake_turn` |
| Job | a span whose name ends in `job.run`; `agent_id`, `job_id` and `plan_id` attributes are shown |
| Run | any other trace holding a model call |

A trace with none of these is a plain request.

## A trace is a tree with timing

The trace page shows every span as a tree row and a bar on one time axis. Selecting
a span shows it on the right, with its output and input first. **Graph** and **Log**
are other views of the same spans.

## What is derived, and from what

| Shown | Comes from |
|---|---|
| One model call | A generation whose only child is a generation with the same model and identical input and output tokens, where the child has no payload. Instrumented clients record one provider call this way, the outer span holding the payload and the inner one measuring the stream. **Raw spans** shows them as recorded, and totals always count the call once. |
| The tool result on a tool call | The tool result message with the same call id in a later model call's input. A tool span with the same name folds in too, adding its duration. |
| User message and reply | The last user text of the turn's first model call, and the last text the model produced. |
| Side call | A model call whose system prompt differs from the one most of the turn's calls share, such as a title generator. It does not count as the turn's reply. |
| Tool error | A tool result with an `error` field. The model saw it and carried on, so the span is not marked failed. |
| "Started by the tool call above" | An earlier tool result that names this job's `job_id`. |
| Time match | A trace with no session that started during a request. Nothing records the connection, so another session may own it. |
