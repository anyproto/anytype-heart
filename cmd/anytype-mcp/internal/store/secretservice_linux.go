//go:build linux

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	ssName          = "org.freedesktop.secrets"
	ssPath          = "/org/freedesktop/secrets"
	ssService       = "org.freedesktop.Secret.Service"
	ssCollection    = "org.freedesktop.Secret.Collection"
	ssItem          = "org.freedesktop.Secret.Item"
	ssPrompt        = "org.freedesktop.Secret.Prompt"
	ssNoPrompt      = dbus.ObjectPath("/")
	ssProbeTimeout  = 5 * time.Second
	ssPromptTimeout = 2 * time.Minute
)

// linuxStore keeps one item per connection in the Secret Service's default
// collection, with attributes {service, connection}. The key is held by
// the user's keyring, not by a file Anytype writes; whether it is
// encrypted at rest depends on that keyring having a password.
type linuxStore struct{ service string }

func openPlatform(namespace string) Store { return newPlatformStore(SecretServiceName(namespace)) }

// newPlatformStore takes the service attribute itself; tests use their own.
func newPlatformStore(service string) Store { return &linuxStore{service: service} }

func (l *linuxStore) Kind() string { return "secret-service" }

// ssSecret is the Secret Service's (oayays) struct.
type ssSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

type ssConn struct {
	conn    *dbus.Conn
	svc     dbus.BusObject
	session dbus.ObjectPath
	cancel  context.CancelFunc
}

func (c *ssConn) close() {
	c.cancel()
	_ = c.conn.Close()
}

// busAddress finds the user's session bus without ever launching one.
func busAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	p := filepath.Join(dir, "bus")
	if _, err := os.Stat(p); err == nil {
		return "unix:path=" + p
	}
	return ""
}

// open is the probe: one bounded pass that lets D-Bus start the keyring on
// demand and never asks for an unlock. Only a bus that reports the Secret
// Service name as neither owned nor activatable is ErrNoStore; a bus that
// can't be reached and a keyring that is starting or slow are temporary.
func (l *linuxStore) open(ctx context.Context) (*ssConn, error) {
	addr := busAddress()
	if addr == "" {
		return nil, Temporary(ReasonUnreachable, errors.New("no session bus"))
	}
	connCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(ssProbeTimeout, cancel)
	fail := func(conn *dbus.Conn, reason string, err error) (*ssConn, error) {
		expired := !timer.Stop()
		cancel()
		if conn != nil {
			_ = conn.Close()
		}
		if expired {
			return nil, Temporary(ReasonNotReady, errors.New("the keyring did not answer in time"))
		}
		return nil, Temporary(reason, err)
	}

	conn, err := dbus.Dial(addr, dbus.WithContext(connCtx))
	if err != nil {
		return fail(nil, ReasonUnreachable, err)
	}
	if err := conn.Auth(nil); err != nil {
		return fail(conn, ReasonUnreachable, err)
	}
	if err := conn.Hello(); err != nil {
		return fail(conn, ReasonUnreachable, err)
	}
	bus := conn.BusObject()
	var owned bool
	if err := bus.CallWithContext(connCtx, "org.freedesktop.DBus.NameHasOwner", 0, ssName).Store(&owned); err != nil {
		return fail(conn, ReasonUnreachable, err)
	}
	if !owned {
		var names []string
		if err := bus.CallWithContext(connCtx, "org.freedesktop.DBus.ListActivatableNames", 0).Store(&names); err != nil {
			return fail(conn, ReasonUnreachable, err)
		}
		activatable := false
		for _, n := range names {
			if n == ssName {
				activatable = true
				break
			}
		}
		if !activatable {
			timer.Stop()
			cancel()
			_ = conn.Close()
			return nil, ErrNoStore
		}
		var started uint32
		if err := bus.CallWithContext(connCtx, "org.freedesktop.DBus.StartServiceByName", 0, ssName, uint32(0)).Store(&started); err != nil {
			return fail(conn, ReasonNotReady, err)
		}
	}
	svc := conn.Object(ssName, ssPath)
	var out dbus.Variant
	var session dbus.ObjectPath
	if err := svc.CallWithContext(connCtx, ssService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&out, &session); err != nil {
		return fail(conn, ReasonNotReady, err)
	}
	if !timer.Stop() {
		return fail(conn, ReasonNotReady, nil)
	}
	return &ssConn{conn: conn, svc: svc, session: session, cancel: cancel}, nil
}

// opErr maps a failure after the probe. Nothing here ever means "gone".
func opErr(err error) error {
	var derr dbus.Error
	if errors.As(err, &derr) && strings.HasSuffix(derr.Name, ".IsLocked") {
		return Temporary(ReasonLocked, nil)
	}
	return Temporary(ReasonNotReady, err)
}

// prompt runs a Secret Service prompt and waits for the user. It is only
// reached from an interactive key set.
func (c *ssConn) prompt(ctx context.Context, path dbus.ObjectPath) error {
	ch := make(chan *dbus.Signal, 8)
	c.conn.Signal(ch)
	defer c.conn.RemoveSignal(ch)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(ssPrompt)}
	if err := c.conn.AddMatchSignal(match...); err != nil {
		return opErr(err)
	}
	defer func() { _ = c.conn.RemoveMatchSignal(match...) }()
	obj := c.conn.Object(ssName, path)
	if err := obj.Call(ssPrompt+".Prompt", 0, "").Err; err != nil {
		return opErr(err)
	}
	timeout := time.NewTimer(ssPromptTimeout)
	defer timeout.Stop()
	for {
		select {
		case sig, ok := <-ch:
			if !ok {
				return Temporary(ReasonUnreachable, nil)
			}
			if sig.Path != path || sig.Name != ssPrompt+".Completed" || len(sig.Body) == 0 {
				continue
			}
			if dismissed, _ := sig.Body[0].(bool); dismissed {
				return Temporary(ReasonLocked, errors.New("the unlock prompt was dismissed"))
			}
			return nil
		case <-timeout.C:
			_ = obj.Call(ssPrompt+".Dismiss", 0).Err
			return Temporary(ReasonLocked, errors.New("the unlock prompt timed out"))
		case <-ctx.Done():
			_ = obj.Call(ssPrompt+".Dismiss", 0).Err
			return Temporary(ReasonLocked, ctx.Err())
		}
	}
}

