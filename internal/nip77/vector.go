package nip77

import (
	"github.com/michmich112/congee/internal/storage"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip77/negentropy"
	"github.com/nbd-wtf/go-nostr/nip77/negentropy/storage/vector"
)

// BuildVector converts stored sync items into a sealed negentropy vector.
func BuildVector(items []storage.SyncItem) *vector.Vector {
	vec := vector.New()
	for _, it := range items {
		vec.Insert(nostr.Timestamp(it.CreatedAt), it.ID)
	}
	vec.Seal()
	return vec
}

// NewServerNegentropy wraps a sealed vector for relay-side reconciliation.
func NewServerNegentropy(vec *vector.Vector, frameSizeLimit int) *negentropy.Negentropy {
	return negentropy.New(vec, frameSizeLimit)
}

// NewClientNegentropy wraps a sealed vector for client-side reconciliation (upstream pull).
func NewClientNegentropy(vec *vector.Vector, frameSizeLimit int) *negentropy.Negentropy {
	return negentropy.New(vec, frameSizeLimit)
}
