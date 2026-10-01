# Project terminology

Svensk version: [Projektterminologi](terminology.sv.md).

This is Agent Fabric's canonical vocabulary. Keep the English terms in code, APIs, UI, and both language versions. Qualify agent, session, role, prompt, context, resource, and memory whenever their meaning is not explicit.

## Product and runtime

| Term | Meaning |
| --- | --- |
| **Assistant** | Configurable Catalog object owning identity, purpose, instructions, inference selection, tools, policy, and execution settings. |
| **Assistant purpose** | Human-readable explanation of why an Assistant exists and what it is responsible for. |
| **assistant configuration** | The settings contained by an Assistant; not a separate object. |
| **ACP Agent** | The Agent protocol role in ACP. The control plane exposes each Assistant as a logical ACP Agent. |
| **agent runtime** | The control-plane implementation that runs the prompt and tool loop for an ACP Agent. |
| **Project** | Catalog-owned scope grouping threads, Assistants, settings, resources, environments, and files. |
| **Workbench** | Client UI for an open Project: files, chat, editors, previews, dock, and panels. |

Use **Assistant** for the configured product object. Use Agent only in a qualified technical term such as **ACP Agent** or **agent runtime**.

## Conversation

| Term | Meaning |
| --- | --- |
| **ACP connection** | One initialized transport connection between an ACP Client and ACP Agent. |
| **ACP session** | A live runtime handle created by `session/new`; it pins an Assistant snapshot and current model. |
| **thread** | The persistent, user-visible conversation. A thread can outlive many ACP sessions. |
| **turn** | One user prompt and all resulting work until completion, cancellation, or failure. |
| **attempt** | One assistant response variant for a user prompt within a turn. Soft-supersede retry keeps prior attempts inspectable; only the **active** attempt is shown on the main conversation path. |
| **attempt status** | Lifecycle of an assistant attempt: `running` (in flight), `completed` (successful terminal), `failed` (error or interrupted after restart), or `cancelled` (user stop via `session/cancel`). Model context hydrates active assistants only when status is `completed`. |
| **round** | One internal agent-runtime iteration containing a model call and its response. |
| **provider request** | One concrete request to an inference service and its response. Inspector uses this term; round is for runtime tracing. |
| **thread record** | The canonical server-owned record for a thread. |
| **thread message** | A persistent user or assistant entry in a thread record. |
| **message content** | The primary visible text of a thread message. |
| **message part** | A persistent structured part such as thought, tool call, message, or usage. |
| **turn activity** | Client presentation derived from message parts; not a separate source of truth. |
| **thread history** | The ordered persistent thread messages and parts. |
| **conversation view** | The Workbench projection of thread history shown to the user. |
| **model context** | Semantic input assembled for one model call; it can differ from thread history and conversation view. |
| **provider message** | A message serialized into a provider wire API for one provider request. |
| **in-turn tool context** | Tool calls and results retained for the current turn; they need not be rehydrated in the next turn. |
| **provider request capture** | Immutable, scrubbed record of what was sent to and received from an inference service. |

```text
thread record
  └─ thread history
      └─ thread messages
          ├─ message content
          └─ message parts ──rendered as──> turn activities in conversation view

thread history + current input + runtime data
  └─ model context
      └─ provider messages
          └─ provider request capture
```

## Instructions, context, and memory

| Term | Meaning |
| --- | --- |
| **Harness instructions** | General Agent Fabric instructions describing runtime methodology, tool use, and shared behavior. |
| **Assistant instructions** | Persistent Assistant-owned instructions describing its specialization, behavior, and boundaries. |
| **instruction override** | An explicit scoped change to an instruction source, such as a thread or turn override. |
| **effective instructions** | Provider-independent result of composing Harness instructions, Assistant instructions, and applicable overrides for one model call. |
| **provider instruction message** | Provider-specific serialization of effective instructions. |
| **system message** | A provider instruction message with the `system` message role. |
| **developer message** | A provider instruction message with the `developer` message role. |
| **user prompt** | The user's current request that starts or continues a turn. |
| **prompt template** | A reusable, optionally parameterized template from which a prompt can be created. |
| **MCP prompt** | A prompt or prompt template exposed by an MCP server. |
| **policy** | A rule enforced by the control plane. Security must not depend only on model instructions. |
| **message role** | A message's semantic role, such as `user`, `assistant`, `system`, `developer`, or `tool`. |
| **protocol role** | A participant's responsibility in a protocol, such as ACP Client or ACP Agent. |
| **access role** | An authorization or organizational role. Always use the qualified term. |
| **context source** | A source that can contribute to model context, such as instructions, history, memory, or a resource. |
| **memory** | Persistent server-owned knowledge retrievable for later model contexts. Thread history and client UI state are not memory. |
| **memory record** | One persistent item in memory. |
| **memory retrieval** | Selection of relevant memory records for the current request. |
| **memory injection** | Addition of retrieved memory to model context. |
| **MCP resource** | Data exposed by an MCP server. It can be a context source but is not a tool or memory. |
| **Catalog resource** | Project-related material stored or referenced through Catalog. |
| **client UI state** | Local presentation state such as open panels, selected files, and Workbench layout. A **WorkbenchStateStore** persists it. |

