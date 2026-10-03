package gate

import (
	"path"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
)

// Rule IDs for run_command decisions.
const (
	RuleCommandLocalDeny   = "rules.command_local_deny"
	RuleCommandCwdEscape   = "rules.command_cwd_escape"
	RuleCommandForbidden   = "rules.command_forbidden"
	RuleCommandSensitive   = "rules.command_sensitive_path"
	RuleCommandDestructive = "rules.command_destructive_ask"
	RuleCommandOpaque      = "rules.command_opaque_ask"
	RuleCommandSafe        = "rules.command_safe_allow"
	RuleCommandGranted     = "rules.command_granted"
	RuleCommandAsk         = "rules.command_ask"
)

// evaluateCommand classifies one run_command call. Tiers, first match wins:
//
//  1. deny: host execution, cwd escape, privilege/host-control programs,
//     destructive commands aimed at sensitive paths or .git
//  2. ask without a session grant: destructive or unclassifiable commands
//  3. allow: a small read-only allowlist whose path arguments stay in the root
//  4. ask (session grant by command prefix): everything else, including
//     build, test and install tooling
func evaluateCommand(req Request) (Decision, error) {
	args, err := command.Decode(req.Args)
	if err != nil {
		return Decision{Kind: Deny, Reason: err.Error(), RuleID: "rules.bad_args"}, nil
	}
	base := Decision{Command: args.Command, Cwd: args.Cwd}
	deny := func(rule, reason string) (Decision, error) {
		d := base
		d.Kind, d.RuleID, d.Reason = Deny, rule, reason
		return d, nil
	}

	if req.EnvKind != "docker" {
		return deny(RuleCommandLocalDeny, "command execution is only available in Docker environments")
	}
	cwd := args.Cwd
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	if d := evaluatePath(req, cwd, sandboxcore.PathRead); d.Kind != Allow {
		return deny(RuleCommandCwdEscape, "working directory must stay inside the project: "+cwd)
	}

	prog, rest := normalizeProgram(args.Command)
	if forbiddenPrograms[prog] {
		return deny(RuleCommandForbidden, prog+" is not permitted")
	}

	hardAsk := func(rule, reason string) (Decision, error) {
		d := base
		d.Kind, d.RuleID, d.Reason, d.NoSessionGrant = Ask, rule, reason, true
		return d, nil
	}

	if destructivePrograms[prog] || destructiveInvocation(prog, rest) {
		if p := sensitiveArg(req, cwd, rest); p != "" {
			return deny(RuleCommandSensitive, "command targets a protected path: "+p)
		}
		d, _ := hardAsk(RuleCommandDestructive, "command can destroy or change data and needs confirmation: "+prog)
		if sweepingFlags(rest) {
			d.Risk = riskDestructiveWide
		}
		return d, nil
	}
	if opaque := opaqueReason(prog, rest); opaque != "" {
		return hardAsk(RuleCommandOpaque, opaque)
	}

	if safeInvocation(prog, rest) {
		for _, a := range rest {
			if !pathLike(a) {
				continue
			}
			if d := evaluatePath(req, a, sandboxcore.PathRead); d.Kind != Allow {
				ask := base
				ask.Kind, ask.RuleID, ask.NoSessionGrant = Ask, "rules.path_ask", true
				ask.Reason, ask.Path = "path is outside the project: "+a, a
				return ask, nil
			}
		}
		d := base
		d.Kind, d.RuleID = Allow, RuleCommandSafe
		return d, nil
	}

	key := GrantKey(args.Command)
	d := base
	d.GrantKey = key
	for _, granted := range req.CommandGrants {
		if granted == key {
			d.Kind, d.RuleID = Allow, RuleCommandGranted
			return d, nil
		}
	}
	d.Kind, d.RuleID = Ask, RuleCommandAsk
	d.Reason = "run " + key
	return d, nil
}

// GrantKey is the normalized command prefix a session grant covers, such as
// "npm test", "npm run build", "go test" or "python script.py".
func GrantKey(argv []string) string {
	prog, rest := normalizeProgram(argv)
	if len(argv) > 0 && prog == "" {
		prog = argv[0]
	}
	if !subcommandPrograms[prog] {
		return prog
	}
	parts := []string{prog}
	sub := firstNonFlag(rest)
	if sub == "" {
		return prog
	}
	parts = append(parts, sub)
	if runVerbs[prog+" "+sub] {
		after := rest[indexOf(rest, sub)+1:]
		if target := firstNonFlag(after); target != "" {
			parts = append(parts, target)
		}
	}
	return strings.Join(parts, " ")
}

