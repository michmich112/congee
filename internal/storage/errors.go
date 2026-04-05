package storage

import "errors"

// ErrSearchNotImplemented is returned by Store.SearchEvents until NIP-50 is implemented.
var ErrSearchNotImplemented = errors.New("storage: search not implemented")
