package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

const (
	skillFile     = "SKILL.md"
	maxExtraFiles = 200
	maxDepth      = 4
)

// LoadDir reads every skill folder directly under root. A missing root is no
// skills. A folder that fails to parse is skipped and reported as a Problem.
func LoadDir(ctx context.Context, r Reader, root string, source Source) ([]Skill, []Problem, error) {
	entries, err := r.ReadDir(ctx, root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("list %s skills in %s: %w", source, root, err)
	}
	var skills []Skill
	var problems []Problem
	for _, e := range entries {
		if !e.IsDir || strings.HasPrefix(e.Name, ".") {
			continue
		}
		dir := path.Join(root, e.Name)
		raw, readErr := r.ReadFile(ctx, path.Join(dir, skillFile))
		if readErr != nil {
			if errors.Is(readErr, fs.ErrNotExist) {
				continue
			}
			problems = append(problems, Problem{Source: source, Path: dir, Error: readErr.Error()})
			continue
		}
		s, parseErr := parseSkillMD(raw, e.Name)
		if parseErr != nil {
			problems = append(problems, Problem{Source: source, Path: dir, Error: parseErr.Error()})
			continue
		}
		files, listErr := listExtraFiles(ctx, r, dir)
		if listErr != nil {
			problems = append(problems, Problem{Source: source, Path: dir, Error: listErr.Error()})
			continue
		}
		s.Source = source
		s.Path = dir
		s.Files = files
		s.Hash = hashSkill(raw, files)
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, problems, nil
}

func listExtraFiles(ctx context.Context, r Reader, dir string) ([]string, error) {
	var files []string
	var walk func(rel string, depth int) error
	walk = func(rel string, depth int) error {
		entries, err := r.ReadDir(ctx, path.Join(dir, rel))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name, ".") {
				continue
			}
			child := path.Join(rel, e.Name)
			if e.IsDir {
				if depth+1 < maxDepth {
					if err := walk(child, depth+1); err != nil {
						return err
					}
				}
				continue
			}
			if child == skillFile {
				continue
			}
			if len(files) >= maxExtraFiles {
				return fmt.Errorf("more than %d files", maxExtraFiles)
			}
			files = append(files, child)
		}
		return nil
	}
	if err := walk(".", 0); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