// settle handles the prompt path a call returned: none is fine; one is run
// when interactive and dismissed otherwise.
func (c *ssConn) settle(ctx context.Context, prompt dbus.ObjectPath, interactive bool) error {
	if prompt == ssNoPrompt || prompt == "" {
		return nil
	}
	if interactive {
		return c.prompt(ctx, prompt)
	}
	_ = c.conn.Object(ssName, prompt).Call(ssPrompt+".Dismiss", 0).Err
	return Temporary(ReasonLocked, nil)
}

func (c *ssConn) unlock(ctx context.Context, path dbus.ObjectPath, interactive bool) error {
	if !interactive {
		return Temporary(ReasonLocked, nil)
	}
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := c.svc.CallWithContext(ctx, ssService+".Unlock", 0, []dbus.ObjectPath{path}).Store(&unlocked, &prompt); err != nil {
		return opErr(err)
	}
	return c.settle(ctx, prompt, true)
}

// collection returns the default collection, unlocked.
func (c *ssConn) collection(ctx context.Context, interactive bool) (dbus.BusObject, error) {
	var path dbus.ObjectPath
	if err := c.svc.CallWithContext(ctx, ssService+".ReadAlias", 0, "default").Store(&path); err != nil {
		return nil, opErr(err)
	}
	if path == ssNoPrompt || path == "" {
		return nil, Temporary(ReasonNotReady, errors.New("the keyring has no default collection"))
	}
	coll := c.conn.Object(ssName, path)
	v, err := coll.GetProperty(ssCollection + ".Locked")
	if err != nil {
		return nil, opErr(err)
	}
	if locked, _ := v.Value().(bool); locked {
		if err := c.unlock(ctx, path, interactive); err != nil {
			return nil, err
		}
	}
	return coll, nil
}

func (l *linuxStore) attrs(id string) map[string]string {
	return map[string]string{"service": l.service, "connection": id}
}

func (c *ssConn) search(ctx context.Context, attrs map[string]string) (unlocked, locked []dbus.ObjectPath, err error) {
	if err := c.svc.CallWithContext(ctx, ssService+".SearchItems", 0, attrs).Store(&unlocked, &locked); err != nil {
		return nil, nil, opErr(err)
	}
	return unlocked, locked, nil
}

func (l *linuxStore) Probe(ctx context.Context) error {
	c, err := l.open(ctx)
	if err != nil {
		return err
	}
	defer c.close()
	_, err = c.collection(ctx, false)
	return err
}

func (l *linuxStore) Read(ctx context.Context, id string, interactive bool) ([]byte, error) {
	c, err := l.open(ctx)
	if err != nil {
		return nil, err
	}
	defer c.close()
	unlocked, locked, err := c.search(ctx, l.attrs(id))
	if err != nil {
		return nil, err
	}
	if len(unlocked) == 0 {
		if len(locked) == 0 {
			return nil, ErrNotFound
		}
		if err := c.unlock(ctx, locked[0], interactive); err != nil {
			return nil, err
		}
		unlocked = locked[:1]
	}
	var secret ssSecret
	if err := c.conn.Object(ssName, unlocked[0]).CallWithContext(ctx, ssItem+".GetSecret", 0, c.session).Store(&secret); err != nil {
		return nil, opErr(err)
	}
	return secret.Value, nil
}

func (l *linuxStore) Write(ctx context.Context, id string, value []byte, interactive bool) error {
	c, err := l.open(ctx)
	if err != nil {
		return err
	}
	defer c.close()
	coll, err := c.collection(ctx, interactive)
	if err != nil {
		return err
	}
	props := map[string]dbus.Variant{
		ssItem + ".Label":      dbus.MakeVariant("Anytype MCP: " + id),
		ssItem + ".Attributes": dbus.MakeVariant(l.attrs(id)),
	}
	secret := ssSecret{Session: c.session, Parameters: []byte{}, Value: value, ContentType: "text/plain; charset=utf8"}
	var item, prompt dbus.ObjectPath
	if err := coll.CallWithContext(ctx, ssCollection+".CreateItem", 0, props, secret, true).Store(&item, &prompt); err != nil {
		return opErr(err)
	}
	return c.settle(ctx, prompt, interactive)
}

func (l *linuxStore) Delete(ctx context.Context, id string) error {
	c, err := l.open(ctx)
	if err != nil {
		return err
	}
	defer c.close()
	unlocked, locked, err := c.search(ctx, l.attrs(id))
	if err != nil {
		return err
	}
	if len(unlocked) == 0 {
		if len(locked) > 0 {
			return Temporary(ReasonLocked, nil)
		}
		return ErrNotFound
	}
	for _, item := range unlocked {
		var prompt dbus.ObjectPath
		if err := c.conn.Object(ssName, item).CallWithContext(ctx, ssItem+".Delete", 0).Store(&prompt); err != nil {
			return opErr(err)
		}
		if err := c.settle(ctx, prompt, false); err != nil {
			return err
		}
	}
	return nil
}
