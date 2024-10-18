package contexthelper

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestContextWithCloseChan_CloseChanCancellation(t *testing.T) {
	parentCtx := context.Background()
	closeChan := make(chan struct{})
	ctx, cancelFunc := ContextWithCloseChan(parentCtx, closeChan)
	defer cancelFunc() // Ensure resources are released

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			// Expected to be canceled when closeChan is closed
		case <-time.After(1 * time.Second):
			t.Error("context was not canceled when closeChan was closed")
		}
	}()

	// Close the closeChan to trigger cancellation
	close(closeChan)

	wg.Wait()

	// Verify that the context was canceled
	if ctx.Err() == nil {
		t.Error("context error is nil, expected cancellation error")
	}
}

func TestContextWithCloseChan_ParentContextCancellation(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	closeChan := make(chan struct{})
	ctx, cancelFunc := ContextWithCloseChan(parentCtx, closeChan)
	defer cancelFunc() // Ensure resources are released

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			// Expected to be canceled when parentCtx is canceled
		case <-time.After(1 * time.Second):
			t.Error("context was not canceled when parent context was canceled")
		}
	}()

	// Cancel the parent context
	parentCancel()

	wg.Wait()

	// Verify that the context was canceled
	if ctx.Err() == nil {
		t.Error("context error is nil, expected cancellation error")
	}
}

func TestContextWithCloseChan_NoCancellation(t *testing.T) {
	parentCtx := context.Background()
	closeChan := make(chan struct{})
	ctx, cancelFunc := ContextWithCloseChan(parentCtx, closeChan)
	defer cancelFunc() // Ensure resources are released

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			t.Error("context was canceled unexpectedly")
		case <-time.After(50 * time.Millisecond):
			// Expected to timeout here as neither context nor closeChan is canceled
		}
	}()

	wg.Wait()

	// Verify that the context is still active
	if ctx.Err() != nil {
		t.Errorf("context error is %v, expected nil", ctx.Err())
	}
}

func TestContextWithCloseChan_BothCancellation(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	closeChan := make(chan struct{})
	ctx, cancelFunc := ContextWithCloseChan(parentCtx, closeChan)
	defer cancelFunc() // Ensure resources are released

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			// Expected to be canceled
		case <-time.After(1 * time.Second):
			t.Error("context was not canceled when both parent context and closeChan were canceled")
		}
	}()

	// Cancel both parent context and closeChan
	parentCancel()
	close(closeChan)

	wg.Wait()

	// Verify that the context was canceled
	if ctx.Err() == nil {
		t.Error("context error is nil, expected cancellation error")
	}
}
