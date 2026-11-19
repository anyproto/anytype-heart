package anystoremigrate

import "sync"

// pathLocks are the process-wide locks of database paths (LockPath).
var pathLocks = struct {
	sync.Mutex
	held map[string]*pathLock
}{held: make(map[string]*pathLock)}

type pathLock struct {
	sync.Mutex
	// users counts the holders and waiters; the entry goes once it drops to zero
	users int
}

// LockPath takes the process-wide lock of the database at path and returns its release. Whoever
// migrates, opens, upgrades or removes the database holds it, so that work on one file never
// overlaps, whichever component or provider instance does it: an open that outlives its provider's
// Close (its context cancelled, the provider no longer waiting for it) finishes, or gives up, before
// the next provider of the same store can touch the file.
func LockPath(path string) (unlock func()) {
	pathLocks.Lock()
	l, ok := pathLocks.held[path]
	if !ok {
		l = &pathLock{}
		pathLocks.held[path] = l
	}
	l.users++
	pathLocks.Unlock()

	l.Lock()
	return func() {
		l.Unlock()
		pathLocks.Lock()
		l.users--
		if l.users == 0 {
			delete(pathLocks.held, path)
		}
		pathLocks.Unlock()
	}
}
