package gate

import (
	"path"
	"regexp"
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
	// RuleCommandCatastrophic denies wiping the system, home or parent directory.
	RuleCommandCatastrophic = "rules.command_catastrophic"
	// RuleCommandForbiddenName asks about a forbidden program name reached by an unusual path.
	RuleCommandForbiddenName = "rules.command_forbidden_name"
	RuleCommandNetwork       = "rules.command_network_ask"
	RuleCommandExfil         = "rules.command_exfiltration"
	RuleCommandFetchExec     = "rules.command_fetch_exec"
)

// evaluateCommand classifies one run_command call. Tiers, first match wins:
//
//  1. deny: host execution, cwd escape, privilege/host-control programs,
//     destructive commands aimed at the system, home, or protected paths,
//     reverse shells
//  2. ask without a session grant: destructive, network or unclassifiable
//     commands; scored by how far they reach
//  3. allow: a small read-only allowlist whose path arguments stay in the root
//  4. ask (session grant by command prefix): everything else, including
//     build, test and install tooling
func evaluateCommand(req Request) (Decision, error) {
	args, err := command.Decode(req.Args)
	if err != nil {
		return Decision{Kind: Deny, Reason: err.Error(), RuleID: "rules.bad_args"}, nil
	}
	base := Decision{Command: args.Command, Cwd: args.Cwd}
	if req.EnvKind != "docker" {
		d := base
		d.Kind, d.RuleID, d.Reason = Deny, RuleCommandLocalDeny, "command execution is only available in Docker environments"
		return d, nil
	}
	cwd := args.Cwd
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	if d := evaluatePath(req, cwd, sandboxcore.PathRead); d.Kind != Allow {
		deny := base
		deny.Kind, deny.RuleID, deny.Reason = Deny, RuleCommandCwdEscape, "working directory must stay inside the project: "+cwd
		return deny, nil
	}
	d := classifyArgv(req, cwd, args.Command, 0)
	d.Command, d.Cwd = args.Command, args.Cwd
	return d, nil
}

// maxNestedCommands bounds how deep classifyArgv follows wrappers and shell
// strings (env sh -c "timeout 5 sh -c ...").
const maxNestedCommands = 3

