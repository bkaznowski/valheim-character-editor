// Package webapi holds the character-editing request/response contract and
// business logic shared by both the desktop app (api.go, over net/http) and
// the browser/WASM build (cmd/wasm/main.go, over syscall/js). Everything
// here is pure in-memory logic against an already-loaded *fch.SaveFile --
// no file I/O -- so it works unchanged on either side.
package webapi

import (
	"fmt"

	"valheim-character-editor/fch"
)

type ItemDTO struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	PrefabName  string `json:"prefabName"`
	Hash        int32  `json:"hash"`
	Quality     uint16 `json:"quality"`
	MaxQuality  uint16 `json:"maxQuality"`
	Stack       uint16 `json:"stack"`
	Durability  int32  `json:"durability"`
	Variant     int32  `json:"variant"`
	CrafterID   int64  `json:"crafterID"`
	CrafterName string `json:"crafterName"`
	Equipped    bool   `json:"equipped"`
	HasQuality  bool   `json:"hasQuality"`
	HasStack    bool   `json:"hasStack"`
	HasVariant  bool   `json:"hasVariant"`
	HasCrafter  bool   `json:"hasCrafter"`
	HasIcon     bool   `json:"hasIcon"`
	GridX       uint8  `json:"gridX"`
	GridY       uint8  `json:"gridY"`
}

type SkillDTO struct {
	Type        int32   `json:"type"`
	Name        string  `json:"name"`
	Level       float32 `json:"level"`
	Accumulator float32 `json:"accumulator"`
}

type PowerDTO struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Unlocked bool   `json:"unlocked"`
}

type TrophyDTO struct {
	Prefab string `json:"prefab"`
	Name   string `json:"name"`
}

type FoodDTO struct {
	Index       int      `json:"index"`
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Time        *float32 `json:"time"`
	Health      *float32 `json:"health"`
	Stamina     *float32 `json:"stamina"`
}

type CharacterDTO struct {
	Path          string      `json:"path"`
	Name          string      `json:"name"`
	PlayerID      int64       `json:"playerID"`
	Version       int32       `json:"version"`
	MaxHealth     float32     `json:"maxHealth"`
	Health        float32     `json:"health"`
	Stamina       float32     `json:"stamina"`
	Stamina2      float32     `json:"stamina2"`
	MaxEitr       float32     `json:"maxEitr"`
	Eitr          float32     `json:"eitr"`
	PlayerModel   int32       `json:"playerModel"`
	Beard         string      `json:"beard"`
	BeardName     string      `json:"beardName"`
	Hair          string      `json:"hair"`
	HairName      string      `json:"hairName"`
	SkinColor     [3]float32  `json:"skinColor"`
	HairColor     [3]float32  `json:"hairColor"`
	Skills        []SkillDTO  `json:"skills"`
	Inventory     []ItemDTO   `json:"inventory"`
	RecipeCount   int         `json:"recipeCount"`
	MaterialCount int         `json:"materialCount"`
	StationCount  int         `json:"stationCount"`
	InventoryRows int         `json:"inventoryRows"`
	InventoryCols int         `json:"inventoryCols"`
	DefaultRows   int         `json:"defaultRows"`
	Powers        []PowerDTO  `json:"powers"`
	Trophies      []TrophyDTO `json:"trophies"`
	Foods         []FoodDTO   `json:"foods"`
}

