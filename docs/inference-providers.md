# Inference providers

Source of truth for **connection types** Agent Fabric ships and which `settings.inference` fields each type sends on the wire. The model catalog (what a connection can serve) is refreshed from each provider’s `/models` endpoint; capabilities, limits and prices come from synced *model specs* (see below).

When adding or changing a connection type, update this document in the same change.

## Connection types

| Type string | Display name | Base URL | Wire adapter | Credentials |
| --- | --- | --- | --- | --- |
| `openai_compatible` | Custom | User-supplied | Chat Completions | API key + base URL |
| `unsloth_studio` | Unsloth Studio | User-supplied | Chat Completions (+ Unsloth extras) | API key + base URL |
| `berget_ai` | Berget AI | Fixed `https://api.berget.ai/v1` | Chat Completions (+ Berget extras) | API key only |
| `opencode_zen` | OpenCode Zen | Fixed `https://opencode.ai/zen/v1` | Routes per model: Chat Completions, Anthropic Messages, or OpenAI Responses | API key only |
| `opencode_go` | OpenCode Go | Fixed `https://opencode.ai/zen/go/v1` | Same routing as Zen | API key only |

Knobs live on the **assistant** as `settings.inference`, merged via catalog PATCH and **pinned at `session/new`**. Adapters send only set fields (`omitempty`).

**Effective instructions** (Platform + Assistant + Runtime context, composed with variable substitution and pinned at `session/new`) are sent separately from sampling knobs. See the instruction wire mapping below.

## `settings.inference` field matrix

Legend: **UI** = Assistants Inference panel exposes the control for that connection type; **send** = adapter includes the field when set; **—** = not sent for that type (even if stored from a previous connection).

| Catalog key | Wire JSON | Custom | Unsloth | Berget | OpenCode Zen/Go |
| --- | --- | --- | --- | --- | --- |
| `temperature` | `temperature` | UI + send | UI + send | UI + send | UI + send (Chat / Anthropic / Responses as supported) |
| `maxTokens` | `max_tokens` / `max_output_tokens` | UI + send | UI + send | UI + send | UI + send (Responses uses `max_output_tokens`) |
| `reasoningEffort` | `reasoning_effort` / thinking budget / `reasoning.effort` | UI + send | UI + send | UI + send | UI + send (mapped per wire) |
| `topP` | `top_p` | send if set | UI + send | UI + send | send if set (Chat / Anthropic) |
| `topK` | `top_k` | — | UI + send | UI + send | — |
| `minP` | `min_p` | — | UI + send | UI + send | — |
| `repetitionPenalty` | `repetition_penalty` | — | UI + send | UI + send | — |
| `presencePenalty` | `presence_penalty` | — | UI + send | UI + send | — |
| `frequencyPenalty` | `frequency_penalty` | — | — | UI + send | — |
| `enableThinking` | `enable_thinking` | — | UI + send | — | — |
| `thinkingType` | `thinking.type` | — | — | UI + send | — |

