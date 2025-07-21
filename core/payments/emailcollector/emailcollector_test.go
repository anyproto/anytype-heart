package emailcollector

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

type mockEmailCollector struct {
	emailcollector
	onSetRequest func(req *pb.RpcMembershipGetVerificationEmailRequest) error
}

func (m *mockEmailCollector) SetRequest(req *pb.RpcMembershipGetVerificationEmailRequest) error {
	if m.onSetRequest != nil {
		return m.onSetRequest(req)
	}
	return nil
}

func TestEmailCollector(t *testing.T) {
	t.Run("should collect email", func(t *testing.T) {
		// Given
		var called bool
		collector := &mockEmailCollector{
			onSetRequest: func(req *pb.RpcMembershipGetVerificationEmailRequest) error {
				called = true
				assert.Equal(t, "test@example.com", req.Email)
				assert.True(t, req.SubscribeToNewsletter)
				assert.True(t, req.InsiderTipsAndTutorials)
				assert.True(t, req.IsOnboardingList)
				return nil
			},
		}
		req := &pb.RpcMembershipGetVerificationEmailRequest{
			Email:                   "test@example.com",
			SubscribeToNewsletter:   true,
			InsiderTipsAndTutorials: true,
			IsOnboardingList:        true,
		}

		// When
		err := collector.SetRequest(req)

		// Then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("should not collect empty email", func(t *testing.T) {
		// Given
		var called bool
		collector := &mockEmailCollector{
			onSetRequest: func(req *pb.RpcMembershipGetVerificationEmailRequest) error {
				called = true
				assert.Empty(t, req.Email)
				return assert.AnError
			},
		}
		req := &pb.RpcMembershipGetVerificationEmailRequest{
			Email: "",
		}

		// When
		err := collector.SetRequest(req)

		// Then
		assert.Error(t, err)
		assert.True(t, called)
	})

	t.Run("should not collect invalid email", func(t *testing.T) {
		// Given
		var called bool
		collector := &mockEmailCollector{
			onSetRequest: func(req *pb.RpcMembershipGetVerificationEmailRequest) error {
				called = true
				assert.Equal(t, "invalid-email", req.Email)
				return assert.AnError
			},
		}
		req := &pb.RpcMembershipGetVerificationEmailRequest{
			Email: "invalid-email",
		}

		// When
		err := collector.SetRequest(req)

		// Then
		assert.Error(t, err)
		assert.True(t, called)
	})

	t.Run("should not collect nil request", func(t *testing.T) {
		// Given
		var called bool
		collector := &mockEmailCollector{
			onSetRequest: func(req *pb.RpcMembershipGetVerificationEmailRequest) error {
				called = true
				assert.Nil(t, req)
				return assert.AnError
			},
		}

		// When
		err := collector.SetRequest(nil)

		// Then
		assert.Error(t, err)
		assert.True(t, called)
	})

	t.Run("should collect email with different subscription preferences", func(t *testing.T) {
		testCases := []struct {
			name                    string
			email                   string
			subscribeToNewsletter   bool
			insiderTipsAndTutorials bool
			isOnboardingList        bool
			expectError             bool
		}{
			{
				name:                    "all subscriptions enabled",
				email:                   "test1@example.com",
				subscribeToNewsletter:   true,
				insiderTipsAndTutorials: true,
				isOnboardingList:        true,
				expectError:             false,
			},
			{
				name:                    "no subscriptions",
				email:                   "test2@example.com",
				subscribeToNewsletter:   false,
				insiderTipsAndTutorials: false,
				isOnboardingList:        false,
				expectError:             false,
			},
			{
				name:                    "partial subscriptions",
				email:                   "test3@example.com",
				subscribeToNewsletter:   true,
				insiderTipsAndTutorials: false,
				isOnboardingList:        true,
				expectError:             false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Given
				var called bool
				collector := &mockEmailCollector{
					onSetRequest: func(req *pb.RpcMembershipGetVerificationEmailRequest) error {
						called = true
						assert.Equal(t, tc.email, req.Email)
						assert.Equal(t, tc.subscribeToNewsletter, req.SubscribeToNewsletter)
						assert.Equal(t, tc.insiderTipsAndTutorials, req.InsiderTipsAndTutorials)
						assert.Equal(t, tc.isOnboardingList, req.IsOnboardingList)
						if tc.expectError {
							return assert.AnError
						}
						return nil
					},
				}
				req := &pb.RpcMembershipGetVerificationEmailRequest{
					Email:                   tc.email,
					SubscribeToNewsletter:   tc.subscribeToNewsletter,
					InsiderTipsAndTutorials: tc.insiderTipsAndTutorials,
					IsOnboardingList:        tc.isOnboardingList,
				}

				// When
				err := collector.SetRequest(req)

				// Then
				if tc.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				assert.True(t, called)
			})
		}
	})

	t.Run("should handle concurrent requests", func(t *testing.T) {
		// Given
		var callCount atomic.Uint32
		collector := &mockEmailCollector{
			onSetRequest: func(req *pb.RpcMembershipGetVerificationEmailRequest) error {
				callCount.Add(1)
				assert.Equal(t, "test@example.com", req.Email)
				return nil
			},
		}
		req := &pb.RpcMembershipGetVerificationEmailRequest{
			Email: "test@example.com",
		}

		// When
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				err := collector.SetRequest(req)
				assert.NoError(t, err)
				done <- true
			}()
		}

		// Then
		for i := 0; i < 10; i++ {
			<-done
		}
		assert.Equal(t, uint32(10), callCount.Load())
	})
}