// ToDTO builds the JSON-friendly snapshot of a character sent to the
// frontend after a load or a save. path is opaque to the frontend -- the
// desktop build passes the real file path, the WASM build passes a
// display-only name (there is no filesystem path in a browser).
func ToDTO(path string, c *fch.Character) CharacterDTO {
	dto := CharacterDTO{
		Path:          path,
		Name:          c.Name,
		PlayerID:      c.PlayerID,
		Version:       c.Version,
		MaxHealth:     c.MaxHealth,
		Health:        c.Health,
		Stamina:       c.Stamina,
		Stamina2:      c.Stamina2,
		MaxEitr:       c.MaxEitr,
		Eitr:          c.Eitr,
		PlayerModel:   c.PlayerModel,
		Beard:         c.Beard,
		BeardName:     fch.ResolveDisplayNameForPrefab(c.Beard),
		Hair:          c.Hair,
		HairName:      fch.ResolveDisplayNameForPrefab(c.Hair),
		SkinColor:     c.SkinColor,
		HairColor:     c.HairColor,
		RecipeCount:   len(c.Recipes),
		MaterialCount: len(c.KnownMaterial),
		StationCount:  len(c.Stations),
		InventoryRows: c.InventoryRows(),
		InventoryCols: fch.DefaultInventoryWidth,
		DefaultRows:   fch.DefaultInventoryHeight,
	}
	levelByType := map[int32]fch.Skill{}
	for _, s := range c.Skills {
		levelByType[s.Type] = s
	}
	for _, t := range fch.KnownSkillTypes {
		s := levelByType[t] // zero value (level 0) if never trained
		dto.Skills = append(dto.Skills, SkillDTO{Type: t, Name: fch.SkillTypeNames[t], Level: s.Level, Accumulator: s.Accumulator})
	}
	for i, it := range c.Inventory {
		prefab := fch.ResolvePrefabName(it.PrefabHash)
		display := fch.ResolveDisplayName(it.PrefabHash)
		if display == "" {
			display = prefab
		}
		dto.Inventory = append(dto.Inventory, ItemDTO{
			Index:       i,
			Name:        display,
			PrefabName:  prefab,
			Hash:        it.PrefabHash,
			Quality:     it.Quality,
			MaxQuality:  uint16(fch.MaxQualityForPrefab(prefab)),
			Stack:       it.Stack,
			Durability:  it.Durability,
			Variant:     it.Variant,
			CrafterID:   it.CrafterID,
			CrafterName: it.CrafterName,
			Equipped:    it.Equipped(),
			HasQuality:  it.HasQuality(),
			HasStack:    it.HasStack(),
			HasVariant:  it.HasVariant(),
			HasCrafter:  it.HasCrafter(),
			HasIcon:     fch.HasIcon(prefab),
			GridX:       it.GridX,
			GridY:       it.GridY,
		})
	}
	uniqueSet := map[string]bool{}
	for _, u := range c.Uniques {
		uniqueSet[u] = true
	}
	for _, p := range fch.ForsakenPowers {
		dto.Powers = append(dto.Powers, PowerDTO{Key: p.Key, Name: p.Name, Unlocked: uniqueSet[p.Key]})
	}
	for _, t := range c.Trophies {
		dto.Trophies = append(dto.Trophies, TrophyDTO{Prefab: t, Name: fch.ResolveDisplayNameForPrefab(t)})
	}
	for i, f := range c.Foods {
		dto.Foods = append(dto.Foods, FoodDTO{
			Index:       i,
			Name:        f.Name,
			DisplayName: fch.ResolveDisplayNameForPrefab(f.Name),
			Time:        f.Time,
			Health:      f.Health,
			Stamina:     f.Stamina,
		})
	}
	return dto
}

type ItemEdit struct {
	Index       int     `json:"index"`
	Quality     *uint16 `json:"quality"`
	Stack       *uint16 `json:"stack"`
	Durability  *int32  `json:"durability"`
	Variant     *int32  `json:"variant"`
	CrafterID   *int64  `json:"crafterID"`
	CrafterName *string `json:"crafterName"`
	GridX       *uint8  `json:"gridX"`
	GridY       *uint8  `json:"gridY"`
}

type SkillEdit struct {
	Type  int32   `json:"type"`
	Level float32 `json:"level"`
}

type NewItemRequest struct {
	Name    string `json:"name"`
	Stack   uint16 `json:"stack"`
	Quality uint16 `json:"quality"`
	GridX   *uint8 `json:"gridX"`
	GridY   *uint8 `json:"gridY"`
}

type FoodEdit struct {
	Index int      `json:"index"`
	Name  *string  `json:"name"`
	Time  *float32 `json:"time"`
}

// SaveRequest is the full set of edits requested for one save. Path and
// SaveAsPath are meaningless in the WASM build (there is no filesystem) and
// are ignored there.
type SaveRequest struct {
	Path               string           `json:"path"`
	SaveAsPath         string           `json:"saveAsPath"`
	Name               *string          `json:"name"`
	Health             *float32         `json:"health"`
	MaxHealth          *float32         `json:"maxHealth"`
	Stamina            *float32         `json:"stamina"`
	Eitr               *float32         `json:"eitr"`
	MaxEitr            *float32         `json:"maxEitr"`
	PlayerModel        *int32           `json:"playerModel"`
	Beard              *string          `json:"beard"`
	Hair               *string          `json:"hair"`
	SkinColor          *[3]float32      `json:"skinColor"`
	HairColor          *[3]float32      `json:"hairColor"`
	Skills             []SkillEdit      `json:"skills"`
	Items              []ItemEdit       `json:"items"`
	AddGold            *uint16          `json:"addGold"`
	AddItems           []NewItemRequest `json:"addItems"`
	RemoveIndexes      []int            `json:"removeIndexes"`
	InventoryRows      *int             `json:"inventoryRows"`
	PowersOn           []string         `json:"powersOn"`
	PowersOff          []string         `json:"powersOff"`
	AddTrophies        []string         `json:"addTrophies"`
	RemoveTrophies     []string         `json:"removeTrophies"`
	UnlockAllRecipes   bool             `json:"unlockAllRecipes"`
	UnlockAllMaterials bool             `json:"unlockAllMaterials"`
	Foods              []FoodEdit       `json:"foods"`
}

