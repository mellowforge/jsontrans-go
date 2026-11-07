//go:build !jsontext_experiment

package jsonvalue

import "github.com/mellowforge/jsontrans-go/internal/budget"

func Decode(data []byte, b *budget.Budget) (any, error) { return decodeLegacy(data, b) }
