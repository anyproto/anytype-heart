package comptester

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
)

type TestMode string

var (
	TestModeFailOnInit  TestMode
	TestModeFailOnRun   TestMode
	TestModeFailOnClose TestMode
)

type CompTester struct {
	mode    TestMode
	current int
	failAt  int
}

type testComp struct {
	comptester *CompTester
	comp       app.Component
}

type testCompRunnable struct {
	testComp
}

func New(mode TestMode, failAt int) *CompTester {
	return &CompTester{
		mode:    mode,
		current: 0,
		failAt:  failAt,
	}
}

func (t *testCompRunnable) Run(ctx context.Context) error {
	if t.comptester.mode == TestModeFailOnRun {
		if t.comptester.current == t.comptester.failAt {
			return fmt.Errorf("force fail on Run at %s(%d)", t.comp.Name(), t.comptester.current)
		}
		t.comptester.current++
	}

	return t.comp.(app.ComponentRunnable).Run(ctx)
}

func (t *testCompRunnable) Close(ctx context.Context) error {
	if t.comptester.mode == TestModeFailOnClose {
		if t.comptester.current == t.comptester.failAt {
			return fmt.Errorf("force fail on Close at %s(%d)", t.comp.Name(), t.comptester.current)
		}
		t.comptester.current++
	}
	return t.comp.(app.ComponentRunnable).Close(ctx)
}

func (t *CompTester) NewComp(comp app.Component) app.Component {
	tc := testComp{
		comptester: t,
		comp:       comp,
	}
	if _, ok := comp.(app.ComponentRunnable); ok {
		return &testCompRunnable{
			tc,
		}
	}
}

func (t *testComp) Name() string {
	return t.comp.Name()
}

func (t *testComp) Init(a *app.App) error {
	if t.comptester.mode == TestModeFailOnInit {
		if t.comptester.current == t.comptester.failAt {
			return fmt.Errorf("force fail on Init at %s(%d)", t.comp.Name(), t.comptester.current)
		}
		t.comptester.current++
	}
	return t.comp.Init(a)
}
