package application

import (
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// CreateSessionResult carries the minted session token plus the attributes of
// the authenticated principal. Scope, app name and expiry let the HTTP gate
// (core/api/server) enforce key scope and expiration without re-reading the
// app link on every request.
type CreateSessionResult struct {
	Token        string
	AccountId    string
	AccountScope model.AccountAuthLocalApiScope
	// AppName, AppExpireAt, Grant, AppHash and AppCreatedAt are set only for
	// appKey auth; AppExpireAt of 0 means the key never expires, a nil Grant
	// an unscoped key. AppHash is the app link's identity — the same hash
	// ListApps shows and RevokeApp takes — so whoami can name the credential
	// without a second wallet read.
	AppName      string
	AppExpireAt  int64
	Grant        *model.AccountAuthAppGrant
	AppHash      string
	AppCreatedAt int64
}

func (s *Service) CreateSession(req *pb.RpcWalletCreateSessionRequest) (*CreateSessionResult, error) {
	// test if mnemonic is correct
	mnemonic := req.GetMnemonic()
	appKey := req.GetAppKey()
	providedToken := req.GetToken()
	accountKey := req.GetAccountKey()

	if appKey != "" {
		app := s.GetApp()
		if app == nil {
			return nil, ErrApplicationIsNotRunning
		}
		wallet := app.Component(walletComp.CName)
		if wallet == nil {
			return nil, fmt.Errorf("appToken auth not yet supported for the main app")
		}
		w := wallet.(walletComp.Wallet)
		appLink, token, err := s.mintAppKeySession(w, appKey)
		if err != nil {
			return nil, err
		}
		log.Infof("appLink auth %s", appLink.AppName)

		return &CreateSessionResult{
			Token:        token,
			AccountId:    w.Account().SignKey.GetPublic().Account(),
			AccountScope: model.AccountAuthLocalApiScope(appLink.Scope), // nolint:gosec
			AppName:      appLink.AppName,
			AppExpireAt:  appLink.ExpireAt,
			Grant:        appLink.Grant.Proto(),
			AppHash:      appLink.AppHash,
			AppCreatedAt: appLink.CreatedAt,
		}, nil
	}

	if providedToken != "" {
		token, scope, err := s.deriveSession(providedToken)
		if err != nil {
			return nil, err
		}
		// NOTE: this result carries no app attributes — AppName, AppExpireAt
		// and Grant stay zero even when the source token descends from an app
		// link (only the app HASH is tracked, and reading name/expiry/grant
		// back would need the app key, which this branch does not have). The
		// result of the derive branch must therefore never be used for
		// app-level scope/expiry/grant decisions; the HTTP gate only
		// authenticates with AuthOfAppKey, which goes through the branch
		// above.
		return &CreateSessionResult{Token: token, AccountScope: scope}, nil
	}

	var derived crypto.DerivationResult
	var err error

	if accountKey != "" {
		derived, err = core.WalletDeriveFromAccountMasterNode(accountKey)
		if err != nil {
			return nil, errors.Join(ErrBadInput, fmt.Errorf("invalid account key: %w", err))
		}
	} else {
		if s.derivedKeys == nil {
			return nil, ErrWalletNotInitialized
		}

		// Derive keys from provided mnemonic to verify it's correct
		derived, err = core.WalletAccountAt(mnemonic, 0)
		if err != nil {
			return nil, errors.Join(ErrBadInput, fmt.Errorf("invalid mnemonic"))
		}
	}

	// Compare account IDs to verify we are at the same account
	if derived.Identity.GetPublic().Account() != s.derivedKeys.Identity.GetPublic().Account() {
		return nil, errors.Join(ErrBadInput, fmt.Errorf("incorrect mnemonic"))
	}
	token, err := s.sessions.StartSession(s.sessionSigningKey, model.AccountAuth_Full)
	if err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}
	// todo: account is empty, to be implemented with GO-1854
	return &CreateSessionResult{Token: token, AccountScope: model.AccountAuth_Full}, nil
}

