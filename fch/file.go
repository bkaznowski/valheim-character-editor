package fch

import (
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"os"
)

// SaveFile is a fully parsed .fch file plus everything needed to write
// edited fields back out while recomputing every downstream length prefix
// and the outer SHA-512 checksum.
type SaveFile struct {
	raw []byte

	dataLen             int32
	profileDataStartAbs int
	playerDataLenPosAbs int
	playerDataLen       int32
	playerDataStartAbs  int
	hashLenPosAbs       int
	hashLen             int32
	hashStartAbs        int

	// nameOffset is the absolute byte range of the character's name string
	// within the outer profile container (outside the playerData blob,
	// unlike every other editable field, since the game stores the name at
	// the PlayerProfile level, not inside Player.Save's blob).
	nameOffset Range

	Character *Character
}

// NameOffset is the absolute byte range of the character's name string,
// for building a rename Edit. Edits outside the playerData blob (like this
// one) are supported the same way as anywhere else in profileData -- see
// Apply.
func (sf *SaveFile) NameOffset() Range { return sf.nameOffset }

func Load(path string) (*SaveFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*SaveFile, error) {
	sf := &SaveFile{raw: data}

	r := &Reader{Data: data}
	sf.dataLen = r.I32()
	sf.profileDataStartAbs = r.Pos
	profileData := r.Data[r.Pos : r.Pos+int(sf.dataLen)]
	r.Pos += int(sf.dataLen)

	sf.hashLenPosAbs = r.Pos
	sf.hashLen = r.I32()
	sf.hashStartAbs = r.Pos

	pr := &Reader{Data: profileData}
	version := pr.I32()
	skipStatsSection(pr, version)

	if version >= 40 {
		pr.Bool() // firstSpawn
	}
	numWorldEntries := int(pr.I32())
	for i := 0; i < numWorldEntries; i++ {
		pr.I64() // worldUID
		pr.Bool()
		pr.Vec3() // spawn
		pr.Bool()
		pr.Vec3() // logout
		if version >= 30 {
			pr.Bool()
			pr.Vec3() // death
			pr.Vec3() // home
		}
		if version >= 29 {
			if pr.Bool() {
				pr.ByteArray() // map exploration data
			}
		}
	}

	nameStart := pr.Pos
	name := pr.String()
	sf.nameOffset = Range{sf.profileDataStartAbs + nameStart, sf.profileDataStartAbs + pr.Pos}
	playerID := pr.I64()
	seed := pr.String()

	if version >= 38 {
		pr.Bool() // usedCheats
		pr.I64()  // unixTime
	}

	havePlayerData := pr.Bool()
	if !havePlayerData {
		return nil, fmt.Errorf("save has no player data section")
	}

	sf.playerDataLenPosAbs = sf.profileDataStartAbs + pr.Pos
	playerDataLen := pr.I32()
	sf.playerDataLen = playerDataLen
	sf.playerDataStartAbs = sf.profileDataStartAbs + pr.Pos
	playerData := pr.Data[pr.Pos : pr.Pos+int(playerDataLen)]

	c := ParsePlayerData(playerData, sf.playerDataStartAbs)
	c.Name = name
	c.PlayerID = playerID
	c.Seed = seed
	sf.Character = c

	return sf, nil
}

// Edit describes one byte-range replacement against the ORIGINAL file
// buffer this SaveFile was parsed from.
type Edit struct {
	Start, End int
	NewBytes   []byte
}

// Save applies a set of edits (all of which must fall inside the playerData
// blob) in a single pass, fixes up the playerData length prefix, the outer
// dataLen prefix, and recomputes the trailing SHA-512 checksum, then writes
// the result to outPath.
func (sf *SaveFile) Save(outPath string, edits []Edit) error {
	out, err := sf.Apply(edits)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0644)
}

// Apply performs the same edit + checksum-fixup pass as Save but returns
// the resulting bytes instead of writing them, so callers can chain edits
// or inspect the result before persisting. Edits may fall anywhere inside
// profileData (not just the playerData sub-blob, e.g. the character name
// lives outside it) -- every downstream length prefix that could be
// affected (playerData's own length, the outer profileData length, and the
// position of the playerData length field itself if something before it
// changed size) is recomputed from the edits actually given, along with
// the trailing SHA-512 checksum.
func (sf *SaveFile) Apply(edits []Edit) ([]byte, error) {
	profileDataEnd := sf.profileDataStartAbs + int(sf.dataLen)
	for _, e := range edits {
		if e.Start < sf.profileDataStartAbs || e.End > profileDataEnd || e.Start > e.End {
			return nil, fmt.Errorf("edit range [%d,%d) falls outside profileData [%d,%d)", e.Start, e.End, sf.profileDataStartAbs, profileDataEnd)
		}
	}

	sorted := make([]Edit, len(edits))
	copy(sorted, edits)
	insertionSortEdits(sorted)

	var out []byte
	cursor := 0
	playerDataDelta := 0
	shiftBeforePlayerDataLenField := 0
	for _, e := range sorted {
		if e.Start < cursor {
			return nil, fmt.Errorf("overlapping edits at offset %d", e.Start)
		}
		out = append(out, sf.raw[cursor:e.Start]...)
		out = append(out, e.NewBytes...)
		cursor = e.End

		editDelta := len(e.NewBytes) - (e.End - e.Start)
		if e.Start >= sf.playerDataStartAbs && e.End <= sf.playerDataStartAbs+int(sf.playerDataLen) {
			playerDataDelta += editDelta
		}
		if e.Start < sf.playerDataLenPosAbs {
			shiftBeforePlayerDataLenField += editDelta
		}
	}
	out = append(out, sf.raw[cursor:]...)

	newPlayerDataLenPosAbs := sf.playerDataLenPosAbs + shiftBeforePlayerDataLenField
	newPlayerDataLen := sf.playerDataLen + int32(playerDataDelta)
	binary.LittleEndian.PutUint32(out[newPlayerDataLenPosAbs:], uint32(newPlayerDataLen))

	totalDelta := len(out) - len(sf.raw)
	newDataLen := sf.dataLen + int32(totalDelta)
	binary.LittleEndian.PutUint32(out[0:], uint32(newDataLen))

	newProfileData := out[sf.profileDataStartAbs : sf.profileDataStartAbs+int(newDataLen)]
	newHash := sha512.Sum512(newProfileData)
	if int(sf.hashLen) != len(newHash) {
		return nil, fmt.Errorf("unexpected hash length %d", sf.hashLen)
	}
	newHashStartAbs := sf.profileDataStartAbs + int(newDataLen) + 4
	copy(out[newHashStartAbs:newHashStartAbs+int(sf.hashLen)], newHash[:])

	return out, nil
}

func insertionSortEdits(edits []Edit) {
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j].Start < edits[j-1].Start; j-- {
			edits[j], edits[j-1] = edits[j-1], edits[j]
		}
	}
}
