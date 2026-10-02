package build

import (
	"strings"

	"github.com/ushineko/nmsbonker/internal/build/cache"
	"github.com/ushineko/nmsbonker/internal/modscript"
)

/*
Signature is the set of (file, key) pairs a script edits (spec 005 R4.1).

Shared by `mods check`, which compares a library script against the enabled
built-ins, and by the build plan, which reports the same comparison for the
mods in the build (spec 014 R3), so the two never disagree on what an overlap is.

The key is a VALUE_CHANGE_TABLE key where there is one, and the wrapper or
currency name where the edit is a WRAPPER_MULT or a CURRENCY_MULT -- those name
the block they multiply rather than the field, and comparing them by field would
miss the overlap that matters most, two mods multiplying the same reward.

A block with neither (a bare ADD or REMOVE) contributes the file alone, which is
deliberately weak: two mods adding different entries to the reward table do not
overlap in any sense the user can act on.
*/
func Signature(def *modscript.Definition) map[string]bool {
	out := map[string]bool{}
	if def == nil {
		return out
	}
	for _, mod := range def.Modifications {
		for _, ch := range mod.Changes {
			for _, src := range ch.Sources {
				file := cache.Key(src)
				for _, blk := range ch.Blocks {
					for _, vc := range blk.ValueChanges {
						out[signatureKey(file, vc.Key)] = true
					}
					if blk.WrapperMult != nil {
						out[signatureKey(file, blk.WrapperMult.Wrapper)] = true
					}
					if blk.CurrencyMult != nil {
						out[signatureKey(file, "GcRewardMoney:"+blk.CurrencyMult.Currency)] = true
					}
				}
			}
		}
	}
	return out
}

// signatureKey joins a file and a key the way Signature stores them.
func signatureKey(file, key string) string { return file + "\x00" + key }

// splitSignatureKey is the inverse of signatureKey.
func splitSignatureKey(k string) (file, key string) {
	file, key, _ = strings.Cut(k, "\x00")
	return file, key
}
