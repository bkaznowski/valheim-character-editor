package fch

import (
	"strconv"
	"strings"
)

// DefaultInventoryWidth and DefaultInventoryHeight are Valheim's base
// inventory grid dimensions (confirmed via decompiling Humanoid's own
// constants) before any vendor-purchased row expansion.
const (
	DefaultInventoryWidth  = 8
	DefaultInventoryHeight = 4
)

// InventoryRows returns the character's current total inventory height in
// rows (base 4, plus any vendor-purchased row expansions), read from the
// "invrows N" entry Valheim itself writes into the uniques list on
// purchase. Returns the base default if no such entry exists (a character
// that never bought an expansion).
func (c *Character) InventoryRows() int {
	for _, u := range c.Uniques {
		if strings.HasPrefix(u, "invrows ") {
			if n, err := strconv.Atoi(strings.TrimPrefix(u, "invrows ")); err == nil {
				return n
			}
		}
	}
	return DefaultInventoryHeight
}

// Range is an absolute [start,end) byte offset pair within the original file.
type Range = [2]int

type InventoryItem struct {
	Durability  int32
	GridX       uint8
	GridY       uint8
	WorldLevel  uint8
	Flags       uint8
	Quality     uint16
	Stack       uint16
	Variant     int32
	CrafterID   int64
	CrafterName string
	PrefabHash  int32
	CustomData  map[string]string
	Cheated     bool

	Name string // resolved display name, filled in by the prefab dictionary

	Offsets map[string]Range
	Range   Range
}

func (it *InventoryItem) HasQuality() bool { return it.Flags&4 != 0 }
func (it *InventoryItem) HasStack() bool   { return it.Flags&8 != 0 }
func (it *InventoryItem) HasVariant() bool { return it.Flags&16 != 0 }
func (it *InventoryItem) HasCrafter() bool { return it.Flags&32 != 0 }
func (it *InventoryItem) HasHash() bool    { return it.Flags&64 != 0 }
func (it *InventoryItem) Equipped() bool   { return it.Flags&2 != 0 }
func (it *InventoryItem) PickedUp() bool   { return it.Flags&1 != 0 }

type Skill struct {
	Type        int32
	Level       float32
	Accumulator float32
	LevelOffset Range
}

type Food struct {
	Name    string
	Time    *float32
	Health  *float32
	Stamina *float32

	NameOffset Range // full encoded string range (length prefix + text)
	TimeOffset Range // whichever of Time/Health is actually present for this save version
}

// Character holds the fully parsed player save, including absolute byte
// offsets (into the original file buffer) for every field this editor
// supports changing in place.
type Character struct {
	Version int32

	Name     string
	PlayerID int64
	Seed     string

	MaxHealth float32
	Health    float32
	Stamina   float32

	// GuardianPower is the key of the currently-active Guardian (Forsaken)
	// Power buff (e.g. "GP_Bonemass"), or "" if none is active.
	// GuardianPowerCooldown is the remaining cooldown, in the same seconds
	// unit Valheim itself uses, before another power can be activated.
	GuardianPower         string
	GuardianPowerCooldown float32

	Inventory            []InventoryItem
	InventoryCountOffset Range
	InventoryEndPos      int // absolute offset right after the last item (insertion point)
	InventoryItemVersion int32

	Recipes                  []string
	RecipesCountOffset       Range
	RecipesEndPos            int
	Stations                 map[string]int32
	StationsCountOffset      Range
	StationsEndPos           int
	KnownMaterial            []string
	KnownMaterialCountOffset Range
	KnownMaterialEndPos      int

	ShownTutorials []string
	Uniques        []string
	UniquesRanges  []Range // parallel to Uniques -- each entry's own byte range
	Trophies       []string
	TrophiesRanges []Range // parallel to Trophies
	KnownBiome     []string
	KnownTexts     map[string]string

	UniquesCountOffset  Range
	UniquesEndPos       int // absolute offset right after the last uniques entry (insertion point)
	TrophiesCountOffset Range
	TrophiesEndPos      int
	// InvRowsOffset is the absolute byte range of an existing "invrows N"
	// entry's text within the uniques list, if one is present (Range{} zero
	// value if absent -- a fresh character that never bought an expansion
	// has no such entry at all).
	InvRowsOffset Range

	Beard     string
	Hair      string
	SkinColor [3]float32
	HairColor [3]float32

	PlayerModel       int32
	Foods             []Food
	Skills            []Skill
	SkillsVersion     int32
	SkillsCountOffset Range
	SkillsEndPos      int

	CustomData map[string]string
	Stamina2   float32
	MaxEitr    float32
	Eitr       float32

	BuildUiData []byte

	Offsets map[string]Range
}

