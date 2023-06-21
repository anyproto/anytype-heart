package clickhouse

import (
	"context"
	"database/sql"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"time"

	"github.com/panjf2000/ants/v2"
)

var (
	log = logging.Logger(CName)
)

const (
	CName   = "metrics.clickhouse"
	size    = 10
	seconds = 120
)

type Service interface {
	AddEvent(event Event)
	app.ComponentRunnable
}

type Event interface {
	table() string
	toRecord() []any
}

type client struct {
	pool           *ants.PoolWithFunc
	db             *sql.DB
	cache          map[string][]Event
	count          int
	countDownFlush *time.Timer
}

func New() *client {
	return &client{}
}

func (c *client) Init(a *app.App) (err error) {
	return nil
}

func (c *client) Name() (name string) {
	return CName
}

func (c *client) Close(ctx context.Context) (err error) {
	c.countDownFlush.Stop()
	c.pool.Release()
	err = c.db.Close()
	return err
}

func (c *client) AddEvent(event Event) {
	err := c.pool.Invoke(event)
	if err != nil {
		log.Debug(err)
	}
}

func (c *client) Run(ctx context.Context) (err error) {
	pool, err := ants.NewPoolWithFunc(size, func(i interface{}) {
		event, ok := i.(Event)
		if ok {
			c.handleEvent(event)
		}
	})
	if err != nil {
		log.Errorf("failed to create ants pool: %v", err)
	}
	c.pool = pool
	c.db = connectDB()
	c.cache = make(map[string][]Event)
	c.count = 0
	c.countDownFlush = c.startCountdown()
	return nil
}

func (c *client) startCountdown() *time.Timer {
	return NewTimer(seconds, func() {
		c.flush()
		c.countDownFlush = c.startCountdown()
	})
}

func (c *client) handleEvent(event Event) {
	eventCache, exists := c.cache[event.table()]
	if !exists {
		eventCache = make([]Event, 0)
		c.cache[event.table()] = eventCache
	}
	eventCache = append(eventCache, event)
	c.cache[event.table()] = eventCache
	c.count++
	if c.count >= 100 {
		c.flush()
	}
}

func (c *client) flush() {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("panic during sending the metrics: %v", r)
		}
	}()
	if c.count == 0 {
		return
	}
	scope, err := c.db.Begin()
	for table, events := range c.cache {
		if err != nil {
			log.Errorf("unable to begin transaction: %v", err)
		}
		batch, err := scope.Prepare("INSERT INTO " + table)
		for _, event := range events {
			_, err = batch.Exec(event.toRecord()...)
			if err != nil {
				log.Errorf("unable to insert: %v", err)
			}
		}
	}
	err = scope.Commit()
	if err != nil {
		log.Errorf("unable to commit transaction: %v", err)
	}
	c.cache = make(map[string][]Event)
	c.count = 0
}

func NewTimer(seconds int, action func()) *time.Timer {
	timer := time.NewTimer(time.Second * time.Duration(seconds))

	go func() {
		<-timer.C
		action()
	}()

	return timer
}

func connectDB() *sql.DB {
	return clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{"clickhouse-log1.toolpad.org:8123"},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "",
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: 30 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		Protocol: clickhouse.HTTP,
	})
}
