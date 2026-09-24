---
version: alpha
name: Agent Fabric Workbench
description: A dense, project-scoped workbench for building and operating AI agents across chat, files, tools, and execution environments.
colors:
  primary: "#22D3D1"
  primary-hover: "#4DE0DE"
  primary-muted: "#123D43"
  secondary: "#38A5FF"
  background: "#071017"
  sidebar: "#08131A"
  surface: "#0B161E"
  surface-raised: "#101D26"
  surface-active: "#12303B"
  border: "#20303B"
  border-strong: "#304452"
  text-primary: "#F0F5F7"
  text-secondary: "#B7C3CB"
  text-muted: "#7F929F"
  success: "#35C97A"
  warning: "#F59E42"
  error: "#F06464"
  focus: "#55D9E2"
  transparent: "transparent"
typography:
  heading-lg:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 24px
    fontWeight: 650
    lineHeight: 1.25
    letterSpacing: -0.015em
  heading-md:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 18px
    fontWeight: 650
    lineHeight: 1.3
    letterSpacing: -0.01em
  body-md:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.5
  body-sm:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.45
  label-md:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 13px
    fontWeight: 550
    lineHeight: 1.25
  label-sm:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 12px
    fontWeight: 550
    lineHeight: 1.25
  caption:
    fontFamily: Inter, ui-sans-serif, system-ui, sans-serif
    fontSize: 11px
    fontWeight: 450
    lineHeight: 1.35
  code:
    fontFamily: JetBrains Mono, ui-monospace, SFMono-Regular, Consolas, monospace
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.55
rounded:
  none: 0px
  xs: 3px
  sm: 5px
  md: 7px
  lg: 10px
  full: 9999px
spacing:
  none: 0px
  xxs: 2px
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 24px
  xxl: 32px
  sidebar-width: 304px
  toolbar-height: 56px
  tab-height: 40px
  row-height: 36px
  panel-min-width: 240px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.background}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 10px
    height: 40px
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "{colors.background}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 10px
    height: 40px
  button-secondary:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 8px
    height: 36px
  core-view-toggle:
    backgroundColor: "{colors.transparent}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 10px
    height: 38px
  core-view-toggle-active:
    backgroundColor: "{colors.primary-muted}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 10px
    height: 38px
  workspace-tab:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    padding: 12px
    height: 40px
  workspace-tab-active:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    padding: 12px
    height: 40px
  project-row:
    backgroundColor: "{colors.transparent}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 10px
    height: 40px
  project-row-active:
    backgroundColor: "{colors.surface-active}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.md}"
    padding: 10px
    height: 40px
  context-chip:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-sm}"
    rounded: "{rounded.md}"
    padding: 8px
    height: 34px
  panel-header:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.none}"
    padding: 12px
    height: 42px
  tree-row:
    backgroundColor: "{colors.transparent}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 6px
    height: 30px
  tree-row-active:
    backgroundColor: "{colors.surface-active}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xs}"
    padding: 6px
    height: 30px
  input:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body-md}"
    rounded: "{rounded.md}"
    padding: 12px
    height: 40px
---

# Agent Fabric Design System

## Overview

Agent Fabric is a **project-scoped AI workbench**. It combines the persistent context of an AI conversation with the information density and rearrangeable panes of a developer IDE.

The interface should feel technical, dependable, and calm. It is intended for prolonged use by developers and power users, so density and scanability are more important than decorative whitespace. The visual hierarchy must make four concepts immediately distinguishable:

1. **Projects** organize and isolate work.
2. **Threads** are conversations inside the active project.
3. **Core views** are persistent, dockable capabilities such as Files, Chat, Threads, and Runs.
4. **Document views** are temporary, file-specific views such as editors, rendered previews, webviews, and image viewers.

The selected project owns the entire workspace context: threads, files, pane layout, execution environment, model, tools, MCP servers, and open document views. Only one project workspace is active at a time.

The visual character is a dark, compact workbench: graphite-blue surfaces, restrained cyan interaction color, precise icons, modest rounding, and clear but quiet separators. It should resemble a purpose-built developer tool rather than a consumer chat application.

## Colors

The palette uses tonal layers to separate regions while preserving a unified dark canvas.

- **Background (`#071017`)** is the deepest application canvas.
- **Sidebar (`#08131A`)** identifies the navigation region without creating a detached card.
- **Surface (`#0B161E`)** is the default pane and toolbar surface.
- **Raised surface (`#101D26`)** is used for controls, tab strips, inputs, and hovered regions.
- **Active surface (`#12303B`)** marks selected threads, files, and project rows.
- **Primary cyan (`#22D3D1`)** communicates selection, focus, and the highest-priority action. Use it sparingly.
- **Secondary blue (`#38A5FF`)** identifies file types, links, and supporting technical information.
- **Borders** are low-contrast structural lines. Reserve the stronger border for focus or resizing feedback.
- **Text** has three levels: primary content, secondary interface text, and muted metadata.
- **Status colors** must retain conventional meanings: green for success, amber for warning, and red for failure or destructive actions.

