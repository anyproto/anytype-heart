package anystoreprovider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdOpen is a hook that holds the open reaching it until release is called.
type holdOpen struct {
	reached chan struct{}
	release chan struct{}
}

func newHoldOpen() *holdOpen {
	return &holdOpen{reached: make(chan struct{}), release: make(chan struct{})}
}

func (h *holdOpen) hook(string) {
	close(h.reached)
	<-h.release
}

// seedV2Db leaves a closed v2 database at path, as an earlier run would.
func seedV2Db(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	db, err := anystore.Open(context.Background(), path, nil)
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

// assertNotOpen fails when the database at path is still open in this process (a leaked handle):
// any-store refuses to open a file twice in one process.
func assertNotOpen(t *testing.T, path string) {
	t.Helper()
	db, err := anystore.Open(context.Background(), path, nil)
	require.NoError(t, err, "the database is still open in this process")
	require.NoError(t, db.Close())
}

func TestCloseWithASpaceIndexOpenInFlight(t *testing.T) {
	t.Run("close waits for the open, which registers nothing", func(t *testing.T) {
		// given: an open held between its migration and its registration
		p := newTestProvider(t)
		seedV2Db(t, filepath.Join(p.objectStorePath, "space1", "objects.db"))
		hold := newHoldOpen()
		p.beforeIndexRegister = hold.hook
		opened := make(chan error, 1)
		go func() {
			_, err := p.GetSpaceIndexDb("space1")
			opened <- err
		}()
		<-hold.reached

		// when
		closed := make(chan error, 1)
		go func() { closed <- p.Close(context.Background()) }()
		require.Eventually(t, p.gate.isClosed, time.Second, time.Millisecond)
		select {
		case <-closed:
			t.Fatal("Close returned with an open in flight")
		case <-time.After(50 * time.Millisecond):
		}
		close(hold.release)

		// then
		assert.ErrorIs(t, <-opened, ErrClosed)
		require.NoError(t, <-closed)
		assert.Empty(t, p.spaceIndexDbs)
		assertNotOpen(t, filepath.Join(p.objectStorePath, "space1", "objects.db"))
	})

	t.Run("an open outlasting close's wait closes nothing it never registered", func(t *testing.T) {
		// given
		was := closeOpensWait
		closeOpensWait = 20 * time.Millisecond
		t.Cleanup(func() { closeOpensWait = was })
		p := newTestProvider(t)
		seedV2Db(t, filepath.Join(p.objectStorePath, "space1", "objects.db"))
		hold := newHoldOpen()
		p.beforeIndexRegister = hold.hook
		opened := make(chan error, 1)
		go func() {
			_, err := p.GetSpaceIndexDb("space1")
			opened <- err
		}()
		<-hold.reached

		// when: close gives up waiting, then the open goes on
		require.NoError(t, p.Close(context.Background()))
		close(hold.release)

		// then
		assert.ErrorIs(t, <-opened, ErrClosed)
		assert.Empty(t, p.spaceIndexDbs)
		assertNotOpen(t, filepath.Join(p.objectStorePath, "space1", "objects.db"))
	})
}

func TestCloseWithACrdtOpenInFlight(t *testing.T) {
	// given: an open of a crdt database held after it opened, before it is published, past
	// close's wait
	was := closeOpensWait
	closeOpensWait = 20 * time.Millisecond
	t.Cleanup(func() { closeOpensWait = was })
	p := newTestProvider(t)
	hold := newHoldOpen()
	p.beforeCrdtPublish = hold.hook
	getter := p.GetCrdtDb("space1")
	opened := make(chan error, 1)
	go func() {
		_, err := getter.Wait()
		opened <- err
	}()
	<-hold.reached

	// when
	require.NoError(t, p.Close(context.Background()))
	close(hold.release)

	// then: the open closes the database it opened instead of publishing it
	assert.ErrorIs(t, <-opened, ErrClosed)
	assert.Nil(t, getter.get())
	assertNotOpen(t, filepath.Join(p.objectStorePath, "space1", "crdt.db"))
}

func TestOpensAfterCloseAreRefused(t *testing.T) {
	// given
	p := newTestProvider(t)
	getter := p.GetCrdtDb("space1")
	require.NoError(t, p.Close(context.Background()))

	// when
	_, indexErr := p.GetSpaceIndexDb("space1")
	_, crdtErr := getter.Wait()

	// then
	assert.ErrorIs(t, indexErr, ErrClosed)
	assert.ErrorIs(t, crdtErr, ErrClosed)
}

// An open of a space index is slow when it migrates or copies the database: it holds only its own
// space's path lock, never the lock every other open and every flush takes.
func TestSpaceIndexOpenInFlightBlocksNoOtherSpace(t *testing.T) {
	// given: space1's open held after it opened its database, before it registers it
	p := newTestProvider(t)
	seedV2Db(t, filepath.Join(p.objectStorePath, "space1", "objects.db"))
	seedV2Db(t, filepath.Join(p.objectStorePath, "space2", "objects.db"))
	hold := newHoldOpen()
	p.beforeIndexRegister = func(spaceId string) {
		if spaceId == "space1" {
			hold.hook(spaceId)
		}
	}
	opened := make(chan error, 1)
	go func() {
		_, err := p.GetSpaceIndexDb("space1")
		opened <- err
	}()
	<-hold.reached

	// when
	done := make(chan error, 1)
	go func() {
		_, err := p.GetSpaceIndexDb("space2")
		p.Flush(time.Second, false, anystore.FlushModeCheckpointPassive)
		done <- err
	}()

	// then
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("another space's open or a flush waited for space1's open")
	}
	close(hold.release)
	require.NoError(t, <-opened)
}

// Close stops waiting for an open after closeOpensWait. The open goes on, and the next provider of
// the same store opens that database only once the open has finished with the file.
func TestOpenOutlivingCloseIsFencedFromTheNextProvider(t *testing.T) {
	// given: an open of space1's index outlasting close's wait
	was := closeOpensWait
	closeOpensWait = 20 * time.Millisecond
	t.Cleanup(func() { closeOpensWait = was })
	root := t.TempDir()
	first, err := NewInPath(root)
	require.NoError(t, err)
	p := first.(*provider)
	path := filepath.Join(p.objectStorePath, "space1", "objects.db")
	seedV2Db(t, path)
	hold := newHoldOpen()
	p.beforeIndexRegister = hold.hook
	opened := make(chan error, 1)
	go func() {
		_, err := p.GetSpaceIndexDb("space1")
		opened <- err
	}()
	<-hold.reached
	start := time.Now()
	require.NoError(t, p.Close(context.Background()))
	assert.Less(t, time.Since(start), time.Second, "close does not wait for the open")

	// when: the next provider opens the same database
	second, err := NewInPath(root)
	require.NoError(t, err)
	next := second.(*provider)
	t.Cleanup(func() { _ = next.Close(context.Background()) })
	got := make(chan error, 1)
	go func() {
		_, err := next.GetSpaceIndexDb("space1")
		got <- err
	}()

	// then: it waits for the first provider's open, which gives its database up
	select {
	case err := <-got:
		t.Fatalf("opened while the first provider's open held the file: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(hold.release)
	assert.ErrorIs(t, <-opened, ErrClosed)
	require.NoError(t, <-got)
}
