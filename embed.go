// Package myself holds the files embedded in the binary: the landing page
// that `myself serve` serves at / unless --site points elsewhere.
package myself

import "embed"

// Site is the landing page; its files are under "site/".
//
//go:embed site
var Site embed.FS
