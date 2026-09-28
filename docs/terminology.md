# Project terminology

This working vocabulary distinguishes the same word at catalog, protocol, runtime, and client boundaries. It describes today's system; it does not rename API fields, database tables, or types.

The short rule: qualify **agent**, **session**, and **workspace** whenever the surrounding noun does not make the boundary clear.

## Boundary map

| Boundary | What crosses it | Canonical name |
| --- | --- | --- |
| Settings UI to control plane | Configuration and persisted records | **Catalog HTTP API** (`/v1`) |
| Cockpit to control plane | Interactive conversation and interaction events | **ACP** (`/acp`) |
| Control plane to inference service | Model request and streamed response | **provider wire API** |
| Control plane to execution target | Tool invocation and result | **sandbox registry / environment API** |

Do not call all of these "the agent API." ACP is the runtime protocol; Catalog HTTP configures the things exposed through ACP.

## Agent and inference

| Prefer | Meaning | Avoid when precision matters |
| --- | --- | --- |
| **agent definition** | A versioned catalog record: provider, default model, inference settings, sandbox overlay, policy, and future metadata. Settings edits it; ACP `session/new` pins it. | "agent" when the reader could mean a protocol peer or Go object |
| **ACP Agent** | The ACP protocol role implemented by the control plane. An ACP Client talks to this peer. Each definition is exposed as one logical ACP Agent. | model, provider, server process |
| **agent runtime** | The control-plane prompt/tool-loop implementation serving an ACP Agent. Current Go type: `internal/agent.Agent`. | agent definition |
| **provider** | A configured inference service plus adapter and credentials. | model; agent |
| **provider adapter** / **streamer** | The control-plane component mapping internal messages/events to a provider wire API. | provider, unless the external service is meant |
| **model** | The token-generating model selected through a provider. It is not an ACP peer and does not run tools directly. | agent |
| **inference settings** | Generation controls owned by an agent definition at `settings.inference`, then pinned into an ACP session. | session settings; ACP sampling |

The product UI may use **Agent** as a concise label for an *agent definition*, but explanatory copy should say "configured agent" or "agent definition" on first reference.

## Conversation and lifetime

| Prefer | Meaning | Avoid when precision matters |
| --- | --- | --- |
| **ACP connection** | One initialized Client ↔ ACP Agent transport connection; capabilities are negotiated for this lifetime. | session |
| **ACP session** | A live control-plane handle from `session/new`. It pins a definition snapshot and current model; its ID begins `sess_`. | thread; UI session |
| **catalog thread** (or **thread** in catalog/UI context) | Persisted user-visible conversation: title, selected agent/model, messages, and turn parts. An ACP session may bind to it. | ACP session |
| **turn** | One user prompt and resulting agent work/output committed to a thread. It may have several model/tool rounds. | message; session |
| **round** | One provider inference request/response cycle inside a turn. Tool calling can add rounds. | turn |
| **message part** | An ordered persisted assistant-turn piece: thought, tool call, message, or usage. | message, when the complete record is meant |
| **conversation history** | Persisted thread messages hydrated for later prompts. | transcript, if it implies client ownership |

`ProjectWorkspaceSession` in Flutter is a **client workbench session**, not an ACP session. In prose, call it *open-project workbench state* unless the Dart type itself is relevant.

## Project, workspace, and execution

| Prefer | Meaning | Avoid when precision matters |
| --- | --- | --- |
| **project** | Catalog-owned scope for work, settings, resources, remotes, threads, and a file tree. | workspace, unless the files specifically are meant |
| **project workspace** | The project-owned file tree available to workspace routes and sandbox file tools. Local execution uses a jailed directory; Docker mounts it at `/workspace`. | Flutter workspace; workbench |
| **workbench** | Client UI for a project: dock, files, editor/preview, threads, and chat. | workspace, when referring to the UI shell |
| **sandbox** | Control-plane execution/isolation facility and sandbox-origin tools; local or Docker-backed. | ACP `fs/*`; workspace |
| **sandbox environment** (or **environment**) | Opened per-prompt execution target with filesystem and/or executor capabilities. | sandbox configuration |
| **sandbox engine configuration** | Host-process values in `sandbox.json`: database/listen/data directory and Docker runtime/binary identity. | sandbox profile; catalog settings |
| **sandbox overlay** | Merged catalog configuration - global → project → agent definition - for image, kind, workspace root, TTL, and execution policy. | `sandbox.json` |
| **sandbox scope** | Environment/container reuse lifetime: shared, session, or project. | ACP session, without qualification |

## Tools, policy, and persistence

| Prefer | Meaning | Avoid when precision matters |
| --- | --- | --- |
| **tool origin** | Where a tool executes: `sandbox`, `mcp`, or `client`. Only sandbox-origin tools are currently live. | tool protocol |
| **sandbox tool** | Provider-neutral registry tool executed in a control-plane environment, e.g. `read_file`. | ACP filesystem tool |
| **tool gate** | Control-plane policy chain returning allow, ask, or deny before sandbox-tool execution. | `ask_user` |
| **permission request** | ACP `session/request_permission` caused by a Gate `ask` decision. | clarification |
| **clarification** | Model-initiated question through plane-owned `ask_user` and ACP elicitation. | permission request |
| **hop capture** | Scrubbed, persisted record of traffic between plane-owned hops. | log; client DevTools trace |
| **resource** | Catalog record describing material available to a project; use a more specific noun when known. | integration |
| **integration** | Configured external-service connection/capability; it is not automatically a runtime MCP tool. | provider, resource |

## Usage rules

1. Use **agent definition** for catalog records, settings, versioning, and configuration ownership. Use **ACP Agent** only for the protocol role.
2. Prefix **session** with ACP, client workbench, or sandbox scope whenever two lifetimes appear in the same paragraph.
3. Reserve **thread** for the persisted conversation and **turn** for one prompt/result cycle. A thread can outlive many ACP sessions.
4. Prefix **workspace** with project for files. Call the Flutter UI the **workbench**.
5. Name each protocol boundary: Catalog HTTP, ACP, provider wire API, or sandbox registry. Do not infer protocol from endpoint nouns.
6. Treat MCP and scoped memory as planned runtime capabilities unless a specific implemented catalog-storage feature is being described.

## Phase-2 ambiguity backlog

| Current collision | Why it is costly | Direction to evaluate |
| --- | --- | --- |
| `Agent` catalog model, Go ACP implementation, ACP role, and UI label | One unqualified word crosses storage, protocol, runtime, and product language. | Keep the UI label if useful; rename or document code roles around **Definition** and **Runtime**. |
| `Session` in ACP runtime, sandbox scope, and Flutter `ProjectWorkspaceSession` | Lifetimes and ownership differ. | Qualify prose; consider `ProjectWorkbenchState` for the Flutter type. |
| `Workspace` for file tree and Flutter surface | It hides whether an operation affects server files or client UI state. | Use **project workspace** for files and **workbench** for UI. |
| "Sandbox settings" for `sandbox.json` and catalog overlay | Sources have different ownership and application timing. | Use **engine configuration** and **sandbox overlay**. |
| "Environment" in settings/UI and opened runtime object | A stored profile and a live capability object are different. | Reserve **environment** for the opened target; call stored values an overlay/profile. |

## Sources of truth

This vocabulary follows the current [architecture](architecture.md), especially the agent-concepts, lifecycle, and execution-origin sections, and the accepted [decisions](decisions.md). Record code/documentation conflicts in the phase-2 backlog before renaming a public API, persisted field, or protocol term.