// mintAppKeySession reads the app link and mints a session tracked against
// its hash. Read, mint and track form one critical section with the
// LinkLocalRevokeApp sweep: without it, a mint racing a revoke could track its
// session after the sweep ran, leaving a live session no revoke can reach
// (H4). The app-link file is deleted before the sweep takes the lock, so a
// mint that runs after the sweep fails at ReadAppLink.
func (s *Service) mintAppKeySession(w walletComp.Wallet, appKey string) (*walletComp.AppLinkInfo, string, error) {
	s.appSessionsLock.Lock()
	defer s.appSessionsLock.Unlock()
	appLink, err := w.ReadAppLink(appKey)
	if err != nil {
		return nil, "", fmt.Errorf("read app link: %w", err)
	}
	token, err := s.sessions.StartSession(s.sessionSigningKey, model.AccountAuthLocalApiScope(appLink.Scope)) // nolint:gosec
	if err != nil {
		return nil, "", fmt.Errorf("start session: %w", err)
	}
	s.trackAppSessionLocked(appLink.AppHash, token)
	return appLink, token, nil
}

// deriveSession mints a fresh session from an existing token, inheriting its
// scope and its app-hash association so revoking the app closes derived
// sessions too (H4). Validate and track are one critical section with the
// revoke sweep: either the derive completes first and the new token is swept
// with the rest, or the sweep closed the source token first and ValidateToken
// fails — a revoked token can never be laundered into an untracked session.
func (s *Service) deriveSession(providedToken string) (string, model.AccountAuthLocalApiScope, error) {
	s.appSessionsLock.Lock()
	defer s.appSessionsLock.Unlock()
	scope, err := s.sessions.ValidateToken(s.sessionSigningKey, providedToken)
	if err != nil {
		return "", 0, fmt.Errorf("validate provided token: %w", err)
	}
	token, err := s.sessions.StartSession(s.sessionSigningKey, scope)
	if err != nil {
		return "", 0, fmt.Errorf("start session: %w", err)
	}
	if appHash, ok := s.appHashByToken[providedToken]; ok {
		s.trackAppSessionLocked(appHash, token)
	}
	return token, scope, nil
}

// trackAppSessionLocked records the token as belonging to the app link in
// both directions, so revocation can find every live session of a key (H4).
// The caller must hold s.appSessionsLock.
func (s *Service) trackAppSessionLocked(appHash string, token string) {
	set, ok := s.sessionsByAppHash[appHash]
	if !ok {
		set = make(map[string]struct{})
		s.sessionsByAppHash[appHash] = set
	}
	set[token] = struct{}{}
	s.appHashByToken[token] = appHash
}

// untrackSessionLocked removes the token from both indexes; empty token sets
// are dropped so ListApps' isActive stays a plain presence check. The caller
// must hold s.appSessionsLock.
func (s *Service) untrackSessionLocked(token string) {
	appHash, ok := s.appHashByToken[token]
	if !ok {
		return
	}
	delete(s.appHashByToken, token)
	if set, ok := s.sessionsByAppHash[appHash]; ok {
		delete(set, token)
		if len(set) == 0 {
			delete(s.sessionsByAppHash, appHash)
		}
	}
}

func (s *Service) CloseSession(req *pb.RpcWalletCloseSessionRequest) error {
	if sender, ok := s.eventSender.(session.Closer); ok {
		sender.CloseSession(req.Token)
	}
	// Untrack and close atomically with respect to the derive branch, so a
	// token mid-close cannot be re-minted into an untracked session.
	s.appSessionsLock.Lock()
	s.untrackSessionLocked(req.Token)
	err := s.sessions.CloseSession(req.Token)
	s.appSessionsLock.Unlock()
	if err != nil {
		return fmt.Errorf("close session: %w", err)
	}
	return nil
}

func (s *Service) ValidateSessionToken(token string) (model.AccountAuthLocalApiScope, error) {
	return s.sessions.ValidateToken(s.sessionSigningKey, token)
}

