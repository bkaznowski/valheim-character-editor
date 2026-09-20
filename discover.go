package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// CharacterFile is one discovered Valheim character save (.fch).
type CharacterFile struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Source  string    `json:"source"`
	ModTime time.Time `json:"modTime"`
}

type candidateBase struct {
	dir    string
	source string
}

func candidateBases() []candidateBase {
	home, _ := os.UserHomeDir()
	var bases []candidateBase

	switch runtime.GOOS {
	case "windows":
		lowLow := filepath.Join(home, "AppData", "LocalLow", "IronGate", "Valheim")
		bases = append(bases,
			candidateBase{filepath.Join(lowLow, "characters_local"), "Local"},
			candidateBase{filepath.Join(lowLow, "characters"), "Local"},
		)
		for _, steamRoot := range []string{
			`C:\Program Files (x86)\Steam`,
			`C:\Program Files\Steam`,
		} {
			bases = append(bases, globSteamCharacters(steamRoot)...)
		}
	case "darwin":
		appSupport := filepath.Join(home, "Library", "Application Support")
		bases = append(bases,
			candidateBase{filepath.Join(appSupport, "IronGate", "Valheim", "characters_local"), "Local"},
			candidateBase{filepath.Join(appSupport, "IronGate", "Valheim", "characters"), "Local"},
		)
		bases = append(bases, globSteamCharacters(filepath.Join(appSupport, "Steam"))...)
	default:
		bases = append(bases,
			candidateBase{filepath.Join(home, ".config", "unity3d", "IronGate", "Valheim", "characters_local"), "Local"},
			candidateBase{filepath.Join(home, ".config", "unity3d", "IronGate", "Valheim", "characters"), "Local"},
		)
		bases = append(bases, globSteamCharacters(filepath.Join(home, ".local", "share", "Steam"))...)
		bases = append(bases, globSteamCharacters(filepath.Join(home, ".steam", "steam"))...)
	}
	return bases
}

// globSteamCharacters looks for <steamRoot>/userdata/<anySteamID>/892970/remote/characters*,
// Valheim's Steam Cloud character save folder (892970 is Valheim's Steam AppID).
func globSteamCharacters(steamRoot string) []candidateBase {
	matches, _ := filepath.Glob(filepath.Join(steamRoot, "userdata", "*", "892970", "remote"))
	var out []candidateBase
	for _, m := range matches {
		out = append(out,
			candidateBase{filepath.Join(m, "characters"), "Steam Cloud"},
			candidateBase{filepath.Join(m, "characters_local"), "Steam Cloud"},
		)
	}
	return out
}

// DiscoverCharacters scans every candidate base directory for *.fch files.
func DiscoverCharacters() []CharacterFile {
	seen := map[string]bool{}
	var out []CharacterFile
	for _, base := range candidateBases() {
		entries, err := os.ReadDir(base.dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".fch") {
				continue
			}
			full := filepath.Join(base.dir, e.Name())
			real, err := filepath.EvalSymlinks(full)
			if err != nil {
				real = full
			}
			if seen[real] {
				continue
			}
			seen[real] = true
			info, err := e.Info()
			var modTime time.Time
			if err == nil {
				modTime = info.ModTime()
			}
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			out = append(out, CharacterFile{Name: name, Path: full, Source: base.source, ModTime: modTime})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out
}

// ResolveCustomPath returns a CharacterFile for a manually supplied path,
// or nil if it doesn't look like a usable .fch file.
func ResolveCustomPath(path string) *CharacterFile {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	if !strings.EqualFold(filepath.Ext(path), ".fch") {
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return &CharacterFile{Name: name, Path: path, Source: "Custom", ModTime: info.ModTime()}
}
