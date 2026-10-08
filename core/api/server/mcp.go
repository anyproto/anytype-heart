package server

// mcp.go — the MCP delivery on the API server: POST /mcp/{tier} carries one
// JSON-RPC message per request (Streamable HTTP, JSON responses only), over
// the same origin policy, bearer authentication and key-scope gate as /v2
// (docs/superpowers/specs/2026-09-18-full-wrapper-in-heart-design.md §3).
//
// Every tool call runs on behalf of the caller: the session's client goes
// through the in-process transport with the bearer the /mcp request
// carried, so the inner /v2 call is authenticated, scoped and grant-gated
// exactly as a REST call with that key — the transport injects nothing.
//
// Sessions exist for the curated tiers, whose handles are state: initialize
// mints an id bound to the key and the tier, and every later message must
// carry it. The full tier is stateless and issues none.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
	"github.com/anyproto/anytype-heart/core/api/wrapper/full"
)

const (
	mcpSessionHeader  = "Mcp-Session-Id"
	mcpProtocolHeader = "MCP-Protocol-Version"

	// mcpSessionIdle ends a curated-tier session nobody has used for this
	// long; the client re-initialises on the 404.
	mcpSessionIdle = 30 * time.Minute
	// mcpMaxSessions and mcpMaxSessionsPerKey cap admission; past them a
	// new initialize is refused with 429 until a session ends or expires.
	mcpMaxSessions       = 64
	mcpMaxSessionsPerKey = 8
	// mcpMaxQueue bounds the calls waiting on one session's serialized
	// runner; past it a call is refused with 429 instead of queueing.
	mcpMaxQueue = 8
	// mcpCallTimeout bounds one tool call end to end.
	mcpCallTimeout = 60 * time.Second
	// mcpMaxCallsPerKey bounds the calls one key has in flight across all
	// tiers and sessions, taken before the body is read.
	mcpMaxCallsPerKey = 8
)

// mcpProtocolVersions are the revisions the HTTP delivery answers, oldest
// first; the last is what an unknown request gets. Chosen for this
// delivery rather than reused from stdio: the profile below refuses
// batches, which 2025-03-26 allowed.
var mcpProtocolVersions = []string{"2025-06-18", "2025-11-25"}

// mcpSession is one curated-tier conversation: its MCP loop over a Runner
// whose client carries the key that opened it. A reserved session (server
// still nil) counts against the caps but resolves for nobody.
type mcpSession struct {
	id       string
	key      string
	tier     wrapper.Tier
	server   *wrapper.MCPServer
	lastUsed time.Time
	// slots bounds the calls queued on this session (mcpMaxQueue).
	slots chan struct{}
}

// mcpSessions is the session table and the per-key call gates.
type mcpSessions struct {
	mu        sync.Mutex
	byId      map[string]*mcpSession
	gates     map[string]chan struct{}
	now       func() time.Time
	idle      time.Duration
	maxTotal  int
	maxPerKey int
	callsPer  int
}

func newMCPSessions() *mcpSessions {
	return &mcpSessions{byId: map[string]*mcpSession{}, gates: map[string]chan struct{}{}, now: time.Now,
		idle: mcpSessionIdle, maxTotal: mcpMaxSessions, maxPerKey: mcpMaxSessionsPerKey, callsPer: mcpMaxCallsPerKey}
}

var errMCPSessionCap = errors.New("too many MCP sessions")

// admit takes one of the key's call slots without waiting, before anything
// about the request is read; the returned release gives it back.
func (m *mcpSessions) admit(key string) (func(), bool) {
	m.mu.Lock()
	gate, ok := m.gates[key]
	if !ok {
		gate = make(chan struct{}, m.callsPer)
		m.gates[key] = gate
	}
	m.mu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, true
	default:
		return nil, false
	}
}

// reserve admits a new session for key on tier, or refuses at a cap,
// before anything is built for it. Expired sessions are swept first so a
// cap is counted over live ones. The caller fills the server in (ready) or
// gives the reservation back (release).
func (m *mcpSessions) reserve(key string, tier wrapper.Tier) (*mcpSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	perKey := 0
	for id, s := range m.byId {
		if now.Sub(s.lastUsed) > m.idle {
			delete(m.byId, id)
			continue
		}
		if s.key == key {
			perKey++
		}
	}
	if len(m.byId) >= m.maxTotal || perKey >= m.maxPerKey {
		return nil, errMCPSessionCap
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("mint session id: %w", err)
	}
	s := &mcpSession{id: hex.EncodeToString(b[:]), key: key, tier: tier, lastUsed: now, slots: make(chan struct{}, mcpMaxQueue)}
	m.byId[s.id] = s
	return s, nil
}

