package fch

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"
)

// StableHash reproduces Valheim's GetStableHashCode (used for prefab name
// hashing), reverse-engineered from assembly_valheim.dll.
func StableHash(s string) int32 {
	hash1 := int32(5381)
	hash2 := hash1
	n := len(s)
	i := 0
	for i < n && s[i] != 0 {
		hash1 = (int32(uint32(hash1)<<5) + hash1) ^ int32(s[i])
		if i == n-1 || s[i+1] == 0 {
			break
		}
		hash2 = (int32(uint32(hash2)<<5) + hash2) ^ int32(s[i+1])
		i += 2
	}
	return hash1 + hash2*1566083941
}

//go:embed itemnames.txt
var candidateNamesRaw string

//go:embed item_display_names.json
var displayNamesRaw []byte

//go:embed icons
var iconsFS embed.FS

var prefabByHash map[int32]string
var displayNameByPrefab map[string]string

func init() {
	prefabByHash = map[int32]string{}
	for _, line := range strings.Split(candidateNamesRaw, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		prefabByHash[StableHash(name)] = name
	}

	displayNameByPrefab = map[string]string{}
	if err := json.Unmarshal(displayNamesRaw, &displayNameByPrefab); err != nil {
		panic("fch: malformed item_display_names.json: " + err.Error())
	}

	locKeyByPrefab = map[string]string{}
	if err := json.Unmarshal(locKeysRaw, &locKeyByPrefab); err != nil {
		panic("fch: malformed loc_keys.json: " + err.Error())
	}

	maxQualityByPrefab = map[string]int{}
	if err := json.Unmarshal(maxQualityRaw, &maxQualityByPrefab); err != nil {
		panic("fch: malformed max_quality.json: " + err.Error())
	}

	itemTypeByPrefab = map[string]int{}
	if err := json.Unmarshal(itemTypesRaw, &itemTypeByPrefab); err != nil {
		panic("fch: malformed item_types.json: " + err.Error())
	}
}

// DefaultMaxQuality is used for a prefab we have no extracted max-quality
// data for (e.g. an exact prefab name typed in free-text that isn't in our
// known-item database) -- permissive rather than restrictive, since we'd
// rather not block a legitimate edit just because we lack data for it.
const DefaultMaxQuality = 9

// MaxQualityForPrefab returns the highest quality level this item can
// legitimately reach in-game (extracted from the installed game's own
// ItemDrop.m_shared.m_maxQuality), or DefaultMaxQuality if unknown.
func MaxQualityForPrefab(prefab string) int {
	if q, ok := maxQualityByPrefab[prefab]; ok {
		return q
	}
	return DefaultMaxQuality
}

// ForsakenPowers lists Valheim's boss "Forsaken Power" unique keys (the
// exact strings the game itself writes to the uniques list when a boss
// stone is activated, confirmed via decompiling assembly_valheim.dll and
// scanning the installed game's own StatusEffect assets) alongside a
// display name, in boss progression order.
var ForsakenPowers = []struct{ Key, Name string }{
	{"GP_Eikthyr", "Eikthyr"},
	{"GP_TheElder", "The Elder"},
	{"GP_Bonemass", "Bonemass"},
	{"GP_Moder", "Moder"},
	{"GP_Yagluth", "Yagluth"},
	{"GP_Queen", "The Seeker Queen"},
	{"GP_Fader", "Fader"},
}

// ResolvePrefabName looks up the internal prefab name for a hash found in
// inventory item data. Returns "" if unresolved (the underlying hash is
// always preserved regardless, so an unresolved name never loses data).
func ResolvePrefabName(hash int32) string {
	return prefabByHash[hash]
}

// ResolveDisplayName looks up the real in-game display name (the text shown
// in Valheim's own UI, e.g. "Storm Fang" rather than the internal prefab
// name "BowAshlandsStorm") for a prefab hash. Returns "" if unresolved.
func ResolveDisplayName(hash int32) string {
	prefab := prefabByHash[hash]
	if prefab == "" {
		return ""
	}
	return displayNameByPrefab[prefab]
}

// ResolveDisplayNameForPrefab is ResolveDisplayName but keyed directly by
// prefab name instead of a hash -- for fields that store the plain prefab
// name as text (trophies, foods, beard/hair) rather than its hash.
func ResolveDisplayNameForPrefab(prefab string) string {
	return displayNameByPrefab[prefab]
}

// IconPNG returns the raw PNG bytes for a prefab's item icon (extracted from
// the installed game's own sprite atlas), or nil if this prefab has no
// known icon.
func IconPNG(prefab string) []byte {
	data, err := iconsFS.ReadFile("icons/" + prefab + ".png")
	if err != nil {
		return nil
	}
	return data
}

// HasIcon reports whether IconPNG would return data for this prefab.
func HasIcon(prefab string) bool {
	_, err := iconsFS.Open("icons/" + prefab + ".png")
	return err == nil
}

// KnownItemOptions returns picker entries formatted as "PrefabName —
// DisplayName", one for every prefab confirmed to be a real, addable
// Valheim item (it has an actual ItemDrop component and a real in-game
// display name, both pulled from the installed game's own asset bundles --
// see item_display_names.json). This deliberately excludes the much larger
// set of names in itemnames.txt used for hash *resolution*, since that list
// also contains non-item scene/animation names that would be invalid to
// add as an inventory item. Sorted by display name.
func KnownItemOptions() []string {
	return knownOptionsWithPrefix("")
}

// KnownTrophyOptions is KnownItemOptions restricted to trophy prefabs
// ("TrophyX"), for a trophy picker.
func KnownTrophyOptions() []string {
	return knownOptionsWithPrefix("Trophy")
}

