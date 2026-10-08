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
	"encoding/json"
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
)

// mcpProtocolVersions are the revisions the HTTP delivery answers, oldest
// first; the last is what an unknown request gets. Chosen for this
// delivery rather than reused from stdio: the profile below refuses
// batches, which 2025-03-26 allowed.
var mcpProtocolVersions = []string{"2025-06-18", "2025-11-25"}

// mcpSession is one curated-tier conversation: its MCP loop over a Runner
// whose client carries the key that opened it.
type mcpSession struct {
	id       string
	key      string
	tier     wrapper.Tier
	server   *wrapper.MCPServer
	lastUsed time.Time
	// slots bounds the calls queued on this session (mcpMaxQueue).
	slots chan struct{}
}

// mcpSessions is the session table.
type mcpSessions struct {
	mu        sync.Mutex
	byId      map[string]*mcpSession
	now       func() time.Time
	idle      time.Duration
	maxTotal  int
	maxPerKey int
}

func newMCPSessions() *mcpSessions {
	return &mcpSessions{byId: map[string]*mcpSession{}, now: time.Now, idle: mcpSessionIdle, maxTotal: mcpMaxSessions, maxPerKey: mcpMaxSessionsPerKey}
}

var errMCPSessionCap = errors.New("too many MCP sessions")

// mint admits a new session for key on tier, or refuses at a cap. Expired
// sessions are swept first so a cap is counted over live ones.
func (m *mcpSessions) mint(key string, tier wrapper.Tier, server *wrapper.MCPServer) (*mcpSession, error) {
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
	s := &mcpSession{id: hex.EncodeToString(b[:]), key: key, tier: tier, server: server, lastUsed: now, slots: make(chan struct{}, mcpMaxQueue)}
	m.byId[s.id] = s
	return s, nil
}

// lookup returns the live session id belongs to, if key opened it on tier.
// A foreign key, another tier and an expired or unknown id are one answer:
// the caller learns nothing about sessions that are not its own.
func (m *mcpSessions) lookup(id, key string, tier wrapper.Tier) (*mcpSession, bool) {
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
	if s.key != key || s.tier != tier {
		return nil, false
	}
	s.lastUsed = now
	return s, true
}

// end removes a session the key owns on the tier.
func (m *mcpSessions) end(id, key string, tier wrapper.Tier) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byId[id]
	if !ok || s.key != key || s.tier != tier {
		return false
	}
	delete(m.byId, id)
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
	group.GET("/:tier", func(c *gin.Context) {
		c.Header("Allow", "POST, DELETE")
		c.AbortWithStatusJSON(http.StatusMethodNotAllowed, util.CodeToApiError(http.StatusMethodNotAllowed,
			"the MCP endpoint takes POST (one JSON-RPC message) and DELETE (end the session); it pushes nothing, so there is no GET stream"))
	})
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