var systemBinDirs = map[string]bool{
	"/bin": true, "/usr/bin": true, "/usr/local/bin": true, "/sbin": true, "/usr/sbin": true,
}

// normalizeProgram returns the program name used for classification and the
// remaining arguments. A program given by a non-system path ("./run.sh") is
// returned as-is so it never matches a known name.
func normalizeProgram(argv []string) (string, []string) {
	if len(argv) == 0 {
		return "", nil
	}
	name := strings.TrimSpace(argv[0])
	if strings.Contains(name, "/") {
		dir, file := path.Split(name)
		if !systemBinDirs[strings.TrimRight(dir, "/")] {
			return name, argv[1:]
		}
		name = file
	}
	return name, argv[1:]
}

var forbiddenPrograms = map[string]bool{
	"sudo": true, "su": true, "doas": true,
	"mount": true, "umount": true, "chroot": true, "nsenter": true, "unshare": true,
	"docker": true, "podman": true, "kubectl": true,
	"mkfs": true, "shutdown": true, "reboot": true, "halt": true, "poweroff": true,
}

var destructivePrograms = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "chmod": true, "chown": true, "chgrp": true,
	"dd": true, "truncate": true, "shred": true, "ln": true,
	"kill": true, "pkill": true, "killall": true,
}

// wrapperPrograms run another program, hiding what actually executes.
var wrapperPrograms = map[string]bool{
	"env": true, "xargs": true, "nohup": true, "nice": true, "timeout": true,
	"time": true, "command": true, "exec": true, "busybox": true, "setsid": true,
	"watch": true, "eval": true, "script": true, "stdbuf": true,
}

var shellPrograms = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ash": true, "ksh": true, "fish": true,
}

// inlineCodeFlags make an interpreter run code given on the command line.
var inlineCodeFlags = map[string]map[string]bool{
	"python": {"-c": true}, "python3": {"-c": true},
	"node": {"-e": true, "--eval": true, "-p": true, "--print": true},
	"perl": {"-e": true, "-E": true},
	"ruby": {"-e": true},
	"php":  {"-r": true},
}

var destructiveGitSubs = map[string]bool{
	"push": true, "clean": true, "rebase": true, "filter-branch": true, "gc": true, "prune": true,
	"update-ref": true, "reflog": true,
}

func destructiveInvocation(prog string, rest []string) bool {
	switch prog {
	case "find":
		return hasAnyFlag(rest, "-delete", "-exec", "-execdir", "-ok", "-okdir")
	case "git":
		sub := gitSub(rest)
		if destructiveGitSubs[sub] {
			return true
		}
		switch sub {
		case "reset":
			return hasAnyFlag(rest, "--hard", "--merge", "--keep")
		case "checkout", "restore", "switch":
			return hasAnyFlag(rest, "--", "-f", "--force", ".", "--discard-changes")
		case "branch":
			return hasAnyFlag(rest, "-d", "-D", "--delete", "-m", "-M", "--move", "-f", "--force")
		case "stash":
			return hasAnyFlag(rest, "drop", "clear", "pop")
		}
	}
	return false
}

func opaqueReason(prog string, rest []string) string {
	if wrapperPrograms[prog] {
		return prog + " runs another program, which cannot be classified"
	}
	if shellPrograms[prog] && hasAnyFlag(rest, "-c") {
		return "shell command strings cannot be classified"
	}
	if flags := inlineCodeFlags[prog]; flags != nil {
		for _, a := range rest {
			if flags[a] {
				return "inline " + prog + " code cannot be classified"
			}
		}
	}
	if prog == "git" && hasAnyFlag(rest, "-c", "--exec-path", "--config-env") {
		return "git config overrides can run arbitrary programs"
	}
	return ""
}

// readOnlyPrograms run silently when their path arguments stay in the project.
var readOnlyPrograms = map[string]bool{
	"ls": true, "pwd": true, "cat": true, "head": true, "tail": true, "wc": true,
	"echo": true, "printf": true, "which": true, "whoami": true, "uname": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "diff": true, "cmp": true,
	"stat": true, "file": true, "tree": true, "sort": true, "cut": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"du": true, "df": true, "true": true, "false": true, "find": true,
}

