package app

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// values of this vars will be defined while compilation
	version string
	name    string
)

// Component is a minimal interface for a common app.Component
type Component interface {
	// Init will be called first
	// When returned error is not nil - app start will be aborted
	Init(a *App) (err error)
	// Name must return unique service name
	Name() (name string)
}

// ComponentRunnable is an interface for realizing ability to start background processes or deep configure service
type ComponentRunnable interface {
	Component
	// Run will be called after init stage
	// Non-nil error also will be aborted app start
	Run() (err error)
	// Close will be called when app shutting down
	// Also will be called when service return error on Init or Run stage
	// Non-nil error will be printed to log
	Close() (err error)
}

// App is the central part of the application
// It contains and manages all components
type App struct {
	components []Component
	mu         sync.RWMutex
}

// Name returns app name
func (app *App) Name() string {
	return name
}

// Version return app version
func (app *App) Version() string {
	return version
}

// Register adds service to registry
// All components will be started in the order they were registered
func (app *App) Register(s Component) *App {
	app.mu.Lock()
	defer app.mu.Unlock()
	for _, es := range app.components {
		if s.Name() == es.Name() {
			panic(fmt.Errorf("component '%s' already registered", s.Name()))
		}
	}
	app.components = append(app.components, s)
	return app
}

// Component returns service by name
// If service with given name wasn't registered, nil will be returned
func (app *App) Component(name string) Component {
	app.mu.RLock()
	defer app.mu.RUnlock()
	for _, s := range app.components {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

// MustComponent is like Component, but it will panic if service wasn't found
func (app *App) MustComponent(name string) Component {
	s := app.Component(name)
	if s == nil {
		panic(fmt.Errorf("component '%s' not registered", name))
	}
	return s
}

// ComponentNames returns all registered names
func (app *App) ComponentNames() (names []string) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	names = make([]string, len(app.components))
	for i, c := range app.components {
		names[i] = c.Name()
	}
	return
}

// Start starts the application
// All registered services will be initialized and started
func (app *App) Start() (err error) {
	app.mu.RLock()
	defer app.mu.RUnlock()

	closeServices := func(idx int) {
		for i := idx; i >= 0; i-- {
			if serviceClose, ok := app.components[i].(ComponentRunnable); ok {
				if e := serviceClose.Close(); e != nil {
					logrus.Warnf("Component '%s' close error: %v", serviceClose.Name(), e)
				}
			}
		}
	}

	for i, s := range app.components {
		if err = s.Init(app); err != nil {
			closeServices(i)
			return fmt.Errorf("can't init service '%s': %v", s.Name(), err)
		}
	}

	for i, s := range app.components {
		if serviceRun, ok := s.(ComponentRunnable); ok {
			if err = serviceRun.Run(); err != nil {
				closeServices(i)
				return fmt.Errorf("can't run service '%s': %v", serviceRun.Name(), err)
			}
		}
	}

	return
}

// Close stops the application
// All components with ComponentRunnable implementation will be closed in the reversed order
func (app *App) Close() error {
	logrus.Infof("Close components...")
	app.mu.RLock()
	defer app.mu.RUnlock()
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-time.After(time.Minute):
			panic("app.Close timeout")
		}
	}()

	var errs []string
	for i := len(app.components) - 1; i >= 0; i-- {
		if serviceClose, ok := app.components[i].(ComponentRunnable); ok {
			logrus.Debugf("Close '%s'", serviceClose.Name())
			if e := serviceClose.Close(); e != nil {
				errs = append(errs, fmt.Sprintf("Component '%s' close error: %v", serviceClose.Name(), e))
			}
		}
	}
	close(done)
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}
