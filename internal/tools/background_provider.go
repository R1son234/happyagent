package tools

import (
	"context"

	"happyagent/internal/background"
)

type backgroundStoreContextKey struct{}

func WithBackgroundStore(ctx context.Context, store *background.Store) context.Context {
	return context.WithValue(ctx, backgroundStoreContextKey{}, store)
}

func BackgroundStoreFromContext(ctx context.Context) *background.Store {
	if ctx == nil {
		return nil
	}
	store, _ := ctx.Value(backgroundStoreContextKey{}).(*background.Store)
	return store
}
