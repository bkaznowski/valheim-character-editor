// Command wasm builds the browser (GOOS=js GOARCH=wasm) version of the
// character editor. It has no filesystem or HTTP server: the page loads
// raw .fch bytes via a file picker, this binary parses/edits them entirely
// in memory, and the page triggers a browser download of the result. It
// shares all parsing/editing logic with the desktop build via the fch and
// webapi packages -- this file only adapts that logic to syscall/js.
package main

import (
	"encoding/json"
	"syscall/js"

	"valheim-character-editor/fch"
	"valheim-character-editor/webapi"
)

// current is the in-memory save file being edited. There is exactly one at
// a time, matching a browser tab editing one opened file.
var current *fch.SaveFile

// lastSaveBytes holds the most recent successful save's output bytes,
// fetched by the page via lastSaveBytes() to trigger a download.
var lastSaveBytes []byte

type okResult struct {
	OK    bool                 `json:"ok"`
	DTO   *webapi.CharacterDTO `json:"dto,omitempty"`
	Error string               `json:"error,omitempty"`
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(okResult{OK: false, Error: err.Error()})
	}
	return string(b)
}

func bytesFromJS(v js.Value) []byte {
	buf := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(buf, v)
	return buf
}

func bytesToJS(data []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	return arr
}

// loadCharacter(uint8Array) -> JSON string {ok, dto|error}
func loadCharacter(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return toJSON(okResult{OK: false, Error: "missing file bytes"})
	}
	data := bytesFromJS(args[0])
	sf, err := fch.Parse(data)
	if err != nil {
		return toJSON(okResult{OK: false, Error: err.Error()})
	}
	current = sf
	lastSaveBytes = nil
	dto := webapi.ToDTO("", sf.Character)
	return toJSON(okResult{OK: true, DTO: &dto})
}

// saveCharacter(jsonSaveRequest) -> JSON string {ok, dto|error}. On success,
// the edited bytes are stashed for retrieval via lastSaveBytes().
func saveCharacter(this js.Value, args []js.Value) interface{} {
	if current == nil {
		return toJSON(okResult{OK: false, Error: "no character loaded"})
	}
	if len(args) < 1 {
		return toJSON(okResult{OK: false, Error: "missing request"})
	}
	var sreq webapi.SaveRequest
	if err := json.Unmarshal([]byte(args[0].String()), &sreq); err != nil {
		return toJSON(okResult{OK: false, Error: "bad request: " + err.Error()})
	}

	edits, err := webapi.BuildEdits(current, sreq)
	if err != nil {
		return toJSON(okResult{OK: false, Error: err.Error()})
	}

	out, err := current.Apply(edits)
	if err != nil {
		return toJSON(okResult{OK: false, Error: err.Error()})
	}

	sf2, err := fch.Parse(out)
	if err != nil {
		return toJSON(okResult{OK: false, Error: "saved but failed to reparse result: " + err.Error()})
	}
	current = sf2
	lastSaveBytes = out

	dto := webapi.ToDTO("", sf2.Character)
	return toJSON(okResult{OK: true, DTO: &dto})
}

// lastSaveBytes() -> Uint8Array | null
func lastSaveBytesJS(this js.Value, args []js.Value) interface{} {
	if lastSaveBytes == nil {
		return js.Null()
	}
	return bytesToJS(lastSaveBytes)
}

func itemNames(this js.Value, args []js.Value) interface{} {
	return toJSONRaw(fch.KnownItemOptions())
}

func trophyNames(this js.Value, args []js.Value) interface{} {
	return toJSONRaw(fch.KnownTrophyOptions())
}

func beardNames(this js.Value, args []js.Value) interface{} {
	return toJSONRaw(fch.KnownBeardOptions())
}

func hairNames(this js.Value, args []js.Value) interface{} {
	return toJSONRaw(fch.KnownHairOptions())
}

func itemCatalog(this js.Value, args []js.Value) interface{} {
	return toJSONRaw(fch.KnownItemCatalog())
}

func toJSONRaw(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func maxQuality(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return fch.DefaultMaxQuality
	}
	return fch.MaxQualityForPrefab(args[0].String())
}

// iconPNG(prefabName) -> Uint8Array | null
func iconPNG(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return js.Null()
	}
	data := fch.IconPNG(args[0].String())
	if data == nil {
		return js.Null()
	}
	return bytesToJS(data)
}

func main() {
	api := js.Global().Get("Object").New()
	api.Set("loadCharacter", js.FuncOf(loadCharacter))
	api.Set("saveCharacter", js.FuncOf(saveCharacter))
	api.Set("lastSaveBytes", js.FuncOf(lastSaveBytesJS))
	api.Set("itemNames", js.FuncOf(itemNames))
	api.Set("trophyNames", js.FuncOf(trophyNames))
	api.Set("beardNames", js.FuncOf(beardNames))
	api.Set("hairNames", js.FuncOf(hairNames))
	api.Set("itemCatalog", js.FuncOf(itemCatalog))
	api.Set("maxQuality", js.FuncOf(maxQuality))
	api.Set("iconPNG", js.FuncOf(iconPNG))
	js.Global().Set("valheimApi", api)

	js.Global().Get("document").Call("dispatchEvent", js.Global().Get("CustomEvent").New("valheimApiReady"))

	select {} // keep the Go runtime alive; all work happens via JS callbacks
}