// classifyArgv classifies one argv. depth counts wrapper and shell-string
// nesting; nested commands never yield a session grant.
func classifyArgv(req Request, cwd string, argv []string, depth int) Decision {
	deny := func(rule, reason string) Decision {
		return Decision{Kind: Deny, RuleID: rule, Reason: reason}
	}
	hardAsk := func(rule, reason string, risk Risk) Decision {
		return Decision{Kind: Ask, RuleID: rule, Reason: reason, NoSessionGrant: true, Risk: risk}
	}

	prog, rest := normalizeProgram(argv)
	if forbiddenPrograms[prog] {
		return deny(RuleCommandForbidden, prog+" is not permitted")
	}
	if strings.Contains(prog, "/") && forbiddenPrograms[path.Base(prog)] {
		return hardAsk(RuleCommandForbiddenName, path.Base(prog)+" run from "+path.Dir(prog)+" may be a renamed privileged program", riskForbiddenName)
	}

	if destructivePrograms[prog] || destructiveInvocation(prog, rest) {
		if wipePrograms[prog] && sweepingFlags(rest) {
			for _, a := range rest {
				if catastrophicTarget(a) {
					return deny(RuleCommandCatastrophic, prog+" would destroy "+a)
				}
			}
		}
		if p := sensitiveArg(req, cwd, rest); p != "" {
			return deny(RuleCommandSensitive, "command targets a protected path: "+p)
		}
		if killPrograms[prog] && indexOf(rest, "1") >= 0 {
			return hardAsk(RuleCommandDestructive, prog+" of process 1 stops the container", riskKillInit)
		}
		d := hardAsk(RuleCommandDestructive, "command can destroy or change data and needs confirmation: "+prog, riskDestructive)
		sweeping := sweepingFlags(rest)
		if sweeping {
			d.Risk = riskDestructiveWide
		}
		for _, a := range rest {
			switch {
			case sweeping && projectWideTarget(a):
				d.Risk = max(d.Risk, riskDestructiveProject)
				d.Reason = prog + " would change the whole project: " + a
			case outsideProject(req, a):
				d.Risk = max(d.Risk, riskPathEscape)
			}
		}
		return d
	}

	if networkPrograms[prog] {
		if (prog == "nc" || prog == "ncat" || prog == "netcat") && hasAnyFlag(rest, "-e", "-c", "--exec", "--sh-exec") {
			return deny(RuleCommandForbidden, prog+" would open a remote shell")
		}
		for _, a := range rest {
			if p := exfilArg(req, a); p != "" {
				return hardAsk(RuleCommandExfil, prog+" would send "+p+" to a remote host", riskExfil)
			}
		}
		return hardAsk(RuleCommandNetwork, prog+" talks to the network", riskNetwork)
	}

	if opaque := opaqueReason(prog, rest); opaque != "" {
		d := hardAsk(RuleCommandOpaque, opaque, riskOpaque)
		text := strings.Join(argv, " ")
		if catastrophicText.MatchString(text) {
			return deny(RuleCommandCatastrophic, "command text would destroy the system or home directory")
		}
		if fetchExecText.MatchString(text) {
			d.RuleID, d.Risk, d.Reason = RuleCommandFetchExec, riskFetchExec, "command downloads code and runs it"
		}
		if depth >= maxNestedCommands {
			return d
		}
		for _, inner := range nestedCommands(prog, rest) {
			in := classifyArgv(req, cwd, inner, depth+1)
			if in.Kind == Deny {
				in.Reason = "inside " + prog + ": " + in.Reason
				return in
			}
			if r := decisionRisk(in); r > d.Risk {
				d.Risk, d.Reason = r, "inside "+prog+": "+in.Reason
			}
		}
		return d
	}

	if safeInvocation(prog, rest) {
		d := Decision{Kind: Allow, RuleID: RuleCommandSafe}
		for _, a := range rest {
			if outsideProject(req, a) {
				ask := hardAsk("rules.path_ask", "path is outside the project: "+a, 0)
				ask.Path = a
				return ask
			}
			if !strings.HasPrefix(a, "-") && secretPath(a) {
				d.Risk, d.Reason = riskSecretRead, "reads a secret file: "+a
			}
		}
		return d
	}

	// An unclassified program given a path outside the project (sed -i ~/.bashrc,
	// tee /etc/cron.d/x) reaches further than its session grant should cover.
	for _, a := range rest {
		if outsideProject(req, a) {
			ask := hardAsk("rules.path_ask", prog+" is given a path outside the project: "+a, 0)
			ask.Path = a
			return ask
		}
	}

	key := GrantKey(argv)
	d := Decision{GrantKey: key}
	if depth == 0 {
		for _, granted := range req.CommandGrants {
			if granted == key {
				d.Kind, d.RuleID = Allow, RuleCommandGranted
				return d
			}
		}
	}
	d.Kind, d.RuleID = Ask, RuleCommandAsk
	d.Reason = "run " + key
	return d
}

// decisionRisk is a decision's score, falling back to its rule's default.
func decisionRisk(d Decision) Risk {
	if d.Risk > 0 {
		return d.Risk
	}
	return ruleRisk(d)
}

// outsideProject reports a path argument that names the home directory or
// fails the project path policy. Argv execution does not expand ~ or $HOME,
// but an agent that writes them means the home directory.
func outsideProject(req Request, a string) bool {
	if a == "" || strings.HasPrefix(a, "-") {
		return false
	}
	if homeRef(a) {
		return true
	}
	return pathLike(a) && evaluatePath(req, a, sandboxcore.PathRead).Kind != Allow
}

func homeRef(a string) bool {
	return strings.HasPrefix(a, "~") || strings.Contains(a, "$HOME") || strings.Contains(a, "${HOME}")
}

