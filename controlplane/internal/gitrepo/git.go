package gitrepo

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"
	"unicode"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

const (
	authorName  = "Agent Fabric"
	authorEmail = "agent-fabric@local"
	seedIgnore  = `.DS_Store
.env
.env.*
*.pem
*.key
*.p12
*.pfx
id_rsa
id_rsa.pub
.ssh/
`
	gitTimeout = 30 * time.Second
)

var ErrNotRepo = errors.New("not a git repository")

type Commit struct {
	SHA         string
	Message     string
	CommittedAt time.Time
}

func EnsureRepo(ctx context.Context, exec sandboxcore.Executor, fsys sandboxcore.FS) error {
	if exec == nil {
		return fmt.Errorf("git executor is required")
	}
	if err := initRepo(ctx, exec); err != nil {
		return err
	}
	if err := configureIdentity(ctx, exec); err != nil {
		return err
	}
	if fsys != nil {
		if err := seedGitignore(ctx, fsys); err != nil {
			return err
		}
	}
	return commitGitignoreIfStaged(ctx, exec)
}

func commitGitignoreIfStaged(ctx context.Context, exec sandboxcore.Executor) error {
	if _, err := runGit(ctx, exec, "add", "--", ".gitignore"); err != nil {
		return err
	}
	staged, err := runGit(ctx, exec, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(staged) == "" {
		return nil
	}
	_, err = runGit(ctx, exec, "commit", "--no-gpg-sign", "-m", "chore: seed .gitignore")
	return err
}

func seedGitignore(ctx context.Context, fsys sandboxcore.FS) error {
	_, err := fsys.Stat(ctx, ".gitignore")
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) && !strings.Contains(strings.ToLower(err.Error()), "not exist") &&
		!strings.Contains(strings.ToLower(err.Error()), "no such") {
		// Missing file from jailed FS may not wrap fs.ErrNotExist; still try write if Stat failed.
		if err != nil && fileExistsError(err) {
			return err
		}
	}
	return fsys.WriteFile(ctx, ".gitignore", []byte(seedIgnore))
}

func fileExistsError(err error) bool {
	msg := strings.ToLower(err.Error())
	return !strings.Contains(msg, "not exist") && !strings.Contains(msg, "no such") && !errors.Is(err, fs.ErrNotExist)
}

func initRepo(ctx context.Context, exec sandboxcore.Executor) error {
	inside, err := runGit(ctx, exec, "rev-parse", "--is-inside-work-tree")
	if err == nil && strings.TrimSpace(inside) == "true" {
		return nil
	}
	if _, err := runGit(ctx, exec, "init", "-b", "main"); err != nil {
		if _, err2 := runGit(ctx, exec, "init"); err2 != nil {
			return fmt.Errorf("git init: %w", err2)
		}
		_, _ = runGit(ctx, exec, "symbolic-ref", "HEAD", "refs/heads/main")
	}
	return nil
}

func configureIdentity(ctx context.Context, exec sandboxcore.Executor) error {
	if _, err := runGit(ctx, exec, "config", "user.name", authorName); err != nil {
		return err
	}
	if _, err := runGit(ctx, exec, "config", "user.email", authorEmail); err != nil {
		return err
	}
	_, _ = runGit(ctx, exec, "config", "commit.gpgsign", "false")
	return nil
}

func CommitIfDirty(ctx context.Context, exec sandboxcore.Executor, message string) (string, bool, error) {
	if _, err := runGit(ctx, exec, "add", "-A"); err != nil {
		return "", false, err
	}
	status, err := runGit(ctx, exec, "status", "--porcelain")
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(status) == "" {
		return "", false, nil
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "checkpoint"
	}
	if _, err := runGit(ctx, exec, "commit", "--no-gpg-sign", "-m", message); err != nil {
		return "", false, err
	}
	sha, err := RevParse(ctx, exec, "HEAD")
	if err != nil {
		return "", false, err
	}
	return sha, true, nil
}

func Log(ctx context.Context, exec sandboxcore.Executor) ([]Commit, error) {
	out, err := runGit(ctx, exec, "log", "--pretty=format:%H%x00%s%x00%cI", "--max-count=100")
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "bad default revision") ||
			strings.Contains(err.Error(), "unknown revision") {
			return []Commit{}, nil
		}
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return []Commit{}, nil
	}
	lines := strings.Split(out, "\n")
	commits := make([]Commit, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\x00")
		if len(parts) < 2 {
			continue
		}
		c := Commit{SHA: parts[0], Message: parts[1]}
		if len(parts) >= 3 {
			if ts, perr := time.Parse(time.RFC3339, parts[2]); perr == nil {
				c.CommittedAt = ts.UTC()
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

func Diff(ctx context.Context, exec sandboxcore.Executor, from, to string) (string, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" {
		return "", fmt.Errorf("from is required")
	}
	if err := validateRev(from); err != nil {
		return "", err
	}
	args := []string{"diff", "--no-color", "--find-renames"}
	if to == "" {
		args = append(args, from)
	} else {
		if err := validateRev(to); err != nil {
			return "", err
		}
		args = append(args, from, to)
	}
	out, err := runGit(ctx, exec, args...)
	if err != nil {
		return "", err
	}
	return out, nil
}

func CheckoutForce(ctx context.Context, exec sandboxcore.Executor, sha string) error {
	if err := validateRev(sha); err != nil {
		return err
	}
	if _, err := runGit(ctx, exec, "checkout", "--force", sha); err != nil {
		return err
	}
	_, err := runGit(ctx, exec, "clean", "-fd")
	return err
}

func TagAnnotated(ctx context.Context, exec sandboxcore.Executor, name, message string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("tag name is required")
	}
	if strings.ContainsAny(name, " \t\n") || strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid tag name")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = name
	}
	_, err := runGit(ctx, exec, "tag", "-a", name, "-m", message)
	return err
}

func RevParse(ctx context.Context, exec sandboxcore.Executor, rev string) (string, error) {
	if err := validateRev(rev); err != nil {
		return "", err
	}
	out, err := runGit(ctx, exec, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		out, err = runGit(ctx, exec, "rev-parse", "--verify", rev)
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(out), nil
}

func AgentCommitMessage(threadTitle, threadID, userPrompt string) string {
	label := strings.TrimSpace(threadTitle)
	if label == "" {
		label = strings.TrimSpace(threadID)
	}
	if label == "" {
		label = "thread"
	}
	sum := sha1.Sum([]byte(userPrompt))
	short := hex.EncodeToString(sum[:])[:7]
	return fmt.Sprintf("agent: %s (%s)", label, short)
}

func validateRev(rev string) error {
	rev = strings.TrimSpace(rev)
	if rev == "" || strings.HasPrefix(rev, "-") {
		return fmt.Errorf("invalid revision")
	}
	if strings.ContainsAny(rev, " \t\n") {
		return fmt.Errorf("invalid revision")
	}
	for _, r := range rev {
		if r > unicode.MaxASCII {
			return fmt.Errorf("invalid revision")
		}
	}
	return nil
}

func runGit(ctx context.Context, exec sandboxcore.Executor, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd := append([]string{"git"}, args...)
	res, err := exec.Run(ctx, sandboxcore.ExecRequest{
		Cmd:     cmd,
		WorkDir: ".",
		Timeout: gitTimeout,
	})
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(res.Stdout))
		}
		if msg == "" {
			msg = fmt.Sprintf("exit %d", res.ExitCode)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	out := bytes.ReplaceAll(res.Stdout, []byte("\r\n"), []byte("\n"))
	return string(out), nil
}
