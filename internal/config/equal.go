package config

import "reflect"

// Equal reports whether two validated configs are semantically the same.
func Equal(a, b *Config) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(*a, *b)
}
