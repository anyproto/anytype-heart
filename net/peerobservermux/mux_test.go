package peerobservermux

import (
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerobserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorder struct {
	name   string
	events []peerobserver.Event
	log    *[]string
	onCall func(ev peerobserver.Event)
}

func (r *recorder) ObservePeerEvent(ev peerobserver.Event) {
	r.events = append(r.events, ev)
	*r.log = append(*r.log, r.name)
	if r.onCall != nil {
		r.onCall(ev)
	}
}

func TestMux(t *testing.T) {
	newRecorder := func(name string, log *[]string) *recorder {
		return &recorder{name: name, log: log}
	}

	t.Run("fans out in add order", func(t *testing.T) {
		// given
		var log []string
		m := New()
		a, b := newRecorder("a", &log), newRecorder("b", &log)
		m.Add(a)
		m.Add(b)
		ev := peerobserver.Event{Kind: peerobserver.KindDialStarted, PeerId: "p1"}

		// when
		m.ObservePeerEvent(ev)

		// then
		want := []string{"a", "b"}
		assert.Equal(t, want, log)
		assert.Equal(t, []peerobserver.Event{ev}, a.events)
		assert.Equal(t, []peerobserver.Event{ev}, b.events)
	})

	t.Run("an observer added mid-dispatch misses the in-flight event", func(t *testing.T) {
		// given
		var log []string
		m := New()
		late := newRecorder("late", &log)
		first := newRecorder("first", &log)
		first.onCall = func(peerobserver.Event) { m.Add(late) }
		m.Add(first)

		// when
		m.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindDialStarted})
		m.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindConnected})

		// then
		want := []string{"first", "first", "late"}
		assert.Equal(t, want, log)
		require.Len(t, late.events, 1)
		assert.Equal(t, peerobserver.KindConnected, late.events[0].Kind)
	})

	t.Run("a panicking observer does not starve the others", func(t *testing.T) {
		// given
		var log []string
		m := New()
		bad := newRecorder("bad", &log)
		bad.onCall = func(peerobserver.Event) { panic("observer panic") }
		good := newRecorder("good", &log)
		m.Add(bad)
		m.Add(good)

		// when / then
		assert.NotPanics(t, func() { m.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindClosed}) })
		assert.Equal(t, []string{"bad", "good"}, log)
	})

	t.Run("nil observers are ignored", func(t *testing.T) {
		m := New()
		m.Add(nil)
		assert.NotPanics(t, func() { m.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindClosed}) })
	})

	t.Run("fills the peerobserver slot the producers look up", func(t *testing.T) {
		// given
		var log []string
		m := New()
		r := newRecorder("r", &log)
		m.Add(r)
		a := new(app.App)
		a.Register(m)

		// when
		peerobserver.FromApp(a).Notify(peerobserver.Event{Kind: peerobserver.KindDialStarted, PeerId: "p1"})

		// then
		require.Len(t, r.events, 1)
		assert.Equal(t, "p1", r.events[0].PeerId)
	})
}
