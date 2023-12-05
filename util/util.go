package util

import (
	"context"
	"math/rand"
	"time"
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

func SleepContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
