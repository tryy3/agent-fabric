// Package skills embeds the built-in skills shipped with the control plane.
//
// Each folder under builtin/ is one skill: a SKILL.md plus optional extra
// files. The control plane reads them through the same loader as plane and
// project skills, so a built-in can later move to a downloaded folder without
// changing any skill.
package skills

import "embed"

// Builtin holds the built-in skill folders under "builtin".
//
//go:embed all:builtin
var Builtin embed.FS

// BuiltinRoot is the directory inside Builtin that holds the skill folders.
const BuiltinRoot = "builtin"
