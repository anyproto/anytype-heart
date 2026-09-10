package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/application/accountdirlock"
	"github.com/anyproto/anytype-heart/util/vcs"
)

func (s *Service) validateAccountID(accountID string) error {
	if err := accountdirlock.ValidateAccountID(accountID); err != nil {
		return errors.Join(ErrBadInput, err)
	}
	if s.derivedKeys != nil {
		derivedAccountID := s.derivedKeys.Identity.GetPublic().Account()
		if accountID != derivedAccountID {
			return errors.Join(ErrBadInput, fmt.Errorf("requested account %s does not match derived wallet account %s", accountID, derivedAccountID))
		}
	}
	return nil
}

func (s *Service) acquireAccountLease(ctx context.Context, rootPath, accountID string) error {
	if s.accountLease != nil {
		if s.accountLease.Matches(rootPath, accountID) {
			if !s.accountLease.Usable() {
				return errors.Join(accountdirlock.ErrUnavailable, errors.New("account lease has an ambiguous unlock failure"))
			}
			return nil
		}
		return errors.New("account lease invariant violated: another account lease is already held")
	}
	lease, err := newAccountLease(ctx, rootPath, accountID)
	if err != nil {
		return mapAccountLeaseError(err)
	}
	s.accountLease = lease
	return nil
}

// switchAccountLease acquires the target before closing the current account.
// A contended target therefore leaves the current account running.
func (s *Service) switchAccountLease(ctx context.Context, rootPath, accountID string) error {
	if s.accountLease != nil && s.accountLease.Matches(rootPath, accountID) {
		if !s.accountLease.Usable() {
			return errors.Join(accountdirlock.ErrUnavailable, errors.New("account lease has an ambiguous unlock failure"))
		}
		if err := s.closeApp(); err != nil {
			return errors.Join(ErrFailedToStopApplication, err, s.releaseAccountLease())
		}
		return nil
	}
	targetLease, err := newAccountLease(ctx, rootPath, accountID)
	if err != nil {
		return mapAccountLeaseError(err)
	}
	if err = s.stop(); err != nil {
		return errors.Join(ErrFailedToStopApplication, err, targetLease.Release())
	}
	s.accountLease = targetLease
	return nil
}

func newAccountLease(ctx context.Context, rootPath, accountID string) (*accountdirlock.Lease, error) {
	return accountdirlock.Acquire(ctx, rootPath, accountID, vcs.GetVCSInfo().Version())
}

func mapAccountLeaseError(err error) error {
	if errors.Is(err, accountdirlock.ErrLocked) {
		return errors.Join(ErrAnotherProcessIsRunning, err)
	}
	return err
}

func (s *Service) releaseAccountLease() error {
	if s.accountLease == nil {
		return nil
	}
	lease := s.accountLease
	if err := lease.Release(); err != nil {
		return err
	}
	s.accountLease = nil
	return nil
}
