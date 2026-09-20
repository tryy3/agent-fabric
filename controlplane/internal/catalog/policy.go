package catalog

import (
	"path"
	"path/filepath"
	"strings"
)

type PathGrant struct {
	Path  string
	Read  bool
	Write bool
	Exec  bool
}

func OverlayPathPolicy(overlay Overlay, kind, overlayRoot, hostRoot string) []PathGrant {
	overlayRoot = path.Clean(strings.TrimSpace(overlayRoot))
	if overlayRoot == "" {
		overlayRoot = DefaultWorkspaceRoot
	}
	out := make([]PathGrant, 0, len(overlay.Volumes)+len(overlay.ExtraPaths))
	for _, row := range overlay.Volumes {
		if !flagTrue(row.Whitelisted) {
			continue
		}
		target := path.Clean(strings.TrimSpace(stringValue(row.Target)))
		if target == "" || target == "." {
			continue
		}
		if kind == "local" {
			if target != overlayRoot {
				continue
			}
			host := strings.TrimSpace(hostRoot)
			if host == "" {
				continue
			}
			target = filepath.Clean(host)
		}
		out = append(out, PathGrant{
			Path:  target,
			Read:  flagTrue(row.Read),
			Write: flagTrue(row.Write),
			Exec:  flagTrue(row.Exec),
		})
	}
	for _, row := range overlay.ExtraPaths {
		if !flagTrue(row.Whitelisted) {
			continue
		}
		entry := strings.TrimSpace(stringValue(row.Path))
		if entry == "" {
			continue
		}
		if kind == "local" {
			if !filepath.IsAbs(entry) {
				continue
			}
			entry = filepath.Clean(entry)
		} else {
			if !path.IsAbs(entry) {
				continue
			}
			entry = path.Clean(entry)
		}
		out = append(out, PathGrant{
			Path:  entry,
			Read:  flagTrue(row.Read),
			Write: flagTrue(row.Write),
			Exec:  flagTrue(row.Exec),
		})
	}
	return out
}

func flagTrue(value *bool) bool {
	return value != nil && *value
}
