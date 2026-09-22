package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/sandbox/docker"
)

var sandboxMigrationKeys = []string{
	"kind",
	"workspaceRoot",
	"image",
	"idleTTLSeconds",
	"dockerfile",
	"buildContext",
	"containerName",
	"volumes",
	"extraPaths",
}

// BackfillResources copies sandbox overlays into container resources.
// It is a no-op when projects.environment_id has already been dropped.
// Projects that already store settings.environment.resourceId are left unchanged.
func BackfillResources(ctx context.Context, pool *pgxpool.Pool, identityPrefix string) error {
	store := Open(pool)
	store.IdentityPrefix = identityPrefix

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin backfill: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hasEnvironmentID bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'projects'
			  AND column_name = 'environment_id'
		)`).Scan(&hasEnvironmentID)
	if err != nil {
		return fmt.Errorf("detect projects.environment_id: %w", err)
	}
	if !hasEnvironmentID {
		return nil
	}

	// Inserts and settings writes share this transaction so a failure rolls back.
	store.q = store.q.WithTx(tx)
	if err := backfillResources(ctx, store); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit backfill: %w", err)
	}
	return nil
}

func backfillResources(ctx context.Context, store *Store) error {
	settings, err := store.GetPlaneSettings(ctx)
	if err != nil {
		return err
	}
	global, err := DecodeOverlay(settings.Sandbox)
	if err != nil {
		return err
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return err
	}
	resolved := make([]backfillProject, 0, len(projects))
	for _, project := range projects {
		env, err := EnvironmentFromSettings(project.Settings)
		if err != nil {
			return err
		}
		if environmentResourceID(env) != "" {
			continue
		}
		item, err := store.resolveBackfillProject(ctx, project, global)
		if err != nil {
			return err
		}
		resolved = append(resolved, item)
	}

	groups, err := groupBackfillProjects(resolved)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if err := store.writeBackfillGroup(ctx, group); err != nil {
			return err
		}
	}
	if err := store.movePlaneSandboxEnvironment(ctx, settings.Sandbox); err != nil {
		return err
	}
	return store.stripAgentSandboxes(ctx)
}

type backfillProject struct {
	project          Project
	containerName    string
	image            string
	dockerfile       string
	buildContext     string
	idle             int64
	mounts           []backfillMount
	environmentID    string
	environmentName  string
	ownWorkspaceRoot json.RawMessage
	ownExtraPaths    json.RawMessage
}

type backfillMount struct {
	name        string
	target      string
	whitelisted bool
	read        bool
	write       bool
	exec        bool
}

type backfillGroup struct {
	containerName string
	displayName   string
	members       []backfillProject
}

type backfillGrant struct {
	VolumeID    string `json:"volumeId"`
	Whitelisted *bool  `json:"whitelisted,omitempty"`
	Read        *bool  `json:"read,omitempty"`
	Write       *bool  `json:"write,omitempty"`
	Exec        *bool  `json:"exec,omitempty"`
}

func (s *Store) resolveBackfillProject(ctx context.Context, project Project, global Overlay) (backfillProject, error) {
	projectOverlay, err := overlayFromSettingsJSON(project.Settings)
	if err != nil {
		return backfillProject{}, err
	}
	resolved := ResolveOverlay(global, projectOverlay)
	if resolved.Kind != nil && strings.TrimSpace(*resolved.Kind) == "local" {
		return backfillProject{}, fmt.Errorf("local sandbox cannot be migrated for project %q", project.ID)
	}

	template := DefaultContainerNameTemplate
	if resolved.ContainerName != nil && strings.TrimSpace(*resolved.ContainerName) != "" {
		template = strings.TrimSpace(*resolved.ContainerName)
	}
	containerName, err := expandBackfillName(template, project.ID)
	if err != nil {
		return backfillProject{}, err
	}

	root := DefaultWorkspaceRoot
	if resolved.WorkspaceRoot != nil && strings.TrimSpace(*resolved.WorkspaceRoot) != "" {
		root = strings.TrimSpace(*resolved.WorkspaceRoot)
	}
	mounts, err := backfillMounts(resolved.Volumes, project.ID, s.IdentityPrefix)
	if err != nil {
		return backfillProject{}, err
	}

	idle := int64(DefaultIdleTTLSeconds)
	if resolved.IdleTTLSeconds != nil {
		idle = *resolved.IdleTTLSeconds
	}
	sandboxRaw, err := SandboxFromSettings(project.Settings)
	if err != nil {
		return backfillProject{}, err
	}
	ownRoot, _, err := rawObjectField(sandboxRaw, "workspaceRoot")
	if err != nil {
		return backfillProject{}, err
	}
	ownPaths, _, err := rawObjectField(sandboxRaw, "extraPaths")
	if err != nil {
		return backfillProject{}, err
	}

	out := backfillProject{
		project:          project,
		containerName:    ApplyIdentityPrefix(containerName, s.IdentityPrefix),
		image:            strings.TrimSpace(stringValue(resolved.Image)),
		dockerfile:       strings.TrimSpace(stringValue(resolved.Dockerfile)),
		buildContext:     strings.TrimSpace(stringValue(resolved.BuildContext)),
		idle:             idle,
		mounts:           mounts,
		ownWorkspaceRoot: ownRoot,
		ownExtraPaths:    ownPaths,
	}
	shared := project.Isolation == IsolationShared &&
		project.EnvironmentID != nil &&
		strings.TrimSpace(*project.EnvironmentID) != ""
	if !shared {
		out.mounts = ensureProjectWorkspaceMount(
			out.mounts,
			root,
			ApplyIdentityPrefix(docker.ProjectVolumeName(project.ID), s.IdentityPrefix),
		)
		return out, nil
	}

	env, err := s.GetEnvironment(ctx, strings.TrimSpace(*project.EnvironmentID))
	if err != nil {
		return backfillProject{}, err
	}
	volume := docker.EnvironmentVolumeName(env.ID)
	if env.VolumeName != nil && strings.TrimSpace(*env.VolumeName) != "" {
		volume = strings.TrimSpace(*env.VolumeName)
	}
	out.environmentID = env.ID
	out.environmentName = env.Name
	out.mounts = replaceWorkspaceMount(out.mounts, root, ApplyIdentityPrefix(volume, s.IdentityPrefix))
	return out, nil
}

func expandBackfillName(template, projectID string) (string, error) {
	if strings.Contains(template, "{threadID}") || strings.Contains(template, "{random}") {
		return "", fmt.Errorf("project %q has an unstable sandbox name", projectID)
	}
	return ExpandName(template, NameVars{ProjectID: projectID})
}

func backfillMounts(rows []VolumeRow, projectID, prefix string) ([]backfillMount, error) {
	mounts := make([]backfillMount, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(stringValue(row.Name))
		target := strings.TrimSpace(stringValue(row.Target))
		if name == "" || target == "" {
			continue
		}
		expanded, err := expandBackfillName(name, projectID)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, backfillMount{
			name:        ApplyIdentityPrefix(expanded, prefix),
			target:      target,
			whitelisted: flagTrue(row.Whitelisted),
			read:        flagTrue(row.Read),
			write:       nilWriteAllowed(row.Write),
			exec:        flagTrue(row.Exec),
		})
	}
	return mounts, nil
}

// A nil write stays allowed. ExpandVolumes treats only an explicit write:false as read-only.
func nilWriteAllowed(value *bool) bool {
	return value == nil || *value
}

func replaceWorkspaceMount(mounts []backfillMount, root, volume string) []backfillMount {
	found := false
	for i := range mounts {
		if mounts[i].target != root {
			continue
		}
		mounts[i].name = volume
		found = true
	}
	if found {
		return mounts
	}
	return append(mounts, workspaceMount(volume, root))
}

func ensureProjectWorkspaceMount(mounts []backfillMount, root, volume string) []backfillMount {
	for _, mount := range mounts {
		if mount.target == root {
			return mounts
		}
	}
	return append(mounts, workspaceMount(volume, root))
}

func workspaceMount(volume, root string) backfillMount {
	return backfillMount{
		name:        volume,
		target:      root,
		whitelisted: true,
		read:        true,
		write:       true,
		exec:        true,
	}
}

func groupBackfillProjects(projects []backfillProject) ([]backfillGroup, error) {
	sharedByEnv := map[string][]backfillProject{}
	var rest []backfillProject
	for _, project := range projects {
		if project.environmentID == "" {
			rest = append(rest, project)
			continue
		}
		sharedByEnv[project.environmentID] = append(sharedByEnv[project.environmentID], project)
	}

	envIDs := make([]string, 0, len(sharedByEnv))
	for id := range sharedByEnv {
		envIDs = append(envIDs, id)
	}
	sort.Strings(envIDs)

	groups := make([]backfillGroup, 0, len(projects))
	usedNames := map[string]string{}
	for _, envID := range envIDs {
		members := sharedByEnv[envID]
		sortBackfillProjects(members)
		containerName := members[0].containerName
		if owner, ok := usedNames[containerName]; ok {
			return nil, disagreeContainerSpec(owner, members[0].project.ID)
		}
		usedNames[containerName] = members[0].project.ID
		group := backfillGroup{
			containerName: containerName,
			displayName:   members[0].environmentName,
			members:       members,
		}
		if err := group.validate(); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	isolated := map[string][]backfillProject{}
	for _, project := range rest {
		if owner, ok := usedNames[project.containerName]; ok {
			return nil, disagreeContainerSpec(owner, project.project.ID)
		}
		isolated[project.containerName] = append(isolated[project.containerName], project)
	}
	names := make([]string, 0, len(isolated))
	for name := range isolated {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		members := isolated[name]
		sortBackfillProjects(members)
		group := backfillGroup{
			containerName: name,
			displayName:   members[0].project.Name,
			members:       members,
		}
		if err := group.validate(); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (g backfillGroup) validate() error {
	if len(g.members) == 0 {
		return nil
	}
	base := g.members[0]
	for _, member := range g.members[1:] {
		if !sameContainerSpec(base, member) {
			return disagreeContainerSpec(base.project.ID, member.project.ID)
		}
	}
	return nil
}

func sameContainerSpec(a, b backfillProject) bool {
	if a.image != b.image || a.dockerfile != b.dockerfile || a.buildContext != b.buildContext || a.idle != b.idle {
		return false
	}
	return mountsEqual(a.mounts, b.mounts)
}

func mountsEqual(a, b []backfillMount) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]backfillMount(nil), a...)
	right := append([]backfillMount(nil), b...)
	sortBackfillMounts(left)
	sortBackfillMounts(right)
	for i := range left {
		if left[i].name != right[i].name || left[i].target != right[i].target {
			return false
		}
	}
	return true
}

func sortBackfillProjects(projects []backfillProject) {
	sort.Slice(projects, func(i, j int) bool {
		if !projects[i].project.CreatedAt.Equal(projects[j].project.CreatedAt) {
			return projects[i].project.CreatedAt.Before(projects[j].project.CreatedAt)
		}
		return projects[i].project.ID < projects[j].project.ID
	})
}

func sortBackfillMounts(mounts []backfillMount) {
	sort.Slice(mounts, func(i, j int) bool {
		if mounts[i].target != mounts[j].target {
			return mounts[i].target < mounts[j].target
		}
		return mounts[i].name < mounts[j].name
	})
}

func disagreeContainerSpec(a, b string) error {
	return fmt.Errorf("projects %q and %q disagree on container spec", a, b)
}

func (s *Store) writeBackfillGroup(ctx context.Context, group backfillGroup) error {
	if len(group.members) == 0 {
		return nil
	}
	oldest := group.members[0]
	ids := map[string]string{}
	volumes := make([]volumeSpec, 0, len(oldest.mounts))
	for _, mount := range oldest.mounts {
		key := mountKey(mount)
		id, ok := ids[key]
		if !ok {
			minted, err := newID("vol_")
			if err != nil {
				return err
			}
			id = minted
			ids[key] = id
		}
		volumes = append(volumes, volumeSpec{
			ID:          id,
			Enabled:     true,
			Name:        mount.name,
			Target:      mount.target,
			Whitelisted: mount.whitelisted,
			Read:        mount.read,
			Write:       mount.write,
			Exec:        mount.exec,
		})
	}
	raw, err := json.Marshal(containerSpec{
		Image:          oldest.image,
		Dockerfile:     oldest.dockerfile,
		BuildContext:   oldest.buildContext,
		ContainerName:  group.containerName,
		IdleTTLSeconds: oldest.idle,
		Volumes:        volumes,
	})
	if err != nil {
		return fmt.Errorf("encode container spec: %w", err)
	}
	displayName := strings.TrimSpace(group.displayName)
	if displayName == "" {
		displayName = oldest.project.Name
	}
	resource, err := s.CreateResource(ctx, displayName, KindContainer, raw)
	if err != nil {
		return err
	}

	for i, member := range group.members {
		var grants []backfillGrant
		if i > 0 {
			grants, err = grantsForMember(oldest.mounts, member.mounts, ids)
			if err != nil {
				return disagreeContainerSpec(oldest.project.ID, member.project.ID)
			}
		}
		if err := s.assignProjectResource(
			ctx,
			member.project,
			resource.ID,
			grants,
			member.ownWorkspaceRoot,
			member.ownExtraPaths,
		); err != nil {
			return err
		}
	}
	return nil
}

func grantsForMember(oldest, member []backfillMount, ids map[string]string) ([]backfillGrant, error) {
	base := map[string]backfillMount{}
	for _, mount := range oldest {
		base[mountKey(mount)] = mount
	}
	grants := []backfillGrant{}
	for _, mount := range member {
		flags, ok := base[mountKey(mount)]
		if !ok {
			return nil, fmt.Errorf("mount %q %q missing from resource", mount.name, mount.target)
		}
		grant := backfillGrant{VolumeID: ids[mountKey(mount)]}
		differ := false
		if mount.whitelisted != flags.whitelisted {
			grant.Whitelisted = flagPtr(mount.whitelisted)
			differ = true
		}
		if mount.read != flags.read {
			grant.Read = flagPtr(mount.read)
			differ = true
		}
		if mount.write != flags.write {
			grant.Write = flagPtr(mount.write)
			differ = true
		}
		if mount.exec != flags.exec {
			grant.Exec = flagPtr(mount.exec)
			differ = true
		}
		if differ {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func mountKey(mount backfillMount) string {
	return mount.name + "\x00" + mount.target
}

func flagPtr(value bool) *bool {
	return &value
}

func (s *Store) assignProjectResource(
	ctx context.Context,
	project Project,
	resourceID string,
	grants []backfillGrant,
	workspaceRoot, extraPaths json.RawMessage,
) error {
	env := map[string]any{
		"resourceId": resourceID,
	}
	if len(workspaceRoot) > 0 {
		env["workspaceRoot"] = json.RawMessage(workspaceRoot)
	}
	if len(extraPaths) > 0 {
		env["extraPaths"] = json.RawMessage(extraPaths)
	}
	if len(grants) > 0 {
		env["grants"] = grants
	}
	strip, err := sandboxStripPatch()
	if err != nil {
		return err
	}
	patch, err := json.Marshal(map[string]any{
		"sandbox":     json.RawMessage(strip),
		"environment": env,
	})
	if err != nil {
		return fmt.Errorf("encode project environment: %w", err)
	}
	if _, err := s.UpdateProject(ctx, project.ID, nil, nil, nil, patch); err != nil {
		return fmt.Errorf("assign project resource: %w", err)
	}
	return nil
}

func (s *Store) movePlaneSandboxEnvironment(ctx context.Context, sandbox json.RawMessage) error {
	strip, err := sandboxStripPatch()
	if err != nil {
		return err
	}
	env := map[string]json.RawMessage{}
	root, ok, err := rawObjectField(sandbox, "workspaceRoot")
	if err != nil {
		return err
	}
	if ok {
		env["workspaceRoot"] = root
	}
	paths, ok, err := rawObjectField(sandbox, "extraPaths")
	if err != nil {
		return err
	}
	if ok {
		env["extraPaths"] = paths
	}
	var envPatch json.RawMessage
	if len(env) > 0 {
		envPatch, err = json.Marshal(env)
		if err != nil {
			return fmt.Errorf("encode plane environment: %w", err)
		}
	}
	if _, err := s.PatchPlaneSettings(ctx, strip, envPatch); err != nil {
		return fmt.Errorf("move plane sandbox: %w", err)
	}
	return nil
}

func (s *Store) stripAgentSandboxes(ctx context.Context) error {
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return err
	}
	strip, err := sandboxStripPatch()
	if err != nil {
		return err
	}
	patch, err := json.Marshal(map[string]json.RawMessage{"sandbox": strip})
	if err != nil {
		return fmt.Errorf("encode agent sandbox strip: %w", err)
	}
	now := time.Now().UTC()
	for _, agent := range agents {
		hasKeys, err := sandboxHasStripKeys(agent.Settings)
		if err != nil {
			return err
		}
		if !hasKeys {
			continue
		}
		merged, err := MergeSettings(agent.Settings, patch)
		if err != nil {
			return err
		}
		if _, err := s.q.UpdateAgent(ctx, db.UpdateAgentParams{
			ID:           agent.ID,
			Name:         agent.Name,
			Description:  agent.Description,
			Version:      int32(agent.Version),
			ProviderID:   agent.ProviderID,
			DefaultModel: agent.DefaultModel,
			Settings:     merged,
			UpdatedAt:    timestamptzFromTime(now),
		}); err != nil {
			return fmt.Errorf("strip agent sandbox: %w", err)
		}
	}
	return nil
}

func sandboxStripPatch() (json.RawMessage, error) {
	keys := map[string]any{}
	for _, key := range sandboxMigrationKeys {
		keys[key] = nil
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("encode sandbox strip: %w", err)
	}
	return raw, nil
}

func sandboxHasStripKeys(settings json.RawMessage) (bool, error) {
	sandbox, err := SandboxFromSettings(settings)
	if err != nil {
		return false, err
	}
	fields, err := overlayMap(sandbox)
	if err != nil {
		return false, err
	}
	for _, key := range sandboxMigrationKeys {
		if _, ok := fields[key]; ok {
			return true, nil
		}
	}
	return false, nil
}

func rawObjectField(raw json.RawMessage, key string) (json.RawMessage, bool, error) {
	fields, err := overlayMap(raw)
	if err != nil {
		return nil, false, err
	}
	value, ok := fields[key]
	if !ok || isJSONNull(value) {
		return nil, false, nil
	}
	return value, true, nil
}