// KnownBeardOptions and KnownHairOptions list the real beard/hair style
// prefabs (with their real in-game display names) for an appearance
// picker.
func KnownBeardOptions() []string { return knownOptionsWithPrefix("Beard") }
func KnownHairOptions() []string  { return knownOptionsWithPrefix("Hair") }

func knownOptionsWithPrefix(prefix string) []string {
	type entry struct{ prefab, display string }
	entries := make([]entry, 0, len(displayNameByPrefab))
	for prefab, display := range displayNameByPrefab {
		if prefix != "" && !strings.HasPrefix(prefab, prefix) {
			continue
		}
		entries = append(entries, entry{prefab, display})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].display < entries[j].display })

	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.prefab + " — " + e.display
	}
	return out
}

//go:embed loc_keys.json
var locKeysRaw []byte

//go:embed max_quality.json
var maxQualityRaw []byte

//go:embed item_types.json
var itemTypesRaw []byte

var locKeyByPrefab map[string]string
var maxQualityByPrefab map[string]int

// itemTypeByPrefab maps a prefab name to Valheim's own raw ItemType enum
// value (extracted from the installed game's ItemDrop.m_shared.m_itemType).
// The mapping from that raw int to a human browsing category below was
// derived empirically -- by cross-referencing hundreds of well-known
// prefabs (e.g. every "Helmet*" prefab, every "Trophy*" prefab) against
// their actual scanned raw value and taking the dominant value per group --
// rather than by trusting the enum's in-game display names, since a
// metadata-parsing quirk made the assembly's own enum-constant table
// unreliable to read directly for this project.
var itemTypeByPrefab map[string]int

// ItemCategory buckets a prefab into a human-browsable group, for a
// by-type item picker. Prefabs with no known raw type (not a real item) or
// whose raw type is "Customization" (hair/beard styles, not addable
// inventory items) return "".
func ItemCategory(prefab string) string {
	raw, ok := itemTypeByPrefab[prefab]
	if !ok {
		return ""
	}
	switch raw {
	case 3, 4, 22: // one-handed weapons, bows, staves
		return "Weapons"
	case 14: // two-handed weapons & staves, but also pickaxes -- split by name
		if strings.HasPrefix(prefab, "Pickaxe") {
			return "Tools"
		}
		return "Weapons"
	case 5:
		return "Shields"
	case 6, 7, 11, 17:
		return "Armor"
	case 9, 23:
		return "Ammo"
	case 15, 19:
		return "Tools"
	case 2:
		return "Food & Potions"
	case 1, 21:
		return "Materials & Resources"
	case 13:
		return "Trophies"
	case 18:
		return "Utility"
	case 24:
		return "Trinkets"
	case 16:
		return "Misc"
	case 10: // Customization (hair/beard styles) -- not a real inventory item
		return ""
	default:
		return ""
	}
}

// ItemCatalogEntry is one browsable entry in the by-type item picker.
type ItemCatalogEntry struct {
	Prefab   string `json:"prefab"`
	Name     string `json:"name"`
	Category string `json:"category"`
	HasIcon  bool   `json:"hasIcon"`
}

// itemCategoryOrder fixes a sensible display order for KnownItemCatalog,
// rather than sorting categories alphabetically.
var itemCategoryOrder = []string{
	"Weapons", "Shields", "Armor", "Ammo", "Tools",
	"Food & Potions", "Materials & Resources", "Trophies", "Utility", "Trinkets", "Misc",
}

// KnownItemCatalog returns every known real item grouped into browsing
// categories (see ItemCategory), sorted by category (in itemCategoryOrder)
// then display name -- for a by-type item picker, as an alternative to
// searching by name.
func KnownItemCatalog() []ItemCatalogEntry {
	catRank := map[string]int{}
	for i, c := range itemCategoryOrder {
		catRank[c] = i
	}

	var out []ItemCatalogEntry
	for prefab, display := range displayNameByPrefab {
		cat := ItemCategory(prefab)
		if cat == "" {
			continue
		}
		out = append(out, ItemCatalogEntry{Prefab: prefab, Name: display, Category: cat, HasIcon: HasIcon(prefab)})
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := catRank[out[i].Category], catRank[out[j].Category]
		if ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// AllRecipeKeys returns the localization-key form ("$item_swordiron") of
// every known item/piece prefab, suitable for bulk-adding to a character's
// known recipes or known materials lists (which store entries in this
// form, not the raw prefab name).
func AllRecipeKeys() []string {
	out := make([]string, 0, len(locKeyByPrefab))
	for _, key := range locKeyByPrefab {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// KnownSkillTypes lists every real, trainable Valheim skill type (excludes
// the enum's None=0 and All=999 sentinels), in a stable display order.
var KnownSkillTypes = []int32{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14,
	100, 101, 102, 103, 104, 105, 106, 107, 108, 110,
}

// SkillTypeNames maps Valheim's Skills.SkillType enum values (confirmed via
// decompiling the installed assembly_valheim.dll for game version 1.0.12)
// to their display names.
var SkillTypeNames = map[int32]string{
	0:   "None",
	1:   "Swords",
	2:   "Knives",
	3:   "Clubs",
	4:   "Polearms",
	5:   "Spears",
	6:   "Blocking",
	7:   "Axes",
	8:   "Bows",
	9:   "Elemental Magic",
	10:  "Blood Magic",
	11:  "Unarmed",
	12:  "Pickaxes",
	13:  "Wood Cutting",
	14:  "Crossbows",
	100: "Jump",
	101: "Sneak",
	102: "Run",
	103: "Swim",
	104: "Fishing",
	105: "Cooking",
	106: "Farming",
	107: "Crafting",
	108: "Dodge",
	110: "Ride",
	999: "All",
}
