package session

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/globalsign/mgo/bson"
	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	challengeMaxTries     = 5
	challengeDigits       = 4 // 0000 - 9999
	maxChallengesRequests = 50
)

var (
	ErrChallengeTriesExceeded   = fmt.Errorf("challenge tries exceeded")
	ErrChallengeIdNotFound      = fmt.Errorf("challenge id not found")
	ErrChallengeSolutionWrong   = fmt.Errorf("challenge solution is wrong")
	ErrInvalidScope             = fmt.Errorf("invalid scope")
	ErrTooManyChallengeRequests = fmt.Errorf("too many challenge requests per session")
	currentChallengesRequests   = atomic.NewInt32(0)
)

func (s *service) StartNewChallenge(scope model.AccountAuthLocalApiScope, info *pb.EventAccountLinkChallengeClientInfo) (challengeId string, challengeValue string, err error) {
	if currentChallengesRequests.Load() >= maxChallengesRequests {
		// todo: add limits per process?
		return "", "", ErrTooManyChallengeRequests
	}
	switch scope {
	case model.AccountAuth_Limited, model.AccountAuth_JsonAPI:
		// full scope is not allowed via challenge
	default:
		return "", "", ErrInvalidScope
	}
	// generate random challenge id
	id := bson.NewObjectId().Hex()
	s.lock.Lock()
	defer s.lock.Unlock()
	// generate random challenge value
	value := fmt.Sprintf("%0*d", challengeDigits, rand.Intn(int(math.Pow10(challengeDigits))))

	s.challenges[id] = challenge{
		tries:      0,
		value:      value,
		clientInfo: info,
		scope:      scope,
	}

	currentChallengesRequests.Inc()
	return id, value, nil
}

func (s *service) SolveChallenge(challengeId string, challengeSolution string, signingKey []byte) (clientInfo *pb.EventAccountLinkChallengeClientInfo, token string, scope model.AccountAuthLocalApiScope, err error) {
	s.lock.Lock()
	challenge, ok := s.challenges[challengeId]
	if !ok {
		s.lock.Unlock()

		return nil, "", 0, ErrChallengeIdNotFound
	}
	if challenge.tries >= challengeMaxTries {
		s.lock.Unlock()

		return clientInfo, "", 0, ErrChallengeTriesExceeded
	}

	if challenge.value != challengeSolution {
		s.lock.Unlock()

		challenge.tries++
		return clientInfo, "", 0, ErrChallengeSolutionWrong
	}

	delete(s.challenges, challengeId)
	s.lock.Unlock()

	sessionToken, err := s.StartSession(signingKey, scope)
	if err != nil {
		return nil, "", 0, err
	}
	return challenge.clientInfo, sessionToken, challenge.scope, nil
}

type challenge struct {
	tries      int
	value      string
	clientInfo *pb.EventAccountLinkChallengeClientInfo
	scope      model.AccountAuthLocalApiScope
}