// catastrophicTarget reports the filesystem root, the home directory or the
// project's parent, with or without a trailing "/" or "/*".
func catastrophicTarget(a string) bool {
	switch strings.TrimSuffix(strings.TrimSuffix(a, "/*"), "/") {
	case "~", "$HOME", "${HOME}", "..":
		return true
	case "":
		return a == "/" || a == "/*"
	}
	return false
}

// projectWideTarget reports the project root itself.
func projectWideTarget(a string) bool {
	switch a {
	case ".", "./", "./*", "*":
		return true
	}
	return false
}

// wipePrograms are the destructive programs whose recursive or forced form is
// denied outright when aimed at the system, home or parent directory.
var wipePrograms = map[string]bool{
	"rm": true, "chmod": true, "chown": true, "chgrp": true, "shred": true,
}

var killPrograms = map[string]bool{"kill": true, "pkill": true, "killall": true}

// networkPrograms move data to or from other hosts.
var networkPrograms = map[string]bool{
	"curl": true, "wget": true, "nc": true, "ncat": true, "netcat": true,
	"scp": true, "sftp": true, "ssh": true, "rsync": true, "telnet": true, "ftp": true,
}

// exfilArg returns the secret, system or home path a network command would
// send, if any. Upload syntax wraps the path ("@.env", "file=@.env").
func exfilArg(req Request, a string) string {
	if strings.HasPrefix(a, "-") || strings.Contains(a, "://") {
		return ""
	}
	p := a
	if i := strings.LastIndex(p, "@"); i >= 0 && !strings.Contains(p[i:], ":") {
		p = p[i+1:]
	}
	if p == "" || strings.Contains(p, "@") {
		return ""
	}
	if secretPath(p) || homeRef(p) {
		return p
	}
	if path.IsAbs(p) && sensitiveDeny(path.Clean(p), sandboxcore.PathWrite) {
		return p
	}
	return ""
}

var (
	// catastrophicText finds a recursive or forced rm of the root or home
	// directory inside a shell string or inline code.
	catastrophicText = regexp.MustCompile(`\brm\s+(-\w+\s+)*-\w*[rRf]\w*\s+(-\w+\s+)*(/\*?|~/?\*?|\$\{?HOME\}?/?\*?)(\s|['");&|]|$)`)
	// fetchExecText finds a download piped into a shell or substituted into a command.
	fetchExecText   = regexp.MustCompile(`\b(curl|wget)\b[^;&]*\|\s*(sudo\s+)?(ba|z|da|k)?sh\b|\$\(\s*(curl|wget)\b|` + "`" + `\s*(curl|wget)\b`)
	shellSeparators = regexp.MustCompile(`\|\||&&|[;|&\n]`)
)

// nestedCommands returns the commands an opaque invocation would run, as far
// as they can be read: the segments of a shell string, or the command a
// wrapper is given. Splitting ignores quoting, so this only ever adds risk to
// the opaque score; it never clears a command.
func nestedCommands(prog string, rest []string) [][]string {
	if shellPrograms[prog] {
		i := indexOf(rest, "-c")
		if i < 0 || i+1 >= len(rest) {
			return nil
		}
		var out [][]string
		for _, seg := range shellSeparators.Split(rest[i+1], -1) {
			if f := strings.Fields(strings.NewReplacer(`'`, "", `"`, "").Replace(seg)); len(f) > 0 {
				out = append(out, f)
			}
		}
		return out
	}
	if !wrapperPrograms[prog] {
		return nil
	}
	skipOne := prog == "timeout" // its first operand is the duration
	for i, a := range rest {
		if strings.HasPrefix(a, "-") || (prog == "env" && strings.Contains(a, "=")) {
			continue
		}
		if skipOne {
			skipOne = false
			continue
		}
		return [][]string{rest[i:]}
	}
	return nil
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
		// "of=/dev/sda" names its target after the equals sign; "if=" is only read.
		key, value, _ := strings.Cut(a, "=")
		if key == "if" {
			continue
		}
		for _, p := range []string{a, value} {
			if p == "" {
				continue
			}
			cand := sandboxcore.CandidatePOSIX(base, p)
			if sensitiveDeny(cand, sandboxcore.PathWrite) || gitMetadata(root, cand) {
				return a
			}
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
