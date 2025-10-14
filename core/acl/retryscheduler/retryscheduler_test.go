package retryscheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockTimer struct {
	c        chan time.Time
	id       int64
	provider *mockTimeProvider
}

func (t *mockTimer) C() <-chan time.Time {
	return t.c
}

func (t *mockTimer) Stop() bool {
	return t.provider.stopTimer(t.id)
}

func (t *mockTimer) Reset(d time.Duration) bool {
	return t.provider.resetTimer(t.id, d)
}

type timerInfo struct {
	timer      *mockTimer
	expiryTime time.Time
	stopped    bool
}

type mockTimeProvider struct {
	currentTime time.Time
	timers      map[int64]*timerInfo
	nextID      int64
	mu          sync.Mutex
}

func newMockTimeProvider() *mockTimeProvider {
	return &mockTimeProvider{
		currentTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		timers:      make(map[int64]*timerInfo),
	}
}

func (m *mockTimeProvider) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTime
}

func (m *mockTimeProvider) NewTimer(d time.Duration) Timer {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	timer := &mockTimer{
		c:        make(chan time.Time, 1),
		id:       id,
		provider: m,
	}

	expiryTime := m.currentTime.Add(d)
	m.timers[id] = &timerInfo{
		timer:      timer,
		expiryTime: expiryTime,
		stopped:    false,
	}

	if d <= 0 {
		timer.c <- m.currentTime
	}

	return timer
}

func (m *mockTimeProvider) stopTimer(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.timers[id]
	if !ok || info.stopped {
		return false
	}

	info.stopped = true
	return true
}

func (m *mockTimeProvider) resetTimer(id int64, d time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.timers[id]
	if !ok {
		return false
	}

	wasActive := !info.stopped
	info.expiryTime = m.currentTime.Add(d)
	info.stopped = false

	select {
	case <-info.timer.c:
	default:
	}

	if d <= 0 {
		select {
		case info.timer.c <- m.currentTime:
		default:
		}
	}

	return wasActive
}

func (m *mockTimeProvider) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentTime = m.currentTime.Add(d)

	for _, info := range m.timers {
		if !info.stopped && info.expiryTime.Before(m.currentTime.Add(time.Nanosecond)) {
			select {
			case info.timer.c <- m.currentTime:
				info.stopped = true
			default:
			}
		}
	}
}

func waitForProcessing() {
	time.Sleep(10 * time.Millisecond)
}

type testMessage struct {
	ID      string
	Content string
}

func TestRetryScheduler_BasicOperation(t *testing.T) {
	mockTime := newMockTimeProvider()
	processedItems := make(chan testMessage, 10)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		processedItems <- msg
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     1 * time.Second,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	queue.Schedule("1", testMessage{ID: "1", Content: "first"}, 200*time.Millisecond)
	queue.Schedule("2", testMessage{ID: "2", Content: "second"}, 100*time.Millisecond)
	queue.Schedule("3", testMessage{ID: "3", Content: "third"}, 300*time.Millisecond)

	mockTime.Advance(100 * time.Millisecond)
	msg := <-processedItems
	if msg.ID != "2" {
		t.Errorf("Expected item 2 first, got %s", msg.ID)
	}

	mockTime.Advance(100 * time.Millisecond)
	msg = <-processedItems
	if msg.ID != "1" {
		t.Errorf("Expected item 1 second, got %s", msg.ID)
	}

	mockTime.Advance(100 * time.Millisecond)
	msg = <-processedItems
	if msg.ID != "3" {
		t.Errorf("Expected item 3 third, got %s", msg.ID)
	}

	if queue.Len() != 0 {
		t.Errorf("Expected queue to be empty, got %d items", queue.Len())
	}
}