Do not assign a unique accent color to every pane. Project identity may use a small colored dot, but cyan remains the global interaction color.

## Typography

Use **Inter** for interface and prose, with the system sans-serif stack as fallback. Use **JetBrains Mono** for source code, terminal content, identifiers, and line-oriented technical data.

- Headings are compact and semibold rather than oversized.
- UI labels use medium weight to remain clear at 12–14px.
- Body text defaults to 14px; long assistant responses may use 15px when width permits.
- Metadata such as timestamps, counts, branches, and execution duration uses muted 11–12px text.
- Code preserves comfortable line height and must never inherit proportional typography.
- Avoid uppercase except for established abbreviations. Sentence case keeps the interface calm and readable.

## Layout

### Application regions

The desktop layout has four stacked or adjacent layers:

1. **Project navigation sidebar** on the far left.
2. **Open project tabs** across the top of the workspace.
3. **Project context bar** below the project tabs.
4. **Dockable workspace** containing core panes and document views.

The sidebar is approximately 304px wide and remains fixed while the workspace flexes. It contains the product identity, New thread action, project groups, project-scoped thread lists, Settings, connection state, and user identity.

Open projects appear as a tab strip above the workspace. Exactly one tab is active: the active project's workspace. Clicking another open tab switches the whole workspace to that project; inactive tabs are restorable shortcuts, not live content. Opening a thread (or clicking a project row) in a project that is not tabbed opens it as a new tab. Each tab has a close affordance, and a trailing + opens any project that is not tabbed yet. Closing the active tab activates a neighbor; closing the last tab leaves the empty workspace. The UI must still never mix content from two projects in the same live workspace.

### Project groups and switching

Projects appear as collapsible groups in the sidebar. Expanding a project reveals its threads. Selecting a thread in another project activates that project's workspace and restores its state, and opens that project as a workspace tab when it was not open yet. Clicking a project row does the same without picking a thread; the group chevron only expands and collapses.

Each project preserves independently:

- selected thread;
- open core views;
- pane positions, sizes, and tab groupings;
- open document views;
- model and provider selection;
- execution environment;
- enabled tools and MCP servers;
- project files and runtime status.

Project switching must be explicit and atomic. Never leave files from one project visible beside chat or runtime controls from another. If a destructive or long-running operation belongs to a background project, identify that project in notifications and activity indicators.

### Project context bar

The context bar communicates the active project's operational configuration without requiring a trip to Settings. It contains compact controls for:

- execution environment;
- active model/provider;
- enabled tool count and access;
- enabled MCP server count and access;
- runtime or connection state when relevant.

These controls may open popovers for quick inspection or changes. Changes are scoped to the active project unless the UI explicitly labels them as global defaults.

Core-view toggles occupy the right end of this bar. They are visible buttons, not navigation destinations.

### Core views

Initial core views are **Threads**, **Files**, **Chat**, and **Runs**. The set is extensible.

A core-view button toggles the corresponding dockable pane:

- active styling means the view is currently visible somewhere in the layout;
- pressing an active toggle closes or hides that view;
- pressing an inactive toggle restores it to its last docked position, or to a sensible default if no saved position exists;
- a core view may be moved, resized, combined into a tab group, or closed from its pane header;
- closing a pane updates its toggle state;
- toggling a view must not navigate away from the active thread or document.

The toggle is a visibility and layout control. It is not the only possible way to focus an already-open pane: selecting its tab or using a keyboard shortcut should also work.

### Files core view

The file browser follows a compact IDE-style tree rather than a table or card list.

- Folders expand inline with disclosure chevrons.
- Indentation communicates hierarchy using a consistent 16px depth step.
- File-type icons aid scanning but do not replace filenames.
- Modified, generated, ignored, and read-only states appear as restrained trailing indicators.
- The pane header provides compact actions such as new file, new folder, refresh, collapse all, and overflow.
- Single click selects; double click or Enter opens a persistent document tab. A preview-open behavior may be supported if it is visually distinguishable.
- The tree supports keyboard navigation and retains expansion state per project.

### Document views

Document views are **not core views** and do not appear in the global core-view toggle group. They are created as a result of opening content.

Examples include:

- source editor for Markdown, HTML, JavaScript, JSON, and other text formats;
- rendered Markdown preview;
- HTML webview;
- image viewer;
- PDF or structured-data viewer;
- diff, log, or artifact viewer added later.

Document views use tabs inside the central workbench. They can be closed, reordered, split, and moved between tab groups. Related views can coexist, such as `architecture.md` beside its rendered Preview. The tab must communicate the resource name, view type when necessary, modified state, and close action.

Opening a document must not replace the active project or thread. Document tab state belongs to the project workspace and is restored when returning to that project.

### Responsive behavior

Agent Fabric is desktop-first.