func skipStatsSection(r *Reader, version int32) {
	if version >= 44 {
		statCount := int(r.I32())
		numSlots := int(r.I32())
		for s := 0; s < numSlots; s++ {
			for i := 0; i < statCount; i++ {
				r.F32()
			}
			skipStringFloatDict(r) // knownWorlds
			skipStringFloatDict(r) // knownWorldKeys
			skipStringFloatDict(r) // knownCommands
			outer := int(r.I32())  // enemyStats
			for j := 0; j < outer; j++ {
				skipStringFloatDict(r)
			}
			skipStringFloatDict(r) // itemPickupStats
			skipStringFloatDict(r) // itemCraftStats
			skipStringFloatDict(r) // pickableStats
			skipStringFloatDict(r) // foodEatenStats
			skipStringFloatDict(r) // piecesPlacedStats
		}
	} else if version >= 38 {
		n := int(r.I32())
		for i := 0; i < n; i++ {
			r.F32()
		}
	} else if version >= 28 {
		for i := 0; i < 4; i++ {
			r.I32()
		}
	}
}

func skipStringFloatDict(r *Reader) {
	n := int(r.I32())
	for i := 0; i < n; i++ {
		r.String()
		r.F32()
	}
}

func parseItemData(r *Reader, itemVersion int32) InventoryItem {
	item := InventoryItem{Offsets: map[string]Range{}, CustomData: map[string]string{}}
	start := r.Pos

	p := r.Pos
	item.Durability = r.I32()
	item.Offsets["durability"] = r.Range(p)

	p = r.Pos
	item.GridX = r.U8()
	item.GridY = r.U8()
	item.Offsets["gridpos"] = r.Range(p)

	p = r.Pos
	item.WorldLevel = r.U8()
	item.Offsets["worldLevel"] = r.Range(p)

	p = r.Pos
	item.Flags = r.U8()
	item.Offsets["flags"] = r.Range(p)

	if item.HasQuality() {
		p = r.Pos
		item.Quality = r.U16()
		item.Offsets["quality"] = r.Range(p)
	} else {
		item.Quality = 1
	}

	if item.HasStack() {
		p = r.Pos
		item.Stack = r.U16()
		item.Offsets["stack"] = r.Range(p)
	} else {
		item.Stack = 1
	}

	if item.HasVariant() {
		p = r.Pos
		item.Variant = r.I32()
		item.Offsets["variant"] = r.Range(p)
	}

	if item.HasCrafter() {
		p = r.Pos
		item.CrafterID = r.I64()
		item.Offsets["crafterID"] = r.Range(p)
		p = r.Pos
		item.CrafterName = r.String()
		item.Offsets["crafterName"] = r.Range(p)
	}

	if item.HasHash() {
		p = r.Pos
		item.PrefabHash = r.I32()
		item.Offsets["prefab_hash"] = r.Range(p)
	}

	if item.Flags&128 != 0 {
		n := r.NumItems()
		for i := 0; i < n; i++ {
			k := r.String()
			v := r.String()
			item.CustomData[k] = v
		}
	}

	if itemVersion >= 109 || itemVersion == 107 {
		p = r.Pos
		cheatByte := r.U8()
		item.Offsets["cheated"] = r.Range(p)
		item.Cheated = cheatByte&1 != 0
	}

	item.Range = r.Range(start)
	return item
}

