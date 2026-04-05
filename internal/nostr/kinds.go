package nostr

// KindClass describes NIP-01 storage and replacement semantics for an event kind.
type KindClass int

const (
	KindRegular KindClass = iota
	KindReplaceable
	KindEphemeral
	KindAddressable
)

// ClassifyKind returns the NIP-01 kind class. Kinds outside the explicit NIP-01
// ranges are treated as regular (stored, not subject to replaceable/addressable rules).
func ClassifyKind(kind int) KindClass {
	switch {
	case kind >= 20000 && kind < 30000:
		return KindEphemeral
	case kind >= 30000 && kind < 40000:
		return KindAddressable
	case kind == 0 || kind == 3 || (kind >= 10000 && kind < 20000):
		return KindReplaceable
	case kind == 1 || kind == 2 || (kind >= 4 && kind < 45) || (kind >= 1000 && kind < 10000):
		return KindRegular
	default:
		return KindRegular
	}
}

// IsRegular reports whether kind is classified as regular per NIP-01.
func IsRegular(kind int) bool { return ClassifyKind(kind) == KindRegular }

// IsReplaceable reports whether kind uses pubkey+kind replacement per NIP-01.
func IsReplaceable(kind int) bool { return ClassifyKind(kind) == KindReplaceable }

// IsEphemeral reports whether kind must not be stored per NIP-01 convention.
func IsEphemeral(kind int) bool { return ClassifyKind(kind) == KindEphemeral }

// IsAddressable reports whether kind uses pubkey+kind+d-tag replacement per NIP-01.
func IsAddressable(kind int) bool { return ClassifyKind(kind) == KindAddressable }
