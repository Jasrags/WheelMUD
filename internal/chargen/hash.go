package chargen

import "hash/fnv"

// HashID stably derives an int32 key from a catalog string id via
// FNV-32a. Character schema keys for feats and skills are int32
// (creature.Character.Feats / .Skills); until those tables are
// authored as proper enums, hashing the catalog id keeps the runtime
// output stable across runs without a manual numbering table.
//
// Both chargen (first-level commit) and the cmd-layer spend verbs
// (`learn`, future `pick feat`) MUST go through this helper so the
// int32 keys round-trip — diverging hashes would orphan persisted
// ranks. FNV-32 has tiny collision probability over the ~30-skill /
// ~25-feat catalogs; the catalog loader can grow a duplicate-hash
// check if a collision ever surfaces (tracked as a polish item in
// chargen_features_followups.md).
func HashID(id string) int32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	return int32(h.Sum32())
}