func (s *Service) LinkLocalStartNewChallenge(scope model.AccountAuthLocalApiScope, clientInfo *pb.EventAccountLinkChallengeClientInfo, requestedGrant *model.AccountAuthAppGrant) (id string, err error) {
	if s.app == nil {
		return "", ErrApplicationIsNotRunning
	}
	// Validate the requested grant before the challenge exists: an invalid one
	// would otherwise surface only at persist time, after the user already
	// typed the code. The same validation runs again inside the wallet when
	// the solved challenge persists.
	if err = walletComp.ValidateAppLinkGrant(walletComp.AppLinkGrantFromProto(requestedGrant), scope); err != nil {
		return "", fmt.Errorf("validate requested grant: %w", err)
	}

	id, value, err := s.sessions.StartNewChallenge(scope, clientInfo, requestedGrant)
	if err != nil {
		return "", fmt.Errorf("start new challenge: %w", err)
	}
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountLinkChallenge{
		AccountLinkChallenge: &pb.EventAccountLinkChallenge{
			Challenge:      value,
			ClientInfo:     clientInfo,
			Scope:          scope,
			RequestedGrant: requestedGrant,
		},
	}))
	return id, nil
}

func (s *Service) LinkLocalSolveChallenge(req *pb.RpcAccountLocalLinkSolveChallengeRequest) (token string, appKey string, err error) {
	if s.app == nil {
		return "", "", ErrApplicationIsNotRunning
	}
	clientInfo, token, scope, requestedGrant, err := s.sessions.SolveChallenge(req.ChallengeId, req.Answer, s.sessionSigningKey)
	if err != nil {
		return "", "", fmt.Errorf("solve challenge: %w", err)
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	name := clientInfo.Name
	if name == "" {
		name = clientInfo.ProcessName
	}
	// The challenge path never sets an expiry (expireAt=0): a default lifetime
	// is a product decision deferred until the consent picker exists. The
	// requested grant is persisted as-is: grants only narrow, so a
	// self-requested restriction is fail-safe by monotonicity — this is how
	// CLI users get scoped keys before any consent picker exists.
	appInfo, err := wallet.PersistAppLink(name, scope, 0, walletComp.AppLinkGrantFromProto(requestedGrant))
	if err != nil {
		return token, appKey, fmt.Errorf("persist app link: %w", err)
	}

	s.appSessionsLock.Lock()
	s.trackAppSessionLocked(appInfo.AppHash, token)
	s.appSessionsLock.Unlock()
	appKey = appInfo.AppKey
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountLinkChallengeHide{
		AccountLinkChallengeHide: &pb.EventAccountLinkChallengeHide{
			Challenge: req.Answer,
		},
	}))
	return
}

