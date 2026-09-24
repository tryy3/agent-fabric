package catalog

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type resolvedContainer struct {
	Image          string `json:"image"`
	Dockerfile     string `json:"dockerfile"`
	BuildContext   string `json:"buildContext"`
	ContainerName  string `json:"containerName"`
	IdleTTLSeconds int64  `json:"idleTTLSeconds"`
}

// AttachSandboxOptions builds Docker open options from a resolved environment.
// Callers reject a nil resource before calling this.
func AttachSandboxOptions(resolved ResolvedEnvironment, projectID, dockerRuntime, binPath string) (sandbox.OpenOptions, error) {
	if resolved.Resource == nil {
		return sandbox.OpenOptions{}, fmt.Errorf("project %q has no resource", projectID)
	}
	var spec resolvedContainer
	if err := json.Unmarshal(resolved.Resource.Spec, &spec); err != nil {
		return sandbox.OpenOptions{}, fmt.Errorf("decode resource spec: %w", err)
	}
	ttl := time.Duration(spec.IdleTTLSeconds) * time.Second
	if spec.IdleTTLSeconds == 0 {
		ttl = time.Duration(DefaultIdleTTLSeconds) * time.Second
	}
	name := strings.TrimSpace(spec.ContainerName)
	if err := sandbox.ValidateContainerName(name); err != nil {
		return sandbox.OpenOptions{}, err
	}
	mounts := make([]sandbox.Mount, 0, len(resolved.Volumes))
	for _, volume := range resolved.Volumes {
		if err := sandbox.ValidateVolumeName(volume.Name); err != nil {
			return sandbox.OpenOptions{}, err
		}
		mounts = append(mounts, sandbox.Mount{
			Source:   volume.Name,
			Target:   volume.Target,
			Type:     sandbox.MountVolume,
			ReadOnly: !volume.Write,
		})
	}
	opts := sandbox.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: resolved.WorkspaceRoot,
		Docker: &sandbox.DockerOptions{
			IdleTTL:      ttl,
			Runtime:      dockerRuntime,
			BinPath:      binPath,
			Image:        spec.Image,
			Dockerfile:   spec.Dockerfile,
			BuildContext: spec.BuildContext,
			Name:         name,
			Mounts:       mounts,
			Scope: sandbox.Scope{
				Kind:      sandbox.ScopeProject,
				ProjectID: projectID,
			},
		},
		PathPolicy: pathPolicyFromResolved(resolved),
	}
	if missingResolvedWorkspaceVolume(opts) {
		return sandbox.OpenOptions{}, fmt.Errorf("no enabled volume targets workspace root %q", opts.WorkspaceRoot)
	}
	return opts, nil
}

func pathPolicyFromResolved(resolved ResolvedEnvironment) *sandbox.PathPolicy {
	grants := make([]sandbox.PathGrant, 0, len(resolved.Volumes)+len(resolved.ExtraPaths))
	for _, volume := range resolved.Volumes {
		if !volume.Whitelisted {
			continue
		}
		target := path.Clean(strings.TrimSpace(volume.Target))
		if target == "" || target == "." || !path.IsAbs(target) {
			continue
		}
		grants = append(grants, sandbox.PathGrant{
			Path:  target,
			Read:  volume.Read,
			Write: volume.Write,
			Exec:  volume.Exec,
		})
	}
	for _, row := range resolved.ExtraPaths {
		if !flagTrue(row.Whitelisted) {
			continue
		}
		entry := path.Clean(strings.TrimSpace(stringValue(row.Path)))
		if entry == "" || entry == "." || !path.IsAbs(entry) {
			continue
		}
		grants = append(grants, sandbox.PathGrant{
			Path:  entry,
			Read:  flagTrue(row.Read),
			Write: flagTrue(row.Write),
			Exec:  flagTrue(row.Exec),
		})
	}
	return &sandbox.PathPolicy{Grants: grants}
}

func missingResolvedWorkspaceVolume(opts sandbox.OpenOptions) bool {
	if opts.Kind != "docker" || opts.Docker == nil {
		return false
	}
	for _, mount := range opts.Docker.Mounts {
		if mount.Type == sandbox.MountVolume && mount.Target == opts.WorkspaceRoot {
			return false
		}
	}
	return true
}
