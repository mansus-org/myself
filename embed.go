// Package myself holds the files embedded in the binary: the landing page
// that `myself serve` serves at / unless --site points elsewhere, and the
// agent skill it serves at /skill.md.
package myself

import "embed"

// Site is the landing page; its files are under "site/".
//
//go:embed site
var Site embed.FS

// Skill is skill/myself/SKILL.md, served at /skill.md.
//
//go:embed skill/myself/SKILL.md
var Skill []byte