func parseInventory(r *Reader) ([]InventoryItem, Range, int, int32) {
	itemVersion := r.I32()
	if itemVersion < 108 {
		panic("old inventory format not supported")
	}
	countStart := r.Pos
	num := int(r.U16())
	countRange := r.Range(countStart)
	items := make([]InventoryItem, 0, num)
	for i := 0; i < num; i++ {
		items = append(items, parseItemData(r, itemVersion))
	}
	return items, countRange, r.Pos, itemVersion
}

// ParsePlayerData parses the inner Player.Load blob. base is the absolute
// offset of blob[0] within the original file, so every recorded field
// offset ends up absolute and directly usable for editing.
func ParsePlayerData(blob []byte, base int) *Character {
	r := &Reader{Data: blob}
	c := &Character{Offsets: map[string]Range{}}

	abs := func(rel Range) Range { return Range{base + rel[0], base + rel[1]} }

	c.Version = r.I32()
	version := c.Version

	if version >= 7 {
		p := r.Pos
		c.MaxHealth = r.F32()
		c.Offsets["maxHealth"] = abs(r.Range(p))
		p = r.Pos
		c.Health = r.F32()
		c.Offsets["health"] = abs(r.Range(p))
	}
	if version >= 10 {
		p := r.Pos
		c.Stamina = r.F32()
		c.Offsets["stamina"] = abs(r.Range(p))
	}
	if version >= 8 && version < 28 {
		r.Bool()
	}
	if version >= 20 {
		r.F32() // timeSinceDeath
	}
	if version >= 23 {
		p := r.Pos
		c.GuardianPower = r.String()
		c.Offsets["guardianPower"] = abs(r.Range(p))
	}
	if version >= 24 {
		p := r.Pos
		c.GuardianPowerCooldown = r.F32()
		c.Offsets["guardianPowerCooldown"] = abs(r.Range(p))
	}
	if version == 2 {
		r.ZDOID()
	}

	items, countRange, invEnd, itemVersion := parseInventory(r)
	c.Inventory = items
	c.InventoryCountOffset = abs(countRange)
	c.InventoryEndPos = base + invEnd
	c.InventoryItemVersion = itemVersion
	// item offsets were recorded relative to blob (since parseItemData uses
	// the same *Reader as the rest of this function) -- fix them up to absolute.
	for i := range c.Inventory {
		for k, v := range c.Inventory[i].Offsets {
			c.Inventory[i].Offsets[k] = abs(v)
		}
		c.Inventory[i].Range = abs(c.Inventory[i].Range)
	}

	recipesCountStart := r.Pos
	numRecipes := int(r.I32())
	c.RecipesCountOffset = abs(r.Range(recipesCountStart))
	c.Recipes = make([]string, numRecipes)
	for i := range c.Recipes {
		c.Recipes[i] = r.String()
	}
	c.RecipesEndPos = base + r.Pos

	c.Stations = map[string]int32{}
	if version < 15 {
		n := int(r.I32())
		for i := 0; i < n; i++ {
			r.String()
		}
	} else {
		stationsCountStart := r.Pos
		n := int(r.I32())
		c.StationsCountOffset = abs(r.Range(stationsCountStart))
		for i := 0; i < n; i++ {
			name := r.String()
			level := r.I32()
			c.Stations[name] = level
		}
		c.StationsEndPos = base + r.Pos
	}

	materialCountStart := r.Pos
	n := int(r.I32())
	c.KnownMaterialCountOffset = abs(r.Range(materialCountStart))
	c.KnownMaterial = make([]string, n)
	for i := range c.KnownMaterial {
		c.KnownMaterial[i] = r.String()
	}
	c.KnownMaterialEndPos = base + r.Pos

	if version >= 19 {
		n := int(r.I32())
		c.ShownTutorials = make([]string, n)
		for i := range c.ShownTutorials {
			c.ShownTutorials[i] = r.String()
		}
	}
	if version >= 6 {
		countStart := r.Pos
		n := int(r.I32())
		c.UniquesCountOffset = abs(r.Range(countStart))
		c.Uniques = make([]string, n)
		c.UniquesRanges = make([]Range, n)
		for i := range c.Uniques {
			p := r.Pos
			c.Uniques[i] = r.String()
			c.UniquesRanges[i] = abs(r.Range(p))
			if strings.HasPrefix(c.Uniques[i], "invrows ") {
				c.InvRowsOffset = abs(r.Range(p))
			}
		}
		c.UniquesEndPos = base + r.Pos
	}
	if version >= 9 {
		countStart := r.Pos
		n := int(r.I32())
		c.TrophiesCountOffset = abs(r.Range(countStart))
		c.Trophies = make([]string, n)
		c.TrophiesRanges = make([]Range, n)
		for i := range c.Trophies {
			p := r.Pos
			c.Trophies[i] = r.String()
			c.TrophiesRanges[i] = abs(r.Range(p))
		}
		c.TrophiesEndPos = base + r.Pos
	}
	if version >= 33 || version == 31 {
		n := int(r.I32())
		c.KnownBiome = make([]string, n)
		for i := range c.KnownBiome {
			c.KnownBiome[i] = r.String()
		}
	} else if version >= 18 && version < 33 {
		n := int(r.I32())
		for i := 0; i < n; i++ {
			r.I32()
		}
	}
	if version >= 22 {
		n := int(r.I32())
		c.KnownTexts = map[string]string{}
		for i := 0; i < n; i++ {
			k := r.String()
			v := r.String()
			c.KnownTexts[k] = v
		}
	}
	if version >= 4 {
		p := r.Pos
		c.Beard = r.String()
		c.Offsets["beard"] = abs(r.Range(p))
		p = r.Pos
		c.Hair = r.String()
		c.Offsets["hair"] = abs(r.Range(p))
	}
	if version >= 5 {
		p := r.Pos
		c.SkinColor = r.Vec3()
		c.Offsets["skinColor"] = abs(r.Range(p))
		p = r.Pos
		c.HairColor = r.Vec3()
		c.Offsets["hairColor"] = abs(r.Range(p))
	}
	if version >= 11 {
		p := r.Pos
		c.PlayerModel = r.I32()
		c.Offsets["playerModel"] = abs(r.Range(p))
	}
	if version >= 12 {
		n := int(r.I32())
		c.Foods = make([]Food, n)
		for i := range c.Foods {
			pName := r.Pos
			name := r.String()
			f := Food{Name: name, NameOffset: abs(r.Range(pName))}
			pTime := r.Pos
			if version >= 25 {
				t := r.F32()
				f.Time = &t
				f.TimeOffset = abs(r.Range(pTime))
			} else {
				h := r.F32()
				f.Health = &h
				f.TimeOffset = abs(r.Range(pTime))
				if version >= 16 {
					s := r.F32()
					f.Stamina = &s
				}
			}
			c.Foods[i] = f
		}
	}
	if version >= 17 {
		c.SkillsVersion = r.I32()
		countStart := r.Pos
		numSkills := int(r.I32())
		c.SkillsCountOffset = abs(r.Range(countStart))
		c.Skills = make([]Skill, numSkills)
		for i := 0; i < numSkills; i++ {
			skillType := r.I32()
			pLevel := r.Pos
			level := r.F32()
			levelRange := abs(r.Range(pLevel))
			var accum float32
			if c.SkillsVersion >= 2 {
				accum = r.F32()
			}
			c.Skills[i] = Skill{Type: skillType, Level: level, Accumulator: accum, LevelOffset: levelRange}
		}
		c.SkillsEndPos = base + r.Pos
	}
	if version >= 26 {
		n := int(r.I32())
		c.CustomData = map[string]string{}
		for i := 0; i < n; i++ {
			k := r.String()
			v := r.String()
			c.CustomData[k] = v
		}
		p := r.Pos
		c.Stamina2 = r.F32()
		c.Offsets["stamina2"] = abs(r.Range(p))
		p = r.Pos
		c.MaxEitr = r.F32()
		c.Offsets["maxEitr"] = abs(r.Range(p))
		p = r.Pos
		c.Eitr = r.F32()
		c.Offsets["eitr"] = abs(r.Range(p))
	}
	if version >= 33 || version == 31 {
		c.BuildUiData = r.ByteArray()
	}

	return c
}