// unsafeFlags disqualify an otherwise read-only program (write or exec flags).
var unsafeFlags = map[string][]string{
	"rg":   {"--pre", "--pre-glob", "--hostname-bin"},
	"sort": {"-o", "--output", "--compress-program"},
	"tree": {"-o"},
	"diff": {"--to-file", "--from-file"},
	"find": {"-fprint", "-fprint0", "-fprintf", "-fls"},
}

var safeGitSubs = map[string]bool{
	"status": true, "diff": true, "log": true, "show": true, "rev-parse": true,
	"ls-files": true, "blame": true, "describe": true, "shortlog": true,
}

func safeInvocation(prog string, rest []string) bool {
	// A program named by path ("./evil") may be a file the agent just wrote.
	if len(rest) == 1 && !strings.Contains(prog, "/") && (rest[0] == "--version" || rest[0] == "-version") {
		return true
	}
	if prog == "go" && len(rest) == 1 && rest[0] == "version" {
		return true
	}
	if prog == "git" {
		sub := gitSub(rest)
		if safeGitSubs[sub] {
			return !hasAnyFlag(rest, "--output", "--ext-diff", "--textconv")
		}
		if sub == "branch" {
			for _, a := range rest[indexOf(rest, sub)+1:] {
				switch a {
				case "-a", "-r", "-v", "-vv", "--list", "--show-current", "--all", "--remotes":
				default:
					return false
				}
			}
			return true
		}
		return false
	}
	if !readOnlyPrograms[prog] {
		return false
	}
	return !hasAnyFlag(rest, unsafeFlags[prog]...)
}

// subcommandPrograms key session grants on their first non-flag argument.
var subcommandPrograms = map[string]bool{
	"npm": true, "pnpm": true, "yarn": true, "bun": true, "npx": true, "deno": true,
	"go": true, "cargo": true, "git": true, "make": true, "pip": true, "pip3": true,
	"uv": true, "poetry": true, "gradle": true, "mvn": true, "dotnet": true,
	"flutter": true, "dart": true, "python": true, "python3": true, "node": true,
	"ruby": true, "perl": true, "php": true, "bash": true, "sh": true,
}

// runVerbs name a script after the verb, so the grant covers that script only.
var runVerbs = map[string]bool{
	"npm run": true, "npm run-script": true, "pnpm run": true, "yarn run": true,
	"bun run": true, "deno task": true, "deno run": true, "cargo run": true, "go run": true,
}

// sensitiveArg returns the first non-flag argument that names a protected path.
// Relative arguments resolve against the command's working directory, the way
// the program sees them, and are cleaned so "./.git" matches like ".git".
func sensitiveArg(req Request, cwd string, args []string) string {
	root := strings.TrimSpace(req.ProjectRoot)
	base := sandboxcore.CandidatePOSIX(root, cwd)
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		cand := sandboxcore.CandidatePOSIX(base, a)
		if sensitiveDeny(cand, sandboxcore.PathWrite) || gitMetadata(root, cand) {
			return a
		}
	}
	return ""
}

// pathLike reports arguments that name a location outside the working
// directory by absolute path, home, or parent traversal.
func pathLike(a string) bool {
	if strings.HasPrefix(a, "-") {
		return false
	}
	if strings.HasPrefix(a, "/") || strings.HasPrefix(a, "~") {
		return true
	}
	for _, seg := range strings.Split(strings.ReplaceAll(a, "\\", "/"), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func gitSub(args []string) string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-C" || a == "-c":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a
		}
	}
	return ""
}

func firstNonFlag(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f || (strings.HasPrefix(f, "--") && strings.HasPrefix(a, f+"=")) {
				return true
			}
			if f == "-o" && strings.HasPrefix(a, f) && !strings.HasPrefix(a, "--") {
				return true
			}
		}
	}
	return false
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// sweepingFlags reports recursive or forced flags, which widen a destructive
// command's reach (rm -rf, chmod -R, git clean -f).
func sweepingFlags(args []string) bool {
	for _, a := range args {
		if a == "--recursive" || a == "--force" {
			return true
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") &&
			strings.ContainsAny(a[1:], "rRf") {
			return true
		}
	}
	return false
}
