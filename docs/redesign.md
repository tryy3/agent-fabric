# Agent Fabric Redesign Prompt

Redesign the existing Agent Fabric interface using `DESIGN.md` as the visual source of truth and the approved mockup as the target design direction.

This is a redesign of an existing application. Preserve its functional architecture, especially the dockable workspace and project isolation. Do not reduce it to a conventional chat application or a static three-column layout.

## Current Interface

The existing interface has:

- a narrow icon rail on the far left containing Chat, Threads, Files, and Settings;
- separate dockable panes for Threads, Files, and Chat;
- a project selector inside the Threads pane;
- a file pane with workspace actions;
- a large Chat pane with the conversation and composer;
- panes that can be moved, resized, closed, and combined into tab groups, similarly to VS Code;
- project switching that replaces the entire workspace context.

Chat, Threads, and Files currently appear like navigation links in the left rail, but they are actually toggles for dockable panes. Settings is different: it opens a separate settings page.

The existing design makes these roles difficult to understand because the sidebar, pane backgrounds, tab strips, and content surfaces use very similar colors with weak separation. Projects, threads, core panes, and application navigation therefore appear to be at roughly the same hierarchical level.

## Redesign Goal

Create a clearer and more polished project-scoped AI workbench. The interface should combine:

- the project and conversation discoverability of ChatGPT-style navigation;
- the movable, resizable, tabbed workspace behavior of VS Code;
- visible project-specific runtime information;
- a clear distinction between navigation, persistent core views, and temporary file-created views.

The redesign must make frequent project switching easier without weakening project isolation.

## Core Mental Model

Use this hierarchy:

```text
Application
├── Global navigation and settings
└── Active project workspace
    ├── Project-scoped threads
    ├── Project context
    │   ├── Execution environment
    │   ├── Model/provider
    │   ├── Tools
    │   └── MCP servers
    ├── Dockable core views
    │   ├── Threads
    │   ├── Files
    │   ├── Chat
    │   └── Runs
    └── Dynamic document views
        ├── Editors
        ├── Previews and webviews
        ├── Image/document viewers
        └── Future resource-specific views
```

Projects and threads are navigation. Core views control the workspace layout. Document views are opened from resources such as files.

## Project Navigation and Switching

Replace the narrow Chat/Threads/Files icon rail with a persistent navigation sidebar.

The sidebar should contain:

1. Agent Fabric identity.
2. A prominent New thread action.
3. A Projects section.
4. Collapsible project groups such as AI Harness, Keyboard, and AI Benchmarks.
5. Threads nested beneath their project.
6. Settings, connection information, and account controls near the bottom.

Projects should look and behave like first-class groups. Expanding a group exposes its threads. Selecting another project or one of its threads switches the complete workspace to that project.

Keep the existing project isolation model. A project owns:

- its threads and selected conversation;
- its files;
- its open document views;
- its pane arrangement and visible core views;
- its execution environment;
- its selected model/provider;
- its enabled tools and MCP servers;
- its runtime activity and relevant state.

Only one project workspace is active at a time. Open projects are represented as a tab strip at the top of the work area; the active project's tab is the live one, and inactive tabs are shortcuts to their project's workspace. Do not imply that multiple project workspaces are simultaneously sharing the same canvas.

When switching projects, restore that project's previous workspace state. This makes switching inexpensive while keeping the underlying contexts isolated.

## Project Tabs

Show open projects as a tab strip across the top of the main work area. In the reference direction the active tab is `AI Harness`.

A project tab is not an ordinary document tab. It scopes everything below it, so it should be visually distinct from tabs for files, previews, or images. Only the active tab connects to the context bar below it; inactive tabs recede tonally.

Project tabs behave like workspace switches:

- clicking an open tab activates that project and replaces the whole workspace;
- opening a thread — or clicking a project row in the sidebar — opens that project as a tab when it is not open yet, then activates it;
- a trailing + opens any project that is not tabbed yet;
- every tab has a close affordance; closing the active tab activates a neighbor, and closing the last tab leaves an explicit empty workspace;
- open tabs, their order, and the active tab persist across launches, and closing a tab never discards that project's saved workspace state.