`reasoningEffort` allowed values: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`.

`thinkingType` allowed values: `disabled`, `enabled`, `adaptive`.

## Wire mapping notes

| Catalog | Request body |
| --- | --- |
| `topP` | `top_p` |
| `maxTokens` | `max_tokens` (Chat Completions); Responses uses `max_output_tokens` |
| `reasoningEffort` | Chat Completions: `reasoning_effort`; OpenCode Anthropic: thinking budget; OpenCode Responses: `reasoning.effort` |
| `topK` / `minP` / `repetitionPenalty` / `presencePenalty` / `frequencyPenalty` | `top_k` / `min_p` / `repetition_penalty` / `presence_penalty` / `frequency_penalty` |
| `enableThinking` | `enable_thinking` (boolean; Unsloth only) |
| `thinkingType` | `thinking: { "type": "<value>" }` (Berget only; not the same as Unsloth `enable_thinking`) |

## Instruction wire mapping

Pinned **effective instructions** are passed to adapters as `StreamChatOptions.Instructions`. Empty values are omitted (no phantom messages or empty fields).

| Wire adapter | Representation when instructions are set |
| --- | --- |
| Chat Completions (`openai_compatible`, Unsloth, Berget) | Leading `messages[]` entry with `role: "system"` |
| Anthropic Messages | Top-level `system` string |
| OpenAI Responses | Top-level `instructions` string (not duplicated as a developer input item) |

On Anthropic, any `system`-role messages in history are appended after effective instructions in the same `system` string (the plane creates none today).

OpenCode Zen/Go inherit the mapping of whichever sub-adapter the model routes to.

## Provider-specific notes

### Custom (`openai_compatible`)

- Caller supplies any OpenAI-compatible Chat Completions base URL.
- Core knobs only in the Assistants UI; no assumed vendor extras.

### Unsloth Studio (`unsloth_studio`)

- Local Chat Completions endpoint with extended samplers.
- `enableThinking` maps to Unsloth’s `enable_thinking` boolean.
- Stream responses often include Unsloth-style `timings` (prompt/predicted ms and tok/s), surfaced in chat stats when present.

### Berget AI (`berget_ai`)

- Fixed EU endpoint `https://api.berget.ai/v1` ([quickstart](https://docs.berget.ai/quickstart), [API reference](https://api.berget.ai/#tag/models)).
- Extended samplers match vLLM/SGLang-style Chat Completions fields (`top_k`, `min_p`, `repetition_penalty`) plus OpenAI `presence_penalty` / `frequency_penalty`.
- `thinkingType` is Moonshot/Kimi **K2**-oriented CoT control. Models that always think (e.g. Kimi K3) do not support disabling via `thinking`; use `reasoningEffort` instead — on those models `"none"` lowers the budget rather than turning CoT off.
- Usage may include undocumented `co2_grams` and `gpu_energy_joules`; when present they appear in chat stats as `co2Grams` / `gpuEnergyJoules`.

### OpenCode Zen / Go (`opencode_zen`, `opencode_go`)

- Fixed official base URLs; only an API key is required.
- At prompt time the plane picks Chat Completions, Anthropic Messages, or OpenAI Responses from the model id and sends `User-Agent: agent-fabric/…` plus a stable `x-opencode-session`.
- Gemini and Jev models are filtered from model refresh until adapters exist.
- Reasoning effort mapping differs on Anthropic vs Responses paths; sampler extras above are not sent on OpenCode wires.

## Usage / stats fields

Persisted on catalog usage message parts and ACP `usage_update` meta (camelCase). All omitempty when absent.

| Field | Meaning |
| --- | --- |
| `promptTokens` / `completionTokens` / `totalTokens` | Token counts from the provider when reported |
| `ttftMs` / `elapsedMs` | Plane-measured time to first token and turn wall time |
| `promptMs` / `predictedMs` / `promptPerSecond` / `predictedPerSecond` | Provider timings (e.g. Unsloth) when present |
| `co2Grams` / `gpuEnergyJoules` | Berget (and any OpenAI-compatible upstream that emits them) |
| `deltas` | Stream chunk count for the turn |
| `stopReason` | Why generation stopped |

## Model specs (models.dev)

The plane syncs a [models.dev](https://models.dev)-shaped `api.json` (providers, models, capabilities, limits, modalities, USD prices per 1M tokens) instead of embedding it, so new models appear without a release. The snapshot is stored in Postgres (`model_specs`) and refreshed on boot and every `syncIntervalHours` (default 24).

- Source: `GET/PATCH /v1/model-specs/settings` (`sourceUrl`, empty = `https://models.dev/api.json`; `syncIntervalHours`; `enabled`). Point `sourceUrl` at your own file with the same structure to add custom models; the source may be a private/LAN address because only the operator can set it.
- `GET /v1/model-specs/status` shows source, last successful sync, last attempt/error and counts; `POST /v1/model-specs/sync` syncs now. A failed or invalid download keeps the previous snapshot.
- Prices are estimates for display; they never enter provider requests.