- Above 1440px, allow three useful columns: Files, document workbench, and Chat/Runs.
- Between 1024px and 1439px, preserve the sidebar and collapse the least recently used right-side core pane into a tab group.
- Below 1024px, allow the sidebar to collapse and show one primary workspace group at a time; preserve all hidden pane state.
- Never shrink Files below 240px or Chat below 280px. Prefer tabbing or hiding a pane to making it unusable.

Spacing follows a 4px base with 8px and 12px as the most common component intervals. Dense tree rows use 30px height, general rows use 36px, and major toolbars use approximately 56px.

## Elevation & Depth

Depth is expressed primarily through **tonal layering and one-pixel separators**, not shadows.

- The application canvas is darkest.
- Sidebar, panes, and toolbars use subtly different surfaces.
- Active tabs connect visually to their content surface.
- Floating popovers may use a soft shadow, but docked panes do not.
- A resize handle becomes clearer on hover or drag without becoming permanently dominant.
- Focus uses the cyan focus color and must remain visible against every surface.

Avoid nesting multiple rounded cards inside panes. Conversation messages may use limited containment, while long assistant content should usually flow directly in the reading surface.

## Shapes

The shape language is technical and restrained.

- Pane boundaries and tab groups are mostly square.
- Controls use 5–7px corner radii.
- Primary actions may use up to 10px radius.
- Status dots, avatars, and compact numeric badges may be circular.
- Do not apply pill shapes to ordinary buttons merely for decoration.
- Separators, active-tab underlines, and tree hierarchy should remain precise and thin.

## Components

### Project navigation

Project rows combine a disclosure control, optional project color marker, project name, and overflow or state affordance. The active project receives a tonal fill and a slim cyan indicator. Its threads appear immediately below it and align as children of the group.

Thread rows show a concise title and may show secondary metadata such as time, unread state, or running activity. Truncate rather than wrap long thread titles in the navigation list.

### Project tabs

Open projects each get a tab in the strip above the workspace. The active tab connects visually to the context bar below it and carries project identity: the project's color marker and name. Inactive tabs recede tonally. Tabs are workspace switches — selecting one replaces the whole workspace — so they are visually stronger than document tabs. Each tab has a close affordance, and a trailing + opens any project that is not tabbed yet. Closing the last tab leaves an explicit "No project" state; open tabs, their order, and the active tab persist across launches.

### Context controls

Environment, model, Tools, and MCP use compact secondary controls with leading icons and clear dropdown affordances. Counts appear as small badges. A warning or disconnected state changes the status indicator, not the entire toolbar color.

### Core-view toggles

Core-view toggles form a related control group while remaining individually clickable. An active toggle uses cyan border/text or a muted cyan fill. Multiple toggles may be active simultaneously. Include icons and labels on desktop; icons alone require tooltips and accessible names.

### Pane headers

Every dockable core pane has a header containing its title, relevant local actions, and overflow menu. Close, undock, maximize, and tab-group actions appear consistently. Drag regions must be discoverable without consuming excessive space.

### Document tabs

Document tabs live within the workbench, below the project context bar. Active tabs have a high-contrast label and cyan top or bottom indicator. Modified files show a dot or `M` marker without relying only on color. Preview tabs use a view icon and retain a relationship to their source document.

### Chat composer

The composer remains anchored to the bottom of the Chat pane. It may expose attachments, context selection, tool access, voice, model override, and send. Project-scoped model and tool defaults come from the context bar; overrides in the composer must be clearly temporary or thread-specific.

### Interaction states

All interactive components require default, hover, active/selected, focus-visible, disabled, loading, and error states where applicable. Hover must not be the only indication of interactivity. Focus-visible styling must support full keyboard operation.

## Do's and Don'ts

- **Do** preserve project isolation across files, chat, tools, models, environments, document tabs, and pane layouts.
- **Do** treat projects and threads as navigation.
- **Do** treat core-view buttons as pane visibility controls.
- **Do** treat editors, previews, webviews, and image viewers as resource-created document views.
- **Do** keep important project configuration visible in the context bar.
- **Do** restore each project's workspace exactly enough that switching feels inexpensive.
- **Do** keep open project tabs switchable, closable, and persistent across launches.
- **Do** use tonal contrast, spacing, and typography before adding stronger borders.
- **Do** support keyboard navigation, resizable panes, and accessible tooltips.
- **Do** keep status and scope visible before operations that can modify files or execute tools.
- **Don't** place core views in the sidebar as if they were destinations.
- **Don't** mix content from different projects in the same active workspace.
- **Don't** imply several simultaneous live workspaces; open tabs are shortcuts and only the active one is live.
- **Don't** make every region the same background tone.
- **Don't** turn all content into rounded cards.
- **Don't** use cyan on every icon or label; reserve it for interaction, selection, and focus.
- **Don't** hide environment, model, tool, or MCP scope behind project settings alone.
- **Don't** confuse a document Preview tab with a global Preview core view.
- **Don't** silently discard pane or document state when switching projects.