package fch

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
)

// encodeString builds the raw ZPackage bytes for a string field: a 7-bit
// numitems length prefix (1 byte for anything under 128 bytes, which every
// use in this file is) followed by the UTF-8 text.
func encodeString(s string) []byte {
	b := []byte(s)
	if len(b) >= 128 {
		panic("encodeString: string too long for single-byte numitems prefix")
	}
	return append([]byte{byte(len(b))}, b...)
}

func f32Bytes(v float32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func u16Bytes(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

func i32Bytes(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

func i64Bytes(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// --- simple in-place field edits (all fixed-size, delta-free) ---

func EditFloat(offset Range, v float32) Edit {
	return Edit{Start: offset[0], End: offset[1], NewBytes: f32Bytes(v)}
}

func EditU16(offset Range, v uint16) Edit {
	return Edit{Start: offset[0], End: offset[1], NewBytes: u16Bytes(v)}
}

func EditI32(offset Range, v int32) Edit {
	return Edit{Start: offset[0], End: offset[1], NewBytes: i32Bytes(v)}
}

func EditVec3(offset Range, v [3]float32) Edit {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:], math.Float32bits(v[0]))
	binary.LittleEndian.PutUint32(b[4:], math.Float32bits(v[1]))
	binary.LittleEndian.PutUint32(b[8:], math.Float32bits(v[2]))
	return Edit{Start: offset[0], End: offset[1], NewBytes: b}
}

// ItemFieldUpdate describes desired new values for an item's optional
// (flag-gated) fields. Leave a pointer nil to not touch that field.
type ItemFieldUpdate struct {
	Quality     *uint16
	Stack       *uint16
	Variant     *int32
	CrafterID   *int64
	CrafterName *string
}

// SetItemFields updates any combination of an item's optional fields
// (quality, stack, variant, crafter). Any of them may be entirely absent
// from the item's byte layout -- many items are stored without a quality,
// stack, variant, or crafter field at all when they were picked up at the
// implicit default (quality 1, stack 1, variant 0, no crafter), since
// Valheim only writes a field when its flag bit is set. In that case this
// transparently adds the field (setting the flag bit and splicing in the
// value at the correct schema position: quality, then stack, then variant,
// then crafterID+crafterName, in that fixed order) instead of failing. To
// avoid growing an untouched item on every save, a field already absent is
// left alone if the requested value equals its implicit default. A
// requested quality is clamped to this specific item's real in-game
// maximum (MaxQualityForPrefab) so it can't end up in a quality tier the
// item was never designed to reach.
func SetItemFields(it *InventoryItem, u ItemFieldUpdate) ([]Edit, error) {
	flagsOff, ok := it.Offsets["flags"]
	if !ok {
		return nil, fmt.Errorf("item has no flags field in its byte layout")
	}
	newFlags := it.Flags
	var edits []Edit

	if u.Quality != nil {
		maxQ := uint16(MaxQualityForPrefab(ResolvePrefabName(it.PrefabHash)))
		clamped := *u.Quality
		if clamped > maxQ {
			clamped = maxQ
		}
		if clamped < 1 {
			clamped = 1
		}
		u.Quality = &clamped
	}

	cursor := flagsOff[1]
	var pending []byte
	flush := func() {
		if len(pending) > 0 {
			edits = append(edits, Edit{Start: cursor, End: cursor, NewBytes: pending})
			pending = nil
		}
	}

	if off, ok := it.Offsets["quality"]; ok {
		flush()
		if u.Quality != nil {
			edits = append(edits, EditU16(off, *u.Quality))
		}
		cursor = off[1]
	} else if u.Quality != nil && *u.Quality != 1 {
		newFlags |= 4
		pending = append(pending, u16Bytes(*u.Quality)...)
	}

	if off, ok := it.Offsets["stack"]; ok {
		flush()
		if u.Stack != nil {
			edits = append(edits, EditU16(off, *u.Stack))
		}
		cursor = off[1]
	} else if u.Stack != nil && *u.Stack != 1 {
		newFlags |= 8
		pending = append(pending, u16Bytes(*u.Stack)...)
	}

	if off, ok := it.Offsets["variant"]; ok {
		flush()
		if u.Variant != nil {
			edits = append(edits, EditI32(off, *u.Variant))
		}
		cursor = off[1]
	} else if u.Variant != nil && *u.Variant != 0 {
		newFlags |= 16
		pending = append(pending, i32Bytes(*u.Variant)...)
	}

	idOff, hasID := it.Offsets["crafterID"]
	nameOff, hasName := it.Offsets["crafterName"]
	if hasID && hasName {
		flush()
		if u.CrafterID != nil {
			edits = append(edits, Edit{Start: idOff[0], End: idOff[1], NewBytes: i64Bytes(*u.CrafterID)})
		}
		if u.CrafterName != nil {
			edits = append(edits, Edit{Start: nameOff[0], End: nameOff[1], NewBytes: encodeString(*u.CrafterName)})
		}
		cursor = nameOff[1]
	} else if u.CrafterID != nil || u.CrafterName != nil {
		id := it.CrafterID
		if u.CrafterID != nil {
			id = *u.CrafterID
		}
		nm := it.CrafterName
		if u.CrafterName != nil {
			nm = *u.CrafterName
		}
		if id != 0 || nm != "" {
			newFlags |= 32
			pending = append(pending, i64Bytes(id)...)
			pending = append(pending, encodeString(nm)...)
		}
	}
	flush()

	if newFlags != it.Flags {
		edits = append(edits, Edit{Start: flagsOff[0], End: flagsOff[1], NewBytes: []byte{newFlags}})
	}

	return edits, nil
}

// SetItemQualityStack is a convenience wrapper around SetItemFields for
// callers that only need to touch quality and/or stack.
func SetItemQualityStack(it *InventoryItem, quality *uint16, stack *uint16) ([]Edit, error) {
	return SetItemFields(it, ItemFieldUpdate{Quality: quality, Stack: stack})
}

// SetItemQuality is a convenience wrapper around SetItemFields for callers
// that only need to touch quality.
func SetItemQuality(it *InventoryItem, quality uint16) ([]Edit, error) {
	return SetItemFields(it, ItemFieldUpdate{Quality: &quality})
}

// SetItemStack is a convenience wrapper around SetItemFields for callers
// that only need to touch stack.
func SetItemStack(it *InventoryItem, stack uint16) ([]Edit, error) {
	return SetItemFields(it, ItemFieldUpdate{Stack: &stack})
}

// SetItemDurabilityValue edits the item's stored durability field directly,
// in the same raw int32 units already shown by InventoryItem.Durability.
func SetItemDurabilityValue(it *InventoryItem, raw int32) Edit {
	off := it.Offsets["durability"]
	return EditI32(off, raw)
}

func SetSkillLevel(s *Skill, level float32) Edit {
	return EditFloat(s.LevelOffset, level)
}

// SetSkillLevels sets the level for any number of skill types in one
// batch. A skill type already present is a simple in-place overwrite. A
// skill type the character has never trained at all isn't in the byte
// layout yet (Valheim only serializes skills that have been touched) --
// for those this inserts a brand new skill entry (skipped for a requested
// level of 0, since an absent skill is already implicitly level 0). All
// insertions are combined into one blob with a single net count-field
// update, so this is safe to call once per save even when unlocking
// several previously-untrained skills at once (see the uniques/trophies
// version of this same batching problem).
func SetSkillLevels(c *Character, levels map[int32]float32) []Edit {
	var edits []Edit
	byType := map[int32]*Skill{}
	for i := range c.Skills {
		byType[c.Skills[i].Type] = &c.Skills[i]
	}

	var insertBlob []byte
	added := 0
	for skillType, level := range levels {
		if s, ok := byType[skillType]; ok {
			edits = append(edits, SetSkillLevel(s, level))
			continue
		}
		if level == 0 {
			continue
		}
		insertBlob = append(insertBlob, i32Bytes(skillType)...)
		insertBlob = append(insertBlob, f32Bytes(level)...)
		if c.SkillsVersion >= 2 {
			insertBlob = append(insertBlob, f32Bytes(0)...)
		}
		added++
	}
	if insertBlob != nil {
		edits = append(edits, Edit{Start: c.SkillsEndPos, End: c.SkillsEndPos, NewBytes: insertBlob})
	}
	if added != 0 {
		edits = append(edits, EditI32(c.SkillsCountOffset, int32(len(c.Skills)+added)))
	}
	return edits
}

// SetItemGridPos edits an existing item's inventory grid coordinates
// in place (a fixed 2-byte [x,y] pair present on every item).
func SetItemGridPos(it *InventoryItem, x, y uint8) Edit {
	off := it.Offsets["gridpos"]
	return Edit{Start: off[0], End: off[1], NewBytes: []byte{x, y}}
}

// --- inventory insertion (variable length: bumps the item count and splices
// a brand new ItemData record in after the last existing item) ---

func serializeNewItem(itemVersion int32, gridX, gridY uint8, prefabHash int32, stack uint16, quality uint16, durability int32) []byte {
	flags := uint8(1) // pickedUp
	if stack > 0 {
		flags |= 8
	}
	if quality > 1 {
		flags |= 4
	}
	if prefabHash != 0 {
		flags |= 64
	}

	buf := make([]byte, 0, 20)
	buf = append(buf, i32Bytes(durability)...)
	buf = append(buf, gridX, gridY)
	buf = append(buf, 0) // worldLevel
	buf = append(buf, flags)
	if flags&4 != 0 {
		buf = append(buf, u16Bytes(quality)...)
	}
	if flags&8 != 0 {
		buf = append(buf, u16Bytes(stack)...)
	}
	if flags&64 != 0 {
		buf = append(buf, i32Bytes(prefabHash)...)
	}
	if itemVersion >= 109 || itemVersion == 107 {
		buf = append(buf, 0) // cheated = false
	}
	return buf
}

// GridSlot is an inventory grid cell coordinate.
type GridSlot = [2]uint8

// OccupiedGridSlots returns the set of grid cells currently in use, letting
// the caller exclude items that are being removed in the same edit batch so
// their slots can be reused by items being added.
func OccupiedGridSlots(c *Character, excludeIndexes map[int]bool) map[GridSlot]bool {
	occupied := map[GridSlot]bool{}
	for i, it := range c.Inventory {
		if excludeIndexes[i] {
			continue
		}
		occupied[GridSlot{it.GridX, it.GridY}] = true
	}
	return occupied
}

// NextFreeSlot scans an 8-wide grid (Valheim's default player inventory
// width) row by row for the first cell not marked occupied, then marks it
// occupied so repeated calls hand out distinct slots.
func NextFreeSlot(occupied map[GridSlot]bool) (uint8, uint8, error) {
	for y := uint8(0); y < 10; y++ {
		for x := uint8(0); x < 8; x++ {
			slot := GridSlot{x, y}
			if !occupied[slot] {
				occupied[slot] = true
				return x, y, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("no free inventory slot found")
}

// FreeGridSlot finds an unused inventory grid cell among the character's
// current inventory.
func FreeGridSlot(c *Character) (uint8, uint8, error) {
	return NextFreeSlot(OccupiedGridSlots(c, nil))
}

// SerializeItemBytes builds a complete ItemData byte record for a brand new
// stack, in the same byte layout parseItemData reads.
func SerializeItemBytes(itemVersion int32, gridX, gridY uint8, prefabHash int32, stack uint16, quality uint16) []byte {
	return serializeNewItem(itemVersion, gridX, gridY, prefabHash, stack, quality, 0)
}

// InsertItemEdit returns the edit that appends itemBytes right after the
// last existing inventory item. Callers are responsible for also emitting a
// SetInventoryCountEdit reflecting the net item count change.
func InsertItemEdit(c *Character, itemBytes []byte) Edit {
	return Edit{Start: c.InventoryEndPos, End: c.InventoryEndPos, NewBytes: itemBytes}
}

// RemoveItemEdit returns the edit that deletes an existing inventory item's
// entire byte range. Callers are responsible for also emitting a
// SetInventoryCountEdit reflecting the net item count change.
func RemoveItemEdit(c *Character, index int) (Edit, error) {
	if index < 0 || index >= len(c.Inventory) {
		return Edit{}, fmt.Errorf("item index %d out of range", index)
	}
	it := &c.Inventory[index]
	return Edit{Start: it.Range[0], End: it.Range[1], NewBytes: nil}, nil
}

// SetInventoryCountEdit edits the inventory's item-count field directly.
func SetInventoryCountEdit(c *Character, count uint16) Edit {
	return Edit{Start: c.InventoryCountOffset[0], End: c.InventoryCountOffset[1], NewBytes: u16Bytes(count)}
}

// SetInventoryRows sets the character's total inventory height (in rows),
// matching Valheim's own vendor-upgrade mechanic: it rewrites (or, if the
// character has never bought an expansion, inserts) an "invrows N" entry in
// the player's uniques list -- the exact same string the game's own
// Player.SetInventorySize writes on purchase, clamped [0,9] there too.
// Because every valid value is a single digit, an existing entry is always
// a same-length in-place overwrite; only a genuinely absent entry needs an
// insert (which bumps the uniques list's own count field).
func SetInventoryRows(c *Character, rows int) ([]Edit, error) {
	if rows < 0 || rows > 9 {
		return nil, fmt.Errorf("inventory rows must be between 0 and 9 (Valheim's own clamp), got %d", rows)
	}
	text := "invrows " + strconv.Itoa(rows)
	if c.InvRowsOffset != (Range{}) {
		return []Edit{{Start: c.InvRowsOffset[0], End: c.InvRowsOffset[1], NewBytes: encodeString(text)}}, nil
	}
	return []Edit{
		{Start: c.UniquesEndPos, End: c.UniquesEndPos, NewBytes: encodeString(text)},
		EditI32(c.UniquesCountOffset, int32(len(c.Uniques)+1)),
	}, nil
}

// AddItem returns the edits needed to append a brand new stack to the
// inventory: bumping the item count and inserting a new ItemData record
// right after the last existing item.
func AddItem(c *Character, prefabName string, stack uint16, quality uint16) ([]Edit, error) {
	x, y, err := FreeGridSlot(c)
	if err != nil {
		return nil, err
	}
	hash := StableHash(prefabName)
	itemBytes := serializeNewItem(c.InventoryItemVersion, x, y, hash, stack, quality, 0)

	countEdit := Edit{
		Start:    c.InventoryCountOffset[0],
		End:      c.InventoryCountOffset[1],
		NewBytes: u16Bytes(uint16(len(c.Inventory) + 1)),
	}
	insertEdit := Edit{
		Start:    c.InventoryEndPos,
		End:      c.InventoryEndPos,
		NewBytes: itemBytes,
	}
	return []Edit{countEdit, insertEdit}, nil
}

// --- generic plain string-list toggling (uniques flags, trophies, etc) ---

// SetUniqueFlags adds the given plain (no-value) entries to the uniques
// list where not already present, and removes the given entries where
// present -- e.g. add "GP_Bonemass" to unlock a Forsaken Power, or remove
// it to lock it again. A key in both lists is left untouched. All the
// changes are batched into a single net count-field update, so this is
// safe to call once per save even when toggling several flags at once
// (calling several one-flag-at-a-time edit builders instead would each
// compute their own count edit against the same original length and
// collide with each other).
func SetUniqueFlags(c *Character, add []string, remove []string) []Edit {
	var edits []Edit
	have := map[string]bool{}
	for _, u := range c.Uniques {
		have[u] = true
	}
	removeSet := map[string]bool{}
	for _, key := range remove {
		removeSet[key] = true
	}

	var insertBlob []byte
	added := 0
	for _, key := range add {
		if have[key] || removeSet[key] {
			continue
		}
		insertBlob = append(insertBlob, encodeString(key)...)
		have[key] = true
		added++
	}
	removed := 0
	for i, u := range c.Uniques {
		if removeSet[u] {
			edits = append(edits, Edit{Start: c.UniquesRanges[i][0], End: c.UniquesRanges[i][1], NewBytes: nil})
			removed++
		}
	}
	if insertBlob != nil {
		edits = append(edits, Edit{Start: c.UniquesEndPos, End: c.UniquesEndPos, NewBytes: insertBlob})
	}
	if net := added - removed; net != 0 {
		edits = append(edits, EditI32(c.UniquesCountOffset, int32(len(c.Uniques)+net)))
	}
	return edits
}

// SetTrophies is SetUniqueFlags for the trophies list.
func SetTrophies(c *Character, add []string, remove []string) []Edit {
	var edits []Edit
	have := map[string]bool{}
	for _, t := range c.Trophies {
		have[t] = true
	}
	removeSet := map[string]bool{}
	for _, prefab := range remove {
		removeSet[prefab] = true
	}

	var insertBlob []byte
	added := 0
	for _, prefab := range add {
		if have[prefab] || removeSet[prefab] {
			continue
		}
		insertBlob = append(insertBlob, encodeString(prefab)...)
		have[prefab] = true
		added++
	}
	removed := 0
	for i, t := range c.Trophies {
		if removeSet[t] {
			edits = append(edits, Edit{Start: c.TrophiesRanges[i][0], End: c.TrophiesRanges[i][1], NewBytes: nil})
			removed++
		}
	}
	if insertBlob != nil {
		edits = append(edits, Edit{Start: c.TrophiesEndPos, End: c.TrophiesEndPos, NewBytes: insertBlob})
	}
	if net := added - removed; net != 0 {
		edits = append(edits, EditI32(c.TrophiesCountOffset, int32(len(c.Trophies)+net)))
	}
	return edits
}

// AddRecipes adds any of the given recipe names not already known, in one
// combined insertion (a single count bump covering all of them).
func AddRecipes(c *Character, names []string) []Edit {
	known := map[string]bool{}
	for _, r := range c.Recipes {
		known[r] = true
	}
	var blob []byte
	added := 0
	for _, n := range names {
		if known[n] {
			continue
		}
		blob = append(blob, encodeString(n)...)
		known[n] = true
		added++
	}
	if added == 0 {
		return nil
	}
	return []Edit{
		{Start: c.RecipesEndPos, End: c.RecipesEndPos, NewBytes: blob},
		EditI32(c.RecipesCountOffset, int32(len(c.Recipes)+added)),
	}
}

// AddKnownMaterials adds any of the given material names not already known.
func AddKnownMaterials(c *Character, names []string) []Edit {
	known := map[string]bool{}
	for _, m := range c.KnownMaterial {
		known[m] = true
	}
	var blob []byte
	added := 0
	for _, n := range names {
		if known[n] {
			continue
		}
		blob = append(blob, encodeString(n)...)
		known[n] = true
		added++
	}
	if added == 0 {
		return nil
	}
	return []Edit{
		{Start: c.KnownMaterialEndPos, End: c.KnownMaterialEndPos, NewBytes: blob},
		EditI32(c.KnownMaterialCountOffset, int32(len(c.KnownMaterial)+added)),
	}
}

// AddKnownStations adds any of the given station names not already known,
// each at the given level.
func AddKnownStations(c *Character, names []string, level int32) []Edit {
	var blob []byte
	added := 0
	for _, n := range names {
		if _, ok := c.Stations[n]; ok {
			continue
		}
		blob = append(blob, encodeString(n)...)
		blob = append(blob, i32Bytes(level)...)
		added++
	}
	if added == 0 {
		return nil
	}
	return []Edit{
		{Start: c.StationsEndPos, End: c.StationsEndPos, NewBytes: blob},
		EditI32(c.StationsCountOffset, int32(len(c.Stations)+added)),
	}
}

// SetCharacterName renames the character. This is the one editable field
// that lives outside the playerData blob (in the outer PlayerProfile
// container), which Apply supports the same as anywhere else in
// profileData.
func SetCharacterName(sf *SaveFile, newName string) (Edit, error) {
	if newName == "" {
		return Edit{}, fmt.Errorf("name can't be empty")
	}
	off := sf.NameOffset()
	return Edit{Start: off[0], End: off[1], NewBytes: encodeString(newName)}, nil
}

// SetBeardHair changes the character's beard and/or hair style (the
// prefab name of the style, e.g. "Beard12", "Hair35" -- pass nil to leave
// one unchanged). Both fields always exist in the byte layout for any save
// with version >= 4 (effectively all real saves), so this is always a
// simple variable-length in-place replace, never an insert.
func SetBeardHair(c *Character, beard *string, hair *string) []Edit {
	var edits []Edit
	if beard != nil {
		off := c.Offsets["beard"]
		edits = append(edits, Edit{Start: off[0], End: off[1], NewBytes: encodeString(*beard)})
	}
	if hair != nil {
		off := c.Offsets["hair"]
		edits = append(edits, Edit{Start: off[0], End: off[1], NewBytes: encodeString(*hair)})
	}
	return edits
}

// SetFood changes an active food buff slot's item and/or remaining
// time/health. Pass nil for a value you don't want to change. Changing the
// name is a variable-length replace since food names differ in length.
func SetFood(f *Food, name *string, timeOrHealth *float32) []Edit {
	var edits []Edit
	if name != nil {
		edits = append(edits, Edit{Start: f.NameOffset[0], End: f.NameOffset[1], NewBytes: encodeString(*name)})
	}
	if timeOrHealth != nil {
		edits = append(edits, EditFloat(f.TimeOffset, *timeOrHealth))
	}
	return edits
}

// AddOrIncreaseStack adds `amount` to an existing stack of the given prefab
// if one exists (in-place, no length change); otherwise inserts a brand new
// stack via AddItem. This is the general mechanism behind "add gold".
func AddOrIncreaseStack(c *Character, prefabName string, amount uint16) ([]Edit, error) {
	hash := StableHash(prefabName)
	for i := range c.Inventory {
		it := &c.Inventory[i]
		if it.PrefabHash == hash && it.HasStack() {
			newStack := it.Stack + amount
			edits, err := SetItemStack(it, newStack)
			if err != nil {
				return nil, err
			}
			return edits, nil
		}
	}
	return AddItem(c, prefabName, amount, 1)
}