The project name should remain visible both in the expanded sidebar group and in its workspace tab. This intentional repetition confirms scope before the user edits files or runs tools.

## Project Context Bar

Directly below the active project tab, add a compact project context bar.

Show important project-specific configuration such as:

- execution environment, for example `Docker · Ubuntu`;
- selected model/provider, for example `Qwen 3.6`;
- enabled Tools with a count;
- enabled MCP servers with a count;
- runtime or connection state where useful.

These should be interactive compact controls. They may open popovers for inspection or quick changes. Their values always refer to the active project unless explicitly labelled as global defaults.

This bar reduces the need to open Settings merely to inspect or change frequently used project configuration.

## Core Views

Move the core-view controls to the top-right side of the project context bar.

Initially support:

- Threads;
- Files;
- Chat;
- Runs.

The set should remain extensible because more core views may be added later.

These controls are toggles, not navigation links:

- active means the corresponding view is visible somewhere in the workspace;
- clicking an active toggle hides or closes that pane;
- clicking an inactive toggle restores it;
- restoring should use its last docked location when possible;
- closing a pane from its header must update the matching toggle;
- toggling a view must not change the selected project, thread, or document;
- multiple core views may be active simultaneously.

Core panes retain the existing ability to move, resize, close, and combine into tab groups. Do not hard-code the exact arrangement shown in the mockup; treat it as a strong default layout.

Remove Chat, Threads, and Files from the persistent left-side application navigation. Settings remains navigation because it opens a different page.

## Threads

Display common project threads directly beneath their project in the sidebar, similar to conversations grouped under a project.

This makes the separate Threads pane unnecessary in the default layout, but Threads may remain an optional core view for a richer thread-management experience such as filtering, bulk operations, metadata, branches, or archived conversations.

Avoid showing the same simple thread list in two places at once without a clear difference in capability.

## Files

Use a compact VS Code-style file explorer for the Files core view.

Requirements:

- hierarchical tree with inline folder expansion;
- disclosure chevrons and consistent indentation;
- compact rows suitable for large projects;
- recognizable file and folder icons;
- indicators for modified or special states;
- compact pane actions for new file, new folder, refresh, collapse, and overflow;
- keyboard navigation;
- expansion state retained per project.

The Files view is a dockable core pane. It is not the same thing as the editor or viewer created when a file is opened.

## Dynamic Document Views

Opening a file should create an appropriate document view inside the workspace.

Examples:

- Markdown, HTML, JavaScript, JSON, and similar text files open in an editor;
- Markdown can also open a rendered Preview;
- HTML can open a webview;
- images open in an image viewer;
- future formats may open PDFs, structured-data viewers, diffs, logs, or other specialized views.

These are not core views and must not appear in the top-right core-view toggle group.

Document views use tabs within the central workbench. They can be:

- closed;
- reordered;
- split beside another document view;
- moved into another compatible tab group;
- restored as part of the project's workspace state.

Show `architecture.md` in a source editor beside its rendered Preview to demonstrate this distinction. The editor and Preview are document views created by opening a file, while Files and Chat remain core views.

Opening or closing a document must not change the active project or thread.

## Default Workspace Layout

Use the approved mockup as the initial layout:

- persistent project/thread navigation sidebar on the far left;
- Files core pane immediately to its right;
- central document workbench with `architecture.md` source and rendered Preview;
- Chat core pane on the right;
- active project tab above the workspace;
- project context controls beneath the project tab;
- core-view toggles aligned to the upper-right.

The central document workbench should receive the most width. Files should be narrow but comfortably navigable. Chat should remain usable for conversation and its composer.

This arrangement is a default, not a restriction. Users can rearrange panes using the existing docking system.

## Visual Hierarchy

Follow `DESIGN.md` precisely.

Use tonal layering, spacing, tab treatment, and restrained separators to distinguish:

- the global/project navigation sidebar;
- the active project tab;
- the project context bar;
- dockable core panes;
- dynamic document tab groups;
- content inside each pane.