// ready makes a reserved session usable.
func (m *mcpSessions) ready(s *mcpSession, server *wrapper.MCPServer) {
	m.mu.Lock()
	s.server = server
	m.mu.Unlock()
}

// release gives a reservation back.
func (m *mcpSessions) release(s *mcpSession) {
	m.mu.Lock()
	delete(m.byId, s.id)
	m.mu.Unlock()
}

// live returns the session id names if key opened it on tier and it has
// not idled out. A foreign key, another tier, a reservation and an expired
// or unknown id are one answer: the caller learns nothing about sessions
// that are not its own. touch refreshes the idle clock.
func (m *mcpSessions) live(id, key string, tier wrapper.Tier, touch bool) (*mcpSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byId[id]
	if !ok {
		return nil, false
	}
	now := m.now()
	if now.Sub(s.lastUsed) > m.idle {
		delete(m.byId, id)
		return nil, false
	}
	if s.key != key || s.tier != tier || s.server == nil {
		return nil, false
	}
	if touch {
		s.lastUsed = now
	}
	return s, true
}

// end removes a live session the key owns on the tier.
func (m *mcpSessions) end(id, key string, tier wrapper.Tier) bool {
	if _, ok := m.live(id, key, tier, false); !ok {
		return false
	}
	m.mu.Lock()
	delete(m.byId, id)
	m.mu.Unlock()
	return true
}

// registerMCPRoutes mounts the delivery under the shared gates.
func (srv *Server) registerMCPRoutes(router *gin.Engine, mw apicore.ClientCommands) {
	if srv.mcp == nil {
		srv.mcp = newMCPSessions()
	}
	group := router.Group("/mcp")
	group.Use(srv.ensureAuthenticated(mw))
	group.Use(ensureJsonApiScope())
	group.POST("/:tier", srv.handleMCPPost)
	group.DELETE("/:tier", srv.handleMCPDelete)
	group.GET("/:tier", srv.handleMCPGet)
}

// servedTier parses the route's tier; the CLI's ParseTier does not know
// the full tier, which only this delivery serves.
func servedTier(s string) (wrapper.Tier, bool) {
	switch wrapper.Tier(s) {
	case wrapper.TierSmall, wrapper.TierLarge, wrapper.TierFull:
		return wrapper.Tier(s), true
	}
	return "", false
}

// bearerOf returns the key the authenticated request carried.
func bearerOf(c *gin.Context) string {
	return strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
}

// inProcessClient builds the client a session's tools call /v2 through:
// the in-process transport, resolving the engine per request, with the
// caller's own key — never another.
func (srv *Server) inProcessClient(bearer string) *wrapper.Client {
	client := wrapper.NewClient(InProcessBaseURL, bearer)
	client.HTTP = &http.Client{Transport: NewInProcessTransport(srv.resolveEngine), Timeout: mcpCallTimeout}
	return client
}

// resolveEngine is the in-process transport's engine source.
func (srv *Server) resolveEngine() (http.Handler, error) {
	if srv.engine == nil {
		return nil, errors.New("api engine is not built")
	}
	return srv.engine, nil
}

// errNoFullTable is the composition gap: this server was built without the
// full table.
var errNoFullTable = errors.New("the full tool table is not available on this server")

// mcpServerFor builds the MCP loop for one tier over the caller's client.
func (srv *Server) mcpServerFor(tier wrapper.Tier, bearer string) (*wrapper.MCPServer, error) {
	client := srv.inProcessClient(bearer)
	var server *wrapper.MCPServer
	if tier == wrapper.TierFull {
		if srv.fullTable == nil {
			return nil, errNoFullTable
		}
		table, err := srv.fullTable()
		if err != nil {
			return nil, fmt.Errorf("full tool table: %w", err)
		}
		server = wrapper.NewMCPServerOver(full.NewExecutor(client, table), tier, InProcessBaseURL)
	} else {
		server = wrapper.NewMCPServer(wrapper.NewRunner(client, wrapper.NewMemoryStore()), tier)
	}
	server.SetProtocolVersions(mcpProtocolVersions...)
	return server, nil
}

