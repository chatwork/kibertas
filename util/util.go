package util

import (
	"math/rand"
)

func Int32Ptr(i int32) *int32 { return &i }

func GenerateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz1234567890"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}