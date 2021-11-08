package humpback

import (
	"context"
	"log"
	"os"
	"sync"
)

const (
	GoroutineFinishQueue = 1000
)

var (
	once    sync.Once
	manager *GoroutineManager
)

var std = log.New(os.Stderr, "", log.LstdFlags)

func GetGoroutineManager() *GoroutineManager {
	once.Do(func() {
		manager = CreateGoroutineManager()
	})
	return manager
}

func CreateGoroutineManager() *GoroutineManager {
	m := &GoroutineManager{}
	m.done = make(chan struct{})
	m.finish = make(chan struct{}, GoroutineFinishQueue)

	return m
}

type GoroutineManager struct {
	mux        sync.RWMutex
	wg         sync.WaitGroup
	ctx        context.Context
	childCtx   context.Context
	childAbort context.CancelFunc
	finish     chan struct{}
	done       chan struct{}
	logger     Logger
}

func (ro *GoroutineManager) Done() <-chan struct{} {
	return ro.done
}

func (ro *GoroutineManager) Close() error {
	close(ro.done)
	return nil
}

func (ro *GoroutineManager) Finish() {
	ro.finish <- struct{}{}
}

func (ro *GoroutineManager) Logger() Logger {
	if nil == ro.logger {
		return std
	}
	return ro.logger
}

func (ro *GoroutineManager) SetLogger(l Logger) {
	ro.logger = l
}

func (ro *GoroutineManager) Start(ctx context.Context) error {

	ro.mux.Lock()
	ro.ctx = ctx
	ro.childCtx, ro.childAbort = context.WithCancel(ctx)
	ro.mux.Unlock()

	// add monitor wg
	ro.wg.Add(1)

	// monitor abort message
	go func() {
		select {
		case <-ctx.Done():
			ro.wg.Done()
			ro.Logger().Printf("finish monitor goroutine")
			return
		}
	}()

	// monitor child goroutine finish message
	go func() {
		for {
			select {
			case <-ro.finish:
				ro.wg.Done()
			}
		}
	}()

	// monitor child goroutine left
	go func() {
		ro.wg.Wait()
		_ = ro.Close()
		ro.Logger().Printf("goroutine manager has closed")
	}()

	return nil
}

func (ro *GoroutineManager) Restart(ctx context.Context) error {

	ro.childAbort()

	ro.mux.Lock()
	defer ro.mux.Unlock()

	ro.ctx = ctx
	ro.childCtx, ro.childAbort = context.WithCancel(ctx)

	return nil
}

func (ro *GoroutineManager) Apply(f func(ctx context.Context) error) {
	select {
	case <-ro.Done():
		ro.Logger().Printf("goroutine manager has closed, please check the status")
	default:
		go func() {
			defer func() {
				ro.Finish()
			}()

			defer func() {
				if err := recover(); nil != err {
					ro.Logger().Printf("panic occurs %s", err)
				}
			}()

			if err := f(ro.getChildContext()); nil != err {
				ro.Logger().Printf("goroutine return error: %s", err)
			}
		}()
	}
}

func (ro *GoroutineManager) getChildContext() context.Context {
	ro.mux.RLock()
	defer ro.mux.RUnlock()
	return ro.childCtx
}