// BuildEdits turns a SaveRequest into the concrete list of byte-splice
// edits to apply to sf, validating everything (item indexes, grid
// collisions, food indexes) along the way. It touches only sf.Character in
// memory -- no file I/O -- so both the desktop (net/http) and WASM
// (syscall/js) entrypoints can share it, differing only in what they do
// with the resulting edits (sf.Save to disk vs. sf.Apply to bytes for a
// browser download).
func BuildEdits(sf *fch.SaveFile, sreq SaveRequest) ([]fch.Edit, error) {
	c := sf.Character
	var edits []fch.Edit

	if sreq.Name != nil && *sreq.Name != c.Name {
		nameEdit, err := fch.SetCharacterName(sf, *sreq.Name)
		if err != nil {
			return nil, err
		}
		edits = append(edits, nameEdit)
	}
	if sreq.Health != nil {
		edits = append(edits, fch.EditFloat(c.Offsets["health"], *sreq.Health))
	}
	if sreq.MaxHealth != nil {
		edits = append(edits, fch.EditFloat(c.Offsets["maxHealth"], *sreq.MaxHealth))
	}
	if sreq.Stamina != nil {
		edits = append(edits, fch.EditFloat(c.Offsets["stamina"], *sreq.Stamina))
	}
	if sreq.Eitr != nil {
		edits = append(edits, fch.EditFloat(c.Offsets["eitr"], *sreq.Eitr))
	}
	if sreq.MaxEitr != nil {
		edits = append(edits, fch.EditFloat(c.Offsets["maxEitr"], *sreq.MaxEitr))
	}
	if sreq.PlayerModel != nil {
		edits = append(edits, fch.EditI32(c.Offsets["playerModel"], *sreq.PlayerModel))
	}
	if sreq.SkinColor != nil {
		edits = append(edits, fch.EditVec3(c.Offsets["skinColor"], *sreq.SkinColor))
	}
	if sreq.HairColor != nil {
		edits = append(edits, fch.EditVec3(c.Offsets["hairColor"], *sreq.HairColor))
	}
	if sreq.Beard != nil || sreq.Hair != nil {
		edits = append(edits, fch.SetBeardHair(c, sreq.Beard, sreq.Hair)...)
	}
	if len(sreq.Skills) > 0 {
		levels := make(map[int32]float32, len(sreq.Skills))
		for _, se := range sreq.Skills {
			levels[se.Type] = se.Level
		}
		edits = append(edits, fch.SetSkillLevels(c, levels)...)
	}
	if sreq.InventoryRows != nil {
		rowEdits, err := fch.SetInventoryRows(c, *sreq.InventoryRows)
		if err != nil {
			return nil, err
		}
		edits = append(edits, rowEdits...)
	}
	edits = append(edits, fch.SetUniqueFlags(c, sreq.PowersOn, sreq.PowersOff)...)
	edits = append(edits, fch.SetTrophies(c, sreq.AddTrophies, sreq.RemoveTrophies)...)
	if sreq.UnlockAllRecipes {
		edits = append(edits, fch.AddRecipes(c, fch.AllRecipeKeys())...)
	}
	if sreq.UnlockAllMaterials {
		edits = append(edits, fch.AddKnownMaterials(c, fch.AllRecipeKeys())...)
	}
	for _, fe := range sreq.Foods {
		if fe.Index < 0 || fe.Index >= len(c.Foods) {
			return nil, fmt.Errorf("food index %d out of range", fe.Index)
		}
		edits = append(edits, fch.SetFood(&c.Foods[fe.Index], fe.Name, fe.Time)...)
	}

	removeSet := map[int]bool{}
	for _, idx := range sreq.RemoveIndexes {
		if idx < 0 || idx >= len(c.Inventory) {
			return nil, fmt.Errorf("item index %d out of range", idx)
		}
		removeSet[idx] = true
	}

	// Figure out up front which existing item (if any) absorbs an AddGold
	// request, as a stack override rather than a separately-emitted edit.
	// The per-item loop below always receives a Stack value for every item
	// (the frontend sends full current state every save), so folding the
	// gold bump into that same pass -- instead of emitting a second,
	// independent stack edit for the same item -- avoids two edits landing
	// on the identical byte range (which Apply correctly rejects as
	// overlapping).
	newItems := append([]NewItemRequest{}, sreq.AddItems...)
	goldStackOverride := map[int]uint16{}
	if sreq.AddGold != nil && *sreq.AddGold > 0 {
		addedToExisting := false
		for i := range c.Inventory {
			it := &c.Inventory[i]
			if !removeSet[i] && it.PrefabHash == fch.StableHash("Coins") && it.HasStack() {
				goldStackOverride[i] = it.Stack + *sreq.AddGold
				addedToExisting = true
				break
			}
		}
		if !addedToExisting {
			newItems = append(newItems, NewItemRequest{Name: "Coins", Stack: *sreq.AddGold, Quality: 1})
		}
	}

	// Track each surviving item's final grid position (starts at its
	// current position, overridden by any requested move) so we can
	// validate the whole layout is collision-free in one pass instead
	// of trying to reason about incremental moves/swaps.
	finalPos := map[int]fch.GridSlot{}
	for i, it := range c.Inventory {
		if removeSet[i] {
			continue
		}
		finalPos[i] = fch.GridSlot{it.GridX, it.GridY}
	}

	for _, ie := range sreq.Items {
		if ie.Index < 0 || ie.Index >= len(c.Inventory) {
			return nil, fmt.Errorf("item index %d out of range", ie.Index)
		}
		if removeSet[ie.Index] {
			continue // being deleted below; a field edit on it would overlap
		}
		it := &c.Inventory[ie.Index]
		stack := ie.Stack
		if override, ok := goldStackOverride[ie.Index]; ok {
			v := override
			stack = &v
			delete(goldStackOverride, ie.Index) // handled here; skip the fallback pass below
		}
		if ie.Quality != nil || stack != nil || ie.Variant != nil || ie.CrafterID != nil || ie.CrafterName != nil {
			fieldEdits, err := fch.SetItemFields(it, fch.ItemFieldUpdate{
				Quality:     ie.Quality,
				Stack:       stack,
				Variant:     ie.Variant,
				CrafterID:   ie.CrafterID,
				CrafterName: ie.CrafterName,
			})
			if err != nil {
				return nil, err
			}
			edits = append(edits, fieldEdits...)
		}
		if ie.Durability != nil {
			edits = append(edits, fch.SetItemDurabilityValue(it, *ie.Durability))
		}
		if ie.GridX != nil && ie.GridY != nil {
			finalPos[ie.Index] = fch.GridSlot{*ie.GridX, *ie.GridY}
		}
	}

	for idx := range removeSet {
		e, err := fch.RemoveItemEdit(c, idx)
		if err != nil {
			return nil, err
		}
		edits = append(edits, e)
	}

	// Any gold-target item the client didn't also send a per-item edit for
	// (a minimal caller that only sent AddGold) still needs its stack bump
	// applied directly.
	for idx, newStack := range goldStackOverride {
		stackEdits, err := fch.SetItemStack(&c.Inventory[idx], newStack)
		if err != nil {
			return nil, err
		}
		edits = append(edits, stackEdits...)
	}

	// Validate the surviving/moved items don't collide with each other.
	occupied := map[fch.GridSlot]bool{}
	for i, pos := range finalPos {
		if occupied[pos] {
			return nil, fmt.Errorf("item index %d collides with another item at grid position (%d,%d)", i, pos[0], pos[1])
		}
		occupied[pos] = true
	}

	if len(newItems) > 0 {
		for _, ni := range newItems {
			if ni.Name == "" || ni.Stack == 0 {
				return nil, fmt.Errorf("new items need a name and a stack > 0")
			}
			quality := ni.Quality
			if quality == 0 {
				quality = 1
			}
			var x, y uint8
			if ni.GridX != nil && ni.GridY != nil {
				x, y = *ni.GridX, *ni.GridY
				if occupied[fch.GridSlot{x, y}] {
					return nil, fmt.Errorf("no room to add %q: grid position (%d,%d) is occupied", ni.Name, x, y)
				}
				occupied[fch.GridSlot{x, y}] = true
			} else {
				var err error
				x, y, err = fch.NextFreeSlot(occupied)
				if err != nil {
					return nil, err
				}
			}
			itemBytes := fch.SerializeItemBytes(c.InventoryItemVersion, x, y, fch.StableHash(ni.Name), ni.Stack, quality)
			edits = append(edits, fch.InsertItemEdit(c, itemBytes))
		}
	}

	// Emit grid-move edits for any surviving item whose final position
	// differs from where it started.
	for i, pos := range finalPos {
		it := &c.Inventory[i]
		if it.GridX != pos[0] || it.GridY != pos[1] {
			edits = append(edits, fch.SetItemGridPos(it, pos[0], pos[1]))
		}
	}

	if len(removeSet) > 0 || len(newItems) > 0 {
		netCount := len(c.Inventory) - len(removeSet) + len(newItems)
		edits = append(edits, fch.SetInventoryCountEdit(c, uint16(netCount)))
	}

	return edits, nil
}
