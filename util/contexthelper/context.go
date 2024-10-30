package contexthelper

import "context"

// ContextWithCloseChan returns a context that is canceled when either the parent context
// is canceled or when the provided close channel is closed.
func ContextWithCloseChan(ctx context.Context, closeChan <-chan struct{}) (context.Context, context.CancelFunc) {
	// Create a new context that can be canceled
	newCtx, cancel := context.WithCancel(ctx)

	// Start a goroutine that waits for either the closeChan to be closed or
	// the new context to be canceled
	go func() {
		select {
		case <-closeChan:
			cancel()
		case <-newCtx.Done():
			// newCtx is canceled, goroutine exits
		}
	}()

	// Return the cancel function
	return newCtx, cancel
}
