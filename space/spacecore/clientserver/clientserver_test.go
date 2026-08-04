package clientserver

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
)

type fixture struct {
	*clientServer
	yamux *yamuxStub
	quic  *quicStub
}

func newFixture(t *testing.T) *fixture {
	yamuxStub := &yamuxStub{}
	quicStub := &quicStub{}
	s := New().(*clientServer)
	s.yamux = yamuxStub
	s.quic = quicStub
	t.Cleanup(yamuxStub.closeAll)
	return &fixture{
		clientServer: s,
		yamux:        yamuxStub,
		quic:         quicStub,
	}
}

type yamuxStub struct {
	listeners []net.Listener
}

func (y *yamuxStub) AddListener(lis net.Listener) {
	y.listeners = append(y.listeners, lis)
}

func (y *yamuxStub) closeAll() {
	for _, lis := range y.listeners {
		_ = lis.Close()
	}
}

type quicStub struct {
	failures int
	calls    []string
}

func (q *quicStub) ListenAddrs(_ context.Context, addrs ...string) ([]net.Addr, error) {
	q.calls = append(q.calls, addrs...)
	if q.failures > 0 {
		q.failures--
		return nil, errors.New("bind: address already in use")
	}
	res := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		port, err := strconv.Atoi(addr[strings.LastIndex(addr, ":")+1:])
		if err != nil {
			return nil, err
		}
		res = append(res, &net.UDPAddr{IP: net.IPv4zero, Port: port})
	}
	return res, nil
}

func TestListen(t *testing.T) {
	t.Run("tcp listener stays bound and is handed to yamux", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		port, err := fx.listen(context.Background(), 0)

		// then
		require.NoError(t, err)
		require.Greater(t, port, 0)
		require.Len(t, fx.yamux.listeners, 1)
		assert.Equal(t, []string{"0.0.0.0:" + strconv.Itoa(port)}, fx.quic.calls)

		lisPort, err := fx.parsePort(fx.yamux.listeners[0].Addr().String())
		require.NoError(t, err)
		assert.Equal(t, port, lisPort)

		conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
		require.NoError(t, err)
		_ = conn.Close()
	})

	t.Run("saved port is reused when free", func(t *testing.T) {
		// given
		fx := newFixture(t)
		probe, err := net.Listen("tcp", ":")
		require.NoError(t, err)
		savedPort, err := fx.parsePort(probe.Addr().String())
		require.NoError(t, err)
		require.NoError(t, probe.Close())

		// when
		port, err := fx.listen(context.Background(), savedPort)

		// then
		require.NoError(t, err)
		assert.Equal(t, savedPort, port)
	})

	t.Run("falls back to another port when udp side is occupied", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.quic.failures = 1

		// when
		port, err := fx.listen(context.Background(), 0)

		// then
		require.NoError(t, err)
		require.Greater(t, port, 0)
		// the failed attempt's tcp listener must be closed and not registered
		require.Len(t, fx.quic.calls, 2)
		require.Len(t, fx.yamux.listeners, 1)
		lisPort, err := fx.parsePort(fx.yamux.listeners[0].Addr().String())
		require.NoError(t, err)
		assert.Equal(t, port, lisPort)
	})

	t.Run("saved port busy on tcp falls back to another port", func(t *testing.T) {
		// given
		fx := newFixture(t)
		occupied, err := net.Listen("tcp", ":")
		require.NoError(t, err)
		defer occupied.Close()
		savedPort, err := fx.parsePort(occupied.Addr().String())
		require.NoError(t, err)

		// when
		port, err := fx.listen(context.Background(), savedPort)

		// then
		require.NoError(t, err)
		require.Greater(t, port, 0)
		assert.NotEqual(t, savedPort, port)
	})

	t.Run("gives up after all attempts fail", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.quic.failures = listenAttempts

		// when
		_, err := fx.listen(context.Background(), 0)

		// then
		require.Error(t, err)
		assert.Empty(t, fx.yamux.listeners)
	})
}

func TestStartServer(t *testing.T) {
	t.Run("port is persisted and reused across restarts", func(t *testing.T) {
		// given
		provider, err := anystoreprovider.NewInPath(t.TempDir())
		require.NoError(t, err)
		fx := newFixture(t)
		fx.provider = provider

		// when
		require.NoError(t, fx.startServer(context.Background()))
		firstPort := fx.Port()
		fx.yamux.closeAll()
		fx.yamux.listeners = nil

		fx2 := newFixture(t)
		fx2.provider = provider
		require.NoError(t, fx2.startServer(context.Background()))

		// then
		assert.Equal(t, firstPort, fx2.Port())
	})
}