// mcpRefuse answers an HTTP-level refusal in the shared error shape.
func mcpRefuse(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, util.CodeToApiError(status, message))
}

// mcpPreamble checks what every method shares: the tier and the protocol
// version header. It answers the refusal itself and reports whether to go
// on.
func mcpPreamble(c *gin.Context) (wrapper.Tier, bool) {
	tier, ok := servedTier(c.Param("tier"))
	if !ok {
		mcpRefuse(c, http.StatusNotFound,
			fmt.Sprintf("unknown MCP tier %q — tiers: %s, %s, %s", c.Param("tier"), wrapper.TierSmall, wrapper.TierLarge, wrapper.TierFull))
		return "", false
	}
	if v := c.GetHeader(mcpProtocolHeader); v != "" && !supportedMCPVersion(v) {
		mcpRefuse(c, http.StatusBadRequest,
			fmt.Sprintf("unsupported %s %q — this server speaks %s", mcpProtocolHeader, v, strings.Join(mcpProtocolVersions, ", ")))
		return "", false
	}
	return tier, true
}

// errBodyTimeout is a request body that did not arrive in time.
var errBodyTimeout = errors.New("request body read timed out")

// readBodyWithin reads at most limit bytes within d. The read deadline is
// set on the connection where the server supports it; the timer is the
// backstop that ends the wait regardless.
func readBodyWithin(c *gin.Context, limit int64, d time.Duration) ([]byte, error) {
	_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(d))
	body := http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	type read struct {
		data []byte
		err  error
	}
	done := make(chan read, 1)
	go func() {
		data, err := io.ReadAll(body)
		done <- read{data, err}
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.data, r.err
	case <-timer.C:
		return nil, errBodyTimeout
	}
}

// mcpBodyTimeout bounds how long one message may take to arrive.
var mcpBodyTimeout = 10 * time.Second