```text
Harness instructions ───┐
Assistant instructions ─┼──> effective instructions ──> provider instruction message
instruction overrides ──┘

effective instructions + thread history + user prompt + memory/resources
  └─ model context

policy is enforced separately by the control plane
```

## Inference

| Term | Meaning |
| --- | --- |
| **inference connection** | Saved Catalog object containing name, connection type, endpoint, credentials, and model catalog. |
| **connection type** | Selects validation and the provider-adapter family for an inference connection. |
| **inference provider** | The external vendor or product family behind inference. |
| **inference service** | The concrete local or remote service receiving inference requests. |
| **inference endpoint** | The network address of an inference service. |
| **provider adapter** / **streamer** | Control-plane code translating internal data to a provider wire API. |
| **provider wire API** | External request/response format such as Chat Completions, Anthropic Messages, or Responses. |
| **model** | The token-generating model offered by an inference service. |
| **model reference** | Local metadata identifying a model, normally ID and display name. |
| **model catalog** | The discovered model references for one inference connection. |
| **default model** | The model selected by an Assistant for new ACP sessions. |
| **current model** | The model currently pinned by an active ACP session. |
| **inference settings** | Assistant-owned generation settings pinned when an ACP session starts. |

## Files and execution

| Term | Meaning |
| --- | --- |
| **project files** | User-facing name for a Project's persistent files and directories. |
| **project filesystem** | The technical filesystem abstraction and API for project files. |
| **project root** | The root directory of a project filesystem. A concrete path may still be `/workspace`. |
| **project volume** | Persistent storage backing a project filesystem. |
| **execution environment** | The local or containerized runtime context where tools and commands execute. |
| **execution backend** | The mechanism creating an execution environment, such as local, Docker, or Podman. |
| **execution settings** | Catalog-owned settings inherited global → Project → Assistant. |
| **execution overlay** | The internally merged result of inherited execution settings. |
| **environment reuse scope** | How long an execution environment is reused: shared, session, or project. |
| **sandbox policy** | Security and isolation rules constraining execution. Sandbox is not the environment's name. |
| **control plane configuration** | Process bootstrap configuration for database, server, storage, and available execution backends. |
| **`config.json`** | File containing control plane configuration, not Project- or Assistant-owned execution settings. |

Do not use workspace as a product or domain term. Use **Project**, **Workbench**, **project files**, or **project filesystem**. A literal `/workspace` path can remain.

## Tools and interactions

| Term | Meaning |
| --- | --- |
| **tool** | A model-callable capability with a name, description, and input schema. |
| **tool call** | One invocation of a tool with arguments. |
| **tool result** | The output or failure from a tool call. |
| **tool origin** | Executor of a tool call: `environment`, `control_plane`, `mcp`, `client`, or `provider`. |
| **environment tool** | A tool executed through an execution environment. |
| **control plane tool** | A tool handled directly by the control plane, such as `ask_user`. |
| **MCP tool** | A tool exposed by an MCP server and invoked through MCP. |
| **client tool** | A tool executed by the connected client or device. |
| **provider tool** | A hosted capability executed by the inference provider. |
| **MCP** | The protocol through which an MCP server exposes tools, resources, and prompts. MCP is not a tool. |
| **MCP server** | The protocol peer exposing MCP capabilities. |
| **tool policy** | Rules governing what a tool may do. |
| **tool gate** | Control-plane component evaluating a tool call as `allow`, `ask`, or `deny`. |
| **permission request** | A request for authorization of a planned action. |
| **permission decision** | The user's response: allow once, allow for this session, or reject. |
| **permission grant** | Access created by an allowing permission decision. |
| **grant scope** | The lifetime or breadth of a permission grant. |
| **clarification** | A question requesting information or preference, not authorization. |
| **elicitation** | The ACP mechanism used to present a structured clarification. |
| **pending interaction** | Internal UI umbrella for an interaction awaiting the user. |
| **hop capture** | A scrubbed persistent record of traffic between control-plane-owned hops. |
| **integration** | A configured capability or connection to an external service. |
| **tool integration** | A catalog-owned integration that backs a plane tool capability such as `web_search` or `fetch_page`. |
| **tool binding** | An Assistant setting that selects inherit, disabled, or a specific tool integration for a capability. |
| **web_search** | Stable plane tool that returns bounded public-web search results. |
| **fetch_page** | Stable plane tool that returns cleaned Markdown for one public page. |

Use **Allow for this session** for an ACP-session-scoped grant. Use **Always** only for a persistent grant with an explicit lifecycle and revocation mechanism.

## Protocol boundaries

| Boundary | Canonical name |
| --- | --- |
| Settings and persistent configuration | **Catalog HTTP API** (`/v1`) |
| Interactive conversation and user interactions | **ACP** (`/acp`) |
| Inference requests and responses | **provider wire API** |
| External MCP capabilities | **MCP** |
| Tool dispatch inside the control plane | **tool registry / execution environment API** |

This vocabulary is the target language for [architecture](architecture.md), [decisions](decisions.md), code, APIs, and product copy. Implementation migrations are tracked in GitHub rather than recorded as alternative names here.