func TestRetryScheduler_UpdateExisting(t *testing.T) {
	mockTime := newMockTimeProvider()
	processedItems := make(chan testMessage, 10)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		processedItems <- msg
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     1 * time.Second,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	queue.Schedule("1", testMessage{ID: "1", Content: "first"}, 500*time.Millisecond)

	mockTime.Advance(100 * time.Millisecond)
	waitForProcessing()

	queue.Schedule("1", testMessage{ID: "1", Content: "updated"}, 50*time.Millisecond)

	mockTime.Advance(50 * time.Millisecond)

	select {
	case msg := <-processedItems:
		if msg.Content != "updated" {
			t.Errorf("Expected updated content, got %s", msg.Content)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for processed item")
	}
}

func TestRetryScheduler_RemoveUpdate(t *testing.T) {
	mockTime := newMockTimeProvider()
	processedItems := make(chan testMessage, 10)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		processedItems <- msg
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     1 * time.Second,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	queue.Schedule("1", testMessage{ID: "1", Content: "first"}, 100*time.Millisecond)
	queue.Schedule("2", testMessage{ID: "2", Content: "second"}, 200*time.Millisecond)

	queue.Remove("1")

	mockTime.Advance(100 * time.Millisecond)

	select {
	case <-processedItems:
		t.Error("Should not have processed any items yet")
	case <-time.After(20 * time.Millisecond):
	}

	mockTime.Advance(100 * time.Millisecond)

	select {
	case msg := <-processedItems:
		if msg.ID != "2" {
			t.Errorf("Expected item 2, got %s", msg.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for item 2")
	}
}

func TestRetryScheduler_RetryWithBackoff(t *testing.T) {
	mockTime := newMockTimeProvider()
	var attemptCount int32
	attemptTimes := make([]time.Time, 0)
	var mu sync.Mutex
	retryError := errors.New("retry me")

	updateFunc := func(ctx context.Context, msg testMessage) error {
		count := atomic.AddInt32(&attemptCount, 1)
		mu.Lock()
		attemptTimes = append(attemptTimes, mockTime.Now())
		mu.Unlock()

		if count < 3 {
			return retryError
		}
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return errors.Is(err, retryError)
	}

	config := Config{
		DefaultTimeout: 50 * time.Millisecond,
		MaxTimeout:     200 * time.Millisecond,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	initialTimeout := 40 * time.Millisecond
	queue.Schedule("1", testMessage{ID: "1", Content: "retry"}, initialTimeout)

	mockTime.Advance(initialTimeout)
	waitForProcessing()

	mockTime.Advance(60 * time.Millisecond)
	waitForProcessing()

	mockTime.Advance(90 * time.Millisecond)
	waitForProcessing()

	if count := atomic.LoadInt32(&attemptCount); count != 3 {
		t.Errorf("Expected 3 attempts, got %d", count)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(attemptTimes) != 3 {
		t.Fatalf("Expected 3 attempt times, got %d", len(attemptTimes))
	}

	firstInterval := attemptTimes[1].Sub(attemptTimes[0])
	if firstInterval < 55*time.Millisecond || firstInterval > 65*time.Millisecond {
		t.Errorf("First retry interval wrong: expected ~60ms, got %v", firstInterval)
	}

	secondInterval := attemptTimes[2].Sub(attemptTimes[1])
	if secondInterval < 85*time.Millisecond || secondInterval > 95*time.Millisecond {
		t.Errorf("Second retry interval wrong: expected ~90ms, got %v", secondInterval)
	}
}

func TestRetryScheduler_MaxTimeout(t *testing.T) {
	mockTime := newMockTimeProvider()
	attemptTimes := make([]time.Time, 0)
	var mu sync.Mutex

	updateFunc := func(ctx context.Context, msg testMessage) error {
		mu.Lock()
		attemptTimes = append(attemptTimes, mockTime.Now())
		mu.Unlock()
		return errors.New("always fail")
	}

	evaluate := func(msg testMessage, err error) bool {
		mu.Lock()
		attempts := len(attemptTimes)
		mu.Unlock()
		return attempts < 3
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     150 * time.Millisecond,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	queue.Schedule("1", testMessage{ID: "1", Content: "test"}, 120*time.Millisecond)

	mockTime.Advance(120 * time.Millisecond)
	waitForProcessing()

	mockTime.Advance(150 * time.Millisecond)
	waitForProcessing()

	mockTime.Advance(150 * time.Millisecond)
	waitForProcessing()

	mu.Lock()
	defer mu.Unlock()

	if len(attemptTimes) != 3 {
		t.Fatalf("Expected 3 attempts, got %d", len(attemptTimes))
	}

	gap1 := attemptTimes[1].Sub(attemptTimes[0])
	if gap1 < 145*time.Millisecond || gap1 > 155*time.Millisecond {
		t.Errorf("First retry gap should be capped at ~150ms, got %v", gap1)
	}

	gap2 := attemptTimes[2].Sub(attemptTimes[1])
	if gap2 < 145*time.Millisecond || gap2 > 155*time.Millisecond {
		t.Errorf("Second retry gap should be capped at ~150ms, got %v", gap2)
	}
}

func TestRetryScheduler_ZeroTimeout(t *testing.T) {
	mockTime := newMockTimeProvider()
	processed := make(chan testMessage, 1)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		processed <- msg
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     1 * time.Second,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	msg := testMessage{ID: "1", Content: "immediate"}
	queue.Schedule("1", msg, 0)

	select {
	case receivedMsg := <-processed:
		if receivedMsg.ID != msg.ID {
			t.Errorf("Received wrong message: %v", receivedMsg)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("Item with zero timeout was not processed immediately")
	}
}

func TestRetryScheduler_ConcurrentOperations(t *testing.T) {
	mockTime := newMockTimeProvider()
	processedCount := int32(0)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		atomic.AddInt32(&processedCount, 1)
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 10 * time.Millisecond,
		MaxTimeout:     100 * time.Millisecond,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			queue.Schedule(
				string(rune('0'+id)),
				testMessage{ID: string(rune('0' + id)), Content: "test"},
				time.Duration(id*10)*time.Millisecond,
			)
		}(i)
	}

	for i := 5; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(5 * time.Millisecond)
			queue.Remove(string(rune('0' + id)))
		}(i)
	}

	wg.Wait()

	for i := 0; i < 10; i++ {
		mockTime.Advance(10 * time.Millisecond)
		waitForProcessing()
	}

	count := atomic.LoadInt32(&processedCount)
	if count != 7 {
		t.Errorf("Expected 7 items to be processed, got %d", count)
	}
}

func TestRetryScheduler_CloseWhileProcessing(t *testing.T) {
	mockTime := newMockTimeProvider()
	blockCh := make(chan struct{})
	started := make(chan struct{})

	updateFunc := func(ctx context.Context, msg testMessage) error {
		close(started)
		<-blockCh
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 10 * time.Millisecond,
		MaxTimeout:     100 * time.Millisecond,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()

	queue.Schedule("1", testMessage{ID: "1", Content: "test"}, 0)

	<-started

	closeDone := make(chan struct{})
	go func() {
		queue.Close()
		close(closeDone)
	}()

	close(blockCh)

	select {
	case <-closeDone:
	case <-time.After(100 * time.Millisecond):
		t.Error("Close did not complete in time")
	}

	err := queue.Schedule("2", testMessage{ID: "2", Content: "test"}, 0)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestRetryScheduler_ImmediateProcessing(t *testing.T) {
	mockTime := newMockTimeProvider()
	processed := make(chan string, 10)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		processed <- msg.ID
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     1 * time.Second,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	for i := 0; i < 5; i++ {
		queue.Schedule(string(rune('0'+i)), testMessage{ID: string(rune('0' + i))}, 0)
	}

	for i := 0; i < 5; i++ {
		select {
		case <-processed:
		case <-time.After(50 * time.Millisecond):
			t.Errorf("Item %d not processed immediately", i)
		}
	}
}

func TestRetryScheduler_TimerReuse(t *testing.T) {
	mockTime := newMockTimeProvider()
	processedItems := make(chan testMessage, 10)

	updateFunc := func(ctx context.Context, msg testMessage) error {
		processedItems <- msg
		return nil
	}

	evaluate := func(msg testMessage, err error) bool {
		return false
	}

	config := Config{
		DefaultTimeout: 100 * time.Millisecond,
		MaxTimeout:     1 * time.Second,
		TimeProvider:   mockTime,
	}

	queue := NewRetryScheduler(updateFunc, evaluate, config)
	queue.Run()
	defer queue.Close()

	for i := 0; i < 3; i++ {
		queue.Schedule(string(rune('0'+i)), testMessage{ID: string(rune('0' + i))}, 50*time.Millisecond)
		mockTime.Advance(50 * time.Millisecond)

		select {
		case msg := <-processedItems:
			if msg.ID != string(rune('0'+i)) {
				t.Errorf("Expected item %d, got %s", i, msg.ID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Timeout waiting for item %d", i)
		}
	}
}
