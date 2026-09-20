package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"valheim-character-editor/fch"
	"valheim-character-editor/webapi"
)

type addedRegistry struct {
	mu    sync.Mutex
	byKey map[string]CharacterFile
}

func newAddedRegistry() *addedRegistry {
	return &addedRegistry{byKey: make(map[string]CharacterFile)}
}

func (r *addedRegistry) add(cf CharacterFile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[cf.Path] = cf
}

func (r *addedRegistry) all() []CharacterFile {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CharacterFile, 0, len(r.byKey))
	for _, cf := range r.byKey {
		out = append(out, cf)
	}
	return out
}

var added = newAddedRegistry()

func registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/characters", func(w http.ResponseWriter, req *http.Request) {
		discovered := DiscoverCharacters()
		seen := map[string]bool{}
		out := make([]CharacterFile, 0, len(discovered))
		for _, cf := range discovered {
			seen[cf.Path] = true
			out = append(out, cf)
		}
		for _, cf := range added.all() {
			if !seen[cf.Path] {
				out = append(out, cf)
			}
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("/api/characters/add", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		cf := ResolveCustomPath(body.Path)
		if cf == nil {
			http.Error(w, "no .fch file found at that path", http.StatusNotFound)
			return
		}
		added.add(*cf)
		writeJSON(w, cf)
	})

	mux.HandleFunc("/api/itemnames", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, fch.KnownItemOptions())
	})

	mux.HandleFunc("/api/trophynames", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, fch.KnownTrophyOptions())
	})

	mux.HandleFunc("/api/beardnames", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, fch.KnownBeardOptions())
	})

	mux.HandleFunc("/api/hairnames", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, fch.KnownHairOptions())
	})

	mux.HandleFunc("/api/icon", func(w http.ResponseWriter, req *http.Request) {
		prefab := req.URL.Query().Get("prefab")
		data := fch.IconPNG(prefab)
		if data == nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		w.Write(data)
	})

	mux.HandleFunc("/api/maxquality", func(w http.ResponseWriter, req *http.Request) {
		prefab := req.URL.Query().Get("prefab")
		writeJSON(w, fch.MaxQualityForPrefab(prefab))
	})

	mux.HandleFunc("/api/load", func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		sf, err := fch.Load(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, webapi.ToDTO(path, sf.Character))
	})

	mux.HandleFunc("/api/save", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var sreq webapi.SaveRequest
		if err := json.NewDecoder(req.Body).Decode(&sreq); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if sreq.Path == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		sf, err := fch.Load(sreq.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		edits, err := webapi.BuildEdits(sf, sreq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		outPath := sreq.Path
		if sreq.SaveAsPath != "" {
			outPath = sreq.SaveAsPath
		} else if err := backupBeforeOverwrite(outPath); err != nil {
			http.Error(w, "failed to back up existing save before overwriting: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := sf.Save(outPath, edits); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sf2, err := fch.Load(outPath)
		if err != nil {
			http.Error(w, "saved but failed to reload for confirmation: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, webapi.ToDTO(outPath, sf2.Character))
	})
}
