package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed static/index.html
var staticFiles embed.FS

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	registerAPI(mux)

	if addr := os.Getenv("DEV_HTTP_ADDR"); addr != "" {
		log.Println("dev http server listening on", addr)
		log.Fatal(http.ListenAndServe(addr, mux))
		return
	}

	err := wails.Run(&options.App{
		Title:  "Valheim Character Editor",
		Width:  760,
		Height: 860,
		AssetServer: &assetserver.Options{
			Handler: mux,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 26, B: 23, A: 1},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), 500)
	}
}
