package career

import "sync"

var workspaceMutationMu sync.Mutex

func withWorkspaceMutationLock[T any](fn func() (T, error)) (T, error) {
	workspaceMutationMu.Lock()
	defer workspaceMutationMu.Unlock()
	return fn()
}
