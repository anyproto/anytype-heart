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
