package creature

// Equipment helper methods. The Slot enum and Equipment struct live in
// creature.go; the methods on this file centralize the field<->slot
// mapping so command handlers, persistence, and tests don't have to
// switch on the field name themselves.

// Get returns the item id at the named single-occupancy slot, or 0 if
// empty / unknown. Multi-item slots (BeltPouches, WornMisc) return 0
// because they are slices; callers needing those go through the
// fields directly.
func (e *Equipment) Get(slot Slot) int64 {
	switch slot {
	case SlotArmor:
		return e.Armor
	case SlotShield:
		return e.Shield
	case SlotPrimaryWield:
		return e.PrimaryWield
	case SlotOffHand:
		return e.OffHand
	case SlotOutfit:
		return e.Outfit
	case SlotCloak:
		return e.Cloak
	case SlotBackpack:
		return e.Backpack
	case SlotHeldInHand:
		return e.HeldInHand
	case SlotMount:
		return e.Mount
	}
	return 0
}

// Set assigns itemID to the named slot and returns the displaced id
// (0 if the slot was empty). Callers must persist the result.
// Unknown slots are no-ops returning 0.
func (e *Equipment) Set(slot Slot, itemID int64) int64 {
	switch slot {
	case SlotArmor:
		prev := e.Armor
		e.Armor = itemID
		return prev
	case SlotShield:
		prev := e.Shield
		e.Shield = itemID
		return prev
	case SlotPrimaryWield:
		prev := e.PrimaryWield
		e.PrimaryWield = itemID
		return prev
	case SlotOffHand:
		prev := e.OffHand
		e.OffHand = itemID
		return prev
	case SlotOutfit:
		prev := e.Outfit
		e.Outfit = itemID
		return prev
	case SlotCloak:
		prev := e.Cloak
		e.Cloak = itemID
		return prev
	case SlotBackpack:
		prev := e.Backpack
		e.Backpack = itemID
		return prev
	case SlotHeldInHand:
		prev := e.HeldInHand
		e.HeldInHand = itemID
		return prev
	case SlotMount:
		prev := e.Mount
		e.Mount = itemID
		return prev
	}
	return 0
}

// Clear empties the named slot and returns the previously-equipped id
// (0 if it was already empty / unknown).
func (e *Equipment) Clear(slot Slot) int64 {
	return e.Set(slot, 0)
}

// FindByItem returns the slot currently holding itemID. The bool is
// false when the item isn't in any single-occupancy slot. Multi-item
// slices are searched too: BeltPouches and WornMisc returns
// SlotBackpack / SlotOutfit respectively as the closest singular
// approximation only when explicitly needed by callers — the bool
// stays false for slice membership so callers can decide whether to
// scan the slices themselves.
func (e *Equipment) FindByItem(itemID int64) (Slot, bool) {
	if itemID == 0 {
		return 0, false
	}
	switch itemID {
	case e.Armor:
		return SlotArmor, true
	case e.Shield:
		return SlotShield, true
	case e.PrimaryWield:
		return SlotPrimaryWield, true
	case e.OffHand:
		return SlotOffHand, true
	case e.Outfit:
		return SlotOutfit, true
	case e.Cloak:
		return SlotCloak, true
	case e.Backpack:
		return SlotBackpack, true
	case e.HeldInHand:
		return SlotHeldInHand, true
	case e.Mount:
		return SlotMount, true
	}
	return 0, false
}

// ClearItem removes itemID wherever it appears in single-occupancy
// slots and returns the slot it was cleared from. Use after drop /
// give / put to keep equipment_json from holding a dangling pointer.
func (e *Equipment) ClearItem(itemID int64) (Slot, bool) {
	slot, ok := e.FindByItem(itemID)
	if !ok {
		return 0, false
	}
	e.Clear(slot)
	return slot, true
}

// Label returns the player-facing word for a slot ("worn", "wielded",
// "offhand", ...). Used by the inventory annotator and `equipment`.
func (s Slot) Label() string {
	switch s {
	case SlotArmor:
		return "worn"
	case SlotShield:
		return "shield"
	case SlotPrimaryWield:
		return "wielded"
	case SlotOffHand:
		return "offhand"
	case SlotOutfit:
		return "worn"
	case SlotCloak:
		return "worn"
	case SlotBackpack:
		return "worn"
	case SlotHeldInHand:
		return "held"
	case SlotMount:
		return "mount"
	}
	return ""
}

// SlotName is the human-readable slot name used by `equipment`.
func (s Slot) SlotName() string {
	switch s {
	case SlotArmor:
		return "armor"
	case SlotShield:
		return "shield"
	case SlotPrimaryWield:
		return "primary hand"
	case SlotOffHand:
		return "off hand"
	case SlotOutfit:
		return "outfit"
	case SlotCloak:
		return "cloak"
	case SlotBackpack:
		return "backpack"
	case SlotHeldInHand:
		return "held"
	case SlotMount:
		return "mount"
	}
	return "unknown"
}