Do not solve every hierarchy problem with strong borders. The original interface's regions blended together, but the replacement should remain calm and cohesive rather than becoming a grid of outlined boxes.

Use cyan selectively for active state, focus, and primary action. Avoid applying the accent to every icon and label.

Keep the design dense, technical, and suitable for prolonged developer use. Avoid oversized headings, excessive whitespace, consumer-style message bubbles, glassmorphism, strong gradients, and excessive rounded cards.

## Original-to-Target Changes

| Original interface | Target interface | Purpose |
| --- | --- | --- |
| Narrow icon rail for Chat, Threads, Files, and Settings | Project/thread navigation sidebar; Settings remains a destination | Separate navigation from pane controls. |
| Chat, Threads, and Files appear as navigation items | Threads, Files, Chat, and Runs appear as top-right core-view toggles | Accurately communicate that they control dockable views. |
| Project selector lives inside the Threads pane | Projects are persistent, collapsible groups in the sidebar | Make project scope primary and switching quicker. |
| Threads require a dedicated pane in the default layout | Threads appear under the active project | Reduce pane use and support ChatGPT-like discoverability. |
| Project switch replaces the workspace with little persistent indication | Active project tab and context bar identify the complete workspace scope | Reduce user mistakes when files, execution, and tools differ by project. |
| Model and runtime controls are scattered or composer-local | Environment, model, Tools, and MCP appear in the project context bar | Make important project configuration visible and quickly accessible. |
| Sparse file area | Dense IDE-style file tree | Improve real project navigation. |
| Open files are not clearly separated from persistent panes | Files create dynamic editor, Preview, webview, or viewer tabs | Establish a scalable view model for different resource types. |
| Most surfaces share the same background | Sidebar, toolbars, panes, and selected content use restrained tonal layers | Improve orientation and scanability. |
| Workspace state is treated as one current arrangement | Pane geometry and open documents are restored per project | Make project switching less disruptive without mixing contexts. |

## Important Constraints

- Do not turn projects into loose chat folders. A project remains the isolation boundary for files, execution, configuration, tools, and conversations.
- Do not remove the docking, resizing, tabbing, or pane-closing functionality.
- Do not assume the mockup's pane arrangement is permanent.
- Do not treat editors, previews, or image viewers as globally toggled core views.
- Do not show content from two different projects in the same active workspace.
- Do not make global and project-specific settings visually indistinguishable.
- Do not discard existing user state during project switching.
- Preserve existing functionality unless this prompt explicitly relocates it.

## Behavioral Acceptance Criteria

- Selecting a thread under another project activates that project before displaying its thread.
- The project tabs and context bar always match the active sidebar project.
- Opening a thread or project row for an untabbed project opens it as a tab; tab clicks switch the whole workspace; closing the active tab activates a neighbor; closing the last tab shows the empty workspace.
- Switching projects restores the target project's selected thread, pane layout, open documents, environment, model, Tools, and MCP state.
- Toggling Files, Chat, Threads, or Runs only affects pane visibility.
- Closing a core pane updates its corresponding toggle.
- Reopening a core pane restores its previous docked position when possible.
- Opening a text file creates an editor document tab.
- Opening Markdown Preview creates a related Preview tab or split.
- Opening an image creates an image-viewer document tab rather than a new core view.
- A long-running operation identifies its originating project even if the user switches projects.
- The redesigned interface remains fully operable with the keyboard.
- Status and selection do not rely on color alone.

## Deliverable

Redesign the existing interface in place. Begin by identifying the existing components and state that correspond to projects, threads, core-view visibility, docking layout, file opening, runtime configuration, and Settings navigation. Reuse the current behavior where possible, then restructure presentation and state boundaries to match this prompt.

Use:

- `DESIGN.md` for normative visual tokens and component styling;
- the approved mockup for spatial direction and relative hierarchy;
- this document for information architecture, behavior, migration intent, and acceptance criteria.

Where the mockup conflicts with this written prompt, follow this prompt. Where implementation details are unspecified, preserve existing behavior and choose the least destructive migration path.