// handleMCPPost serves one message. The order is the admission contract:
// the key's call slot is taken before the body is read; the message is
// classified before anything is built for it; a session is reserved
// against the caps before its runner exists; and the slot is given back
// only when every handler the call started has returned.
func (srv *Server) handleMCPPost(c *gin.Context) {
	tier, ok := mcpPreamble(c)
	if !ok {
		return
	}
	bearer := bearerOf(c)
	releaseKey, ok := srv.mcp.admit(bearer)
	if !ok {
		mcpRefuse(c, http.StatusTooManyRequests,
			fmt.Sprintf("too many MCP calls in flight for this key (at most %d) — wait for one to finish", srv.mcp.callsPer))
		return
	}
	var releases []func()
	releases = append(releases, releaseKey)
	lease := NewCallLease(func() {
		for _, r := range releases {
			r()
		}
	})
	defer lease.Done()

	body, err := readBodyWithin(c, wrapper.MCPMaxMessageBytes, mcpBodyTimeout)
	if err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			mcpRefuse(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("one MCP message may be at most %d bytes", wrapper.MCPMaxMessageBytes))
		case errors.Is(err, errBodyTimeout):
			mcpRefuse(c, http.StatusRequestTimeout, fmt.Sprintf("the message did not arrive within %s", mcpBodyTimeout))
		default:
			mcpRefuse(c, http.StatusBadRequest, "read request body: "+err.Error())
		}
		return
	}

	classified := wrapper.ClassifyMessage(body)
	switch classified.Kind {
	case wrapper.MessageInvalid:
		c.JSON(http.StatusBadRequest, classified.Response)
		return
	case wrapper.MessageNotification, wrapper.MessageResponse:
		// acknowledged, run nothing; a curated-tier notification that names
		// a session must still name a live one of the caller's
		if id := c.GetHeader(mcpSessionHeader); id != "" && tier != wrapper.TierFull {
			if _, ok := srv.mcp.live(id, bearer, tier, true); !ok {
				mcpRefuse(c, http.StatusNotFound, "unknown or expired MCP session — start a new one with initialize")
				return
			}
		}
		c.Status(http.StatusAccepted)
		return
	}

	var server *wrapper.MCPServer
	var session *mcpSession
	switch {
	case tier == wrapper.TierFull:
		// stateless: one loop per request, nothing kept
		server, err = srv.mcpServerFor(tier, bearer)
		if err != nil {
			mcpBuildFailed(c, err)
			return
		}
	case c.GetHeader(mcpSessionHeader) != "":
		session, ok = srv.mcp.live(c.GetHeader(mcpSessionHeader), bearer, tier, true)
		if !ok {
			mcpRefuse(c, http.StatusNotFound,
				"unknown or expired MCP session — start a new one with initialize and no "+mcpSessionHeader)
			return
		}
		server = session.server
	case classified.Method == "initialize":
		session, err = srv.mcp.reserve(bearer, tier)
		if err != nil {
			if errors.Is(err, errMCPSessionCap) {
				mcpRefuse(c, http.StatusTooManyRequests,
					"too many MCP sessions for this key or this server — end one (DELETE with its "+mcpSessionHeader+") or wait for one to expire")
				return
			}
			mcpRefuse(c, http.StatusInternalServerError, err.Error())
			return
		}
		server, err = srv.mcpServerFor(tier, bearer)
		if err != nil {
			srv.mcp.release(session)
			mcpBuildFailed(c, err)
			return
		}
		srv.mcp.ready(session, server)
	default:
		mcpRefuse(c, http.StatusBadRequest,
			mcpSessionHeader+" is required on the "+string(tier)+" tier after initialize — send initialize first and echo the id it returns")
		return
	}

	if session != nil {
		select {
		case session.slots <- struct{}{}:
			releases = append(releases, func() { <-session.slots })
		default:
			mcpRefuse(c, http.StatusTooManyRequests,
				fmt.Sprintf("too many calls queued on this MCP session (at most %d) — wait for one to finish", mcpMaxQueue))
			return
		}
	}
	ctx, cancel := context.WithTimeout(WithCallLease(c.Request.Context(), lease), mcpCallTimeout)
	defer cancel()
	handled := server.HandleMessage(ctx, body)
	if session != nil && handled.Kind == wrapper.MessageRequest && handled.Method == "initialize" {
		c.Header(mcpSessionHeader, session.id)
	}
	c.JSON(http.StatusOK, handled.Response)
}

// mcpBuildFailed answers a failure to build the tier's loop: a missing
// table is a composition gap (503), a derivation failure a defect (500).
func mcpBuildFailed(c *gin.Context, err error) {
	if errors.Is(err, errNoFullTable) {
		mcpRefuse(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	mcpRefuse(c, http.StatusInternalServerError, err.Error())
}

// handleMCPGet: this delivery pushes nothing, so there is no stream to
// open — but a GET naming a session that is not live is told so first, the
// way every other method is.
func (srv *Server) handleMCPGet(c *gin.Context) {
	tier, ok := mcpPreamble(c)
	if !ok {
		return
	}
	if id := c.GetHeader(mcpSessionHeader); id != "" {
		if _, ok := srv.mcp.live(id, bearerOf(c), tier, false); !ok {
			mcpRefuse(c, http.StatusNotFound, "unknown or expired MCP session")
			return
		}
	}
	c.Header("Allow", "POST, DELETE")
	mcpRefuse(c, http.StatusMethodNotAllowed,
		"the MCP endpoint takes POST (one JSON-RPC message) and DELETE (end the session); it pushes nothing, so there is no GET stream")
}

// handleMCPDelete ends a curated-tier session.
func (srv *Server) handleMCPDelete(c *gin.Context) {
	tier, ok := mcpPreamble(c)
	if !ok {
		return
	}
	id := c.GetHeader(mcpSessionHeader)
	if tier == wrapper.TierFull || id == "" {
		mcpRefuse(c, http.StatusBadRequest,
			"DELETE ends a small or large tier session named by "+mcpSessionHeader+"; the full tier keeps none")
		return
	}
	if !srv.mcp.end(id, bearerOf(c), tier) {
		mcpRefuse(c, http.StatusNotFound, "unknown or expired MCP session")
		return
	}
	c.Status(http.StatusNoContent)
}

func supportedMCPVersion(v string) bool {
	for _, s := range mcpProtocolVersions {
		if s == v {
			return true
		}
	}
	return false
}