// mcpServerFor builds the MCP loop for one tier over the caller's client.
func (srv *Server) mcpServerFor(tier wrapper.Tier, bearer string) (*wrapper.MCPServer, error) {
	client := srv.inProcessClient(bearer)
	var server *wrapper.MCPServer
	if tier == wrapper.TierFull {
		if srv.fullTable == nil {
			return nil, errors.New("the full tool table is not available on this server")
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

// handleMCPPost serves one message.
func (srv *Server) handleMCPPost(c *gin.Context) {
	tier, ok := servedTier(c.Param("tier"))
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotFound, util.CodeToApiError(http.StatusNotFound,
			fmt.Sprintf("unknown MCP tier %q — tiers: %s, %s, %s", c.Param("tier"), wrapper.TierSmall, wrapper.TierLarge, wrapper.TierFull)))
		return
	}
	if v := c.GetHeader(mcpProtocolHeader); v != "" && !supportedMCPVersion(v) {
		c.AbortWithStatusJSON(http.StatusBadRequest, util.CodeToApiError(http.StatusBadRequest,
			fmt.Sprintf("unsupported %s %q — this server speaks %s", mcpProtocolHeader, v, strings.Join(mcpProtocolVersions, ", "))))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, wrapper.MCPMaxMessageBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, util.CodeToApiError(http.StatusRequestEntityTooLarge,
				fmt.Sprintf("one MCP message may be at most %d bytes", wrapper.MCPMaxMessageBytes)))
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, util.CodeToApiError(http.StatusBadRequest, "read request body: "+err.Error()))
		return
	}
	var peek struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &peek)
	bearer := bearerOf(c)

	var server *wrapper.MCPServer
	var session *mcpSession
	switch {
	case tier == wrapper.TierFull:
		// stateless: one loop per request, nothing kept
		server, err = srv.mcpServerFor(tier, bearer)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, util.CodeToApiError(http.StatusServiceUnavailable, err.Error()))
			return
		}
	case c.GetHeader(mcpSessionHeader) != "":
		session, ok = srv.mcp.lookup(c.GetHeader(mcpSessionHeader), bearer, tier)
		if !ok {
			c.AbortWithStatusJSON(http.StatusNotFound, util.CodeToApiError(http.StatusNotFound,
				"unknown or expired MCP session — start a new one with initialize and no "+mcpSessionHeader))
			return
		}
		server = session.server
	case peek.Method == "initialize":
		server, err = srv.mcpServerFor(tier, bearer)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, util.CodeToApiError(http.StatusServiceUnavailable, err.Error()))
			return
		}
		session, err = srv.mcp.mint(bearer, tier, server)
		if err != nil {
			if errors.Is(err, errMCPSessionCap) {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, util.CodeToApiError(http.StatusTooManyRequests,
					"too many MCP sessions for this key or this server — end one (DELETE with its "+mcpSessionHeader+") or wait for one to expire"))
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, util.CodeToApiError(http.StatusInternalServerError, err.Error()))
			return
		}
	default:
		c.AbortWithStatusJSON(http.StatusBadRequest, util.CodeToApiError(http.StatusBadRequest,
			mcpSessionHeader+" is required on the "+string(tier)+" tier after initialize — send initialize first and echo the id it returns"))
		return
	}

	if session != nil {
		select {
		case session.slots <- struct{}{}:
			defer func() { <-session.slots }()
		default:
			c.AbortWithStatusJSON(http.StatusTooManyRequests, util.CodeToApiError(http.StatusTooManyRequests,
				fmt.Sprintf("too many calls queued on this MCP session (at most %d) — wait for one to finish", mcpMaxQueue)))
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), mcpCallTimeout)
	defer cancel()
	handled := server.HandleMessage(ctx, body)
	if session != nil && handled.Kind == wrapper.MessageRequest && handled.Method == "initialize" {
		c.Header(mcpSessionHeader, session.id)
	}
	switch handled.Kind {
	case wrapper.MessageInvalid:
		c.JSON(http.StatusBadRequest, handled.Response)
	case wrapper.MessageRequest:
		c.JSON(http.StatusOK, handled.Response)
	default:
		c.Status(http.StatusAccepted)
	}
}

// handleMCPDelete ends a curated-tier session.
func (srv *Server) handleMCPDelete(c *gin.Context) {
	tier, ok := servedTier(c.Param("tier"))
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotFound, util.CodeToApiError(http.StatusNotFound, fmt.Sprintf("unknown MCP tier %q", c.Param("tier"))))
		return
	}
	id := c.GetHeader(mcpSessionHeader)
	if tier == wrapper.TierFull || id == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, util.CodeToApiError(http.StatusBadRequest,
			"DELETE ends a small or large tier session named by "+mcpSessionHeader+"; the full tier keeps none"))
		return
	}
	if !srv.mcp.end(id, bearerOf(c), tier) {
		c.AbortWithStatusJSON(http.StatusNotFound, util.CodeToApiError(http.StatusNotFound, "unknown or expired MCP session"))
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