func (s *Service) LinkLocalCreateApp(req *pb.RpcAccountLocalLinkCreateAppRequest) (appKey string, err error) {
	if s.app == nil {
		return "", ErrApplicationIsNotRunning
	}
	if req.App == nil {
		return "", errors.Join(ErrBadInput, errors.New("app info is required"))
	}
	// Mirror the challenge-path guard (session.StartNewChallenge): a Full
	// scope app key must never be mintable — Full stays reserved for sessions
	// authenticated with the mnemonic/account key (H3: CreateApp must not
	// mint an escalated key).
	switch req.App.Scope {
	case model.AccountAuth_Limited, model.AccountAuth_JsonAPI:
	default:
		return "", session.ErrInvalidScope
	}
	// ExpireAt is load-bearing (H5): a non-zero value must be a future unix
	// timestamp in seconds. Rejecting past (and negative) values here makes a
	// duration-vs-timestamp mix-up fail loudly at issuance instead of minting
	// a key that is dead on arrival — or, for negative values, immortal.
	if req.App.ExpireAt != 0 && req.App.ExpireAt <= time.Now().Unix() {
		return "", errors.Join(ErrBadInput, errors.New("expireAt must be 0 (never expires) or a future unix timestamp in seconds"))
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	// Grant validation (non-empty spaces, known perms, JsonAPI scope only)
	// lives in the wallet — the single persist point.
	appInfo, err := wallet.PersistAppLink(req.App.AppName, req.App.Scope, req.App.ExpireAt, walletComp.AppLinkGrantFromProto(req.App.Grant))
	if err != nil {
		return "", fmt.Errorf("persist app link: %w", err)
	}
	return appInfo.AppKey, nil
}

// LinkLocalUpdateApp replaces the grant of an existing app link in place —
// the key string never changes, so the holder keeps working (or, if the app
// keys sessions off it, starts being scoped) without redistributing a secret.
// Full-scope only like its siblings (enforced by the gRPC interceptor's
// allowlist). Heart applies whatever the Full caller sends, a nil grant
// included (clears the scoping): widen-requires-re-consent is the desktop
// UI's contract — heart cannot render consent.
func (s *Service) LinkLocalUpdateApp(req *pb.RpcAccountLocalLinkUpdateAppRequest) error {
	if s.app == nil {
		return ErrApplicationIsNotRunning
	}
	if req.AppHash == "" {
		return errors.Join(ErrBadInput, errors.New("app hash is required"))
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	if err := wallet.UpdateAppLinkGrant(req.AppHash, walletComp.AppLinkGrantFromProto(req.Grant)); err != nil {
		return fmt.Errorf("update app link grant: %w", err)
	}

	// The HTTP layer caches key→session entries at mint time (see
	// ApiSessionEntry: a surface that edits a live key's grant must evict that
	// key's entries, or the edit takes effect only after a restart). Sessions
	// themselves stay alive — the key was edited, not revoked — so only the
	// cache is dropped and the next request re-mints with the new grant.
	apiService, hasApiService := s.app.Component(api.CName).(api.Service)
	if !hasApiService {
		return nil
	}
	s.appSessionsLock.Lock()
	tokens := make([]string, 0, len(s.sessionsByAppHash[req.AppHash]))
	for token := range s.sessionsByAppHash[req.AppHash] {
		tokens = append(tokens, token)
	}
	s.appSessionsLock.Unlock()
	for _, token := range tokens {
		apiService.RevokeToken(token)
	}
	return nil
}

func (s *Service) LinkLocalListApps() ([]*model.AccountAuthAppInfo, error) {
	if s.app == nil {
		return nil, ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	links, err := wallet.ListAppLinks()
	if err != nil {
		return nil, fmt.Errorf("list app links: %w", err)
	}
	appsList := make([]*model.AccountAuthAppInfo, len(links))
	s.appSessionsLock.Lock()
	defer s.appSessionsLock.Unlock()

	for i, app := range links {
		if app.AppName == "" {
			app.AppName = app.AppHash
		}
		_, isActive := s.sessionsByAppHash[app.AppHash]
		appsList[i] = &model.AccountAuthAppInfo{
			AppHash:   app.AppHash,
			AppName:   app.AppName,
			AppKey:    app.AppKey,
			CreatedAt: app.CreatedAt,
			ExpireAt:  app.ExpireAt,
			Scope:     model.AccountAuthLocalApiScope(app.Scope), // nolint:gosec
			IsActive:  isActive,
			Grant:     app.Grant.Proto(),
		}
	}
	return appsList, nil
}

func (s *Service) LinkLocalRevokeApp(req *pb.RpcAccountLocalLinkRevokeAppRequest) error {
	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	// The file is deleted BEFORE the sweep below takes appSessionsLock, so any
	// app-key mint that runs after the sweep fails at ReadAppLink — see
	// mintAppKeySession for the other half of that invariant.
	err := wallet.RevokeAppLink(req.AppHash)
	if err != nil {
		return fmt.Errorf("revoke app link: %w", err)
	}

	// Sweep and close EVERY session minted from this key — the app-key mints
	// and the ones derived via the token auth branch (H4). Closing happens
	// inside the same critical section as the sweep: if it did not, a
	// concurrent WalletCreateSession(token:) landing between the sweep and the
	// close could still validate a doomed token and mint an untracked session
	// that no future revoke can find.
	apiService, hasApiService := s.app.Component(api.CName).(api.Service)
	s.appSessionsLock.Lock()
	tokens := make([]string, 0, len(s.sessionsByAppHash[req.AppHash]))
	for token := range s.sessionsByAppHash[req.AppHash] {
		tokens = append(tokens, token)
		delete(s.appHashByToken, token)
		if closeErr := s.sessions.CloseSession(token); closeErr != nil {
			log.Errorf("close session on revoke: %v", closeErr)
		}
	}
	delete(s.sessionsByAppHash, req.AppHash)
	s.appSessionsLock.Unlock()

	// The HTTP cache eviction stays outside the lock (the API server locks
	// internally); the sessions are already dead, so a request racing this
	// loop can at worst hit a cached entry that is evicted moments later.
	if hasApiService {
		for _, token := range tokens {
			apiService.RevokeToken(token)
		}
	}

	return nil
}
