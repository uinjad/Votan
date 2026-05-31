// Package web embeds the overlay and admin UI into the binary so the compiled
// program is a single self-contained executable — no external files to ship.
package web

import (
	"embed"
	"io/fs"
	"regexp"
	"strconv"
)

// Files holds the embedded contents of ./public (index.html, admin.html,
// assets/*). The `all:` prefix also embeds files whose names start with "."
// or "_".
//
//go:embed all:public
var Files embed.FS

// Assets returns the embedded filesystem rooted at the web root, so "/" maps to
// public/index.html.
func Assets() fs.FS {
	sub, err := fs.Sub(Files, "public")
	if err != nil {
		// public is embedded at compile time, so this can only fail if the
		// directory was removed from the source tree.
		panic("web: embedded public directory missing: " + err.Error())
	}
	return sub
}

var (
	headRe = regexp.MustCompile(`^head_(\d+)\.png$`)
	bodyRe = regexp.MustCompile(`^body_(\d+)\.png$`)
)

// ScanSkins reports the highest head_N / body_N skin ids in the embedded
// assets, so the available range isn't hardcoded and grows by adding files.
func ScanSkins() (maxHead, maxBody int) {
	entries, err := fs.ReadDir(Files, "public/assets")
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if m := headRe.FindStringSubmatch(name); m != nil {
			if id, _ := strconv.Atoi(m[1]); id > maxHead {
				maxHead = id
			}
		} else if m := bodyRe.FindStringSubmatch(name); m != nil {
			if id, _ := strconv.Atoi(m[1]); id > maxBody {
				maxBody = id
			}
		}
	}
	return maxHead, maxBody
}
