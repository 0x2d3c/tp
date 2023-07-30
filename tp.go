package tp

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

type TaskAny interface {
	// Execute task execution function
	Execute(ctx context.Context, cacheKey string)
}

// task inner task define
type task struct {
	key    string
	fn     TaskAny
	cancel chan struct{}
	after  time.Duration
}

// TaskMGR task manager
type TaskMGR struct {
	size      int32
	taskKey   string
	cancelKey string
	chTask    chan *task
	chCancel  chan string
	kv        map[string]*task
	runners   map[string]TaskAny
	rdb       redis.UniversalClient
}

// run running task consumer
func (mgr *TaskMGR) run() {
	for {
		select {
		case one := <-mgr.chTask:
			timer := time.NewTimer(one.after * time.Second)

			ctx, cancel := context.WithCancel(context.Background())
			select {
			case <-one.cancel:
				cancel()
			case <-timer.C:
				one.fn.Execute(ctx, one.key)

				mgr.chCancel <- one.key
			}
			timer.Stop()
		}
	}
}

// run running task consumer
func (mgr *TaskMGR) kvClean() {
	for taskKey := range mgr.chCancel {
		delete(mgr.kv, taskKey)
	}
}

// cancel remove task in wait
func (mgr *TaskMGR) cancel() {
	subscribe := mgr.rdb.Subscribe(context.Background(), mgr.cancelKey)
	for {
		mess, err := subscribe.ReceiveMessage(context.Background())
		if err != nil {
			continue
		}

		one, ok := mgr.kv[mess.Payload]
		if !ok {
			continue
		}
		close(one.cancel)

		delete(mgr.kv, mess.Payload)
	}
}

// consumer task in wait
func (mgr *TaskMGR) consumer() {
	for {
		set := mgr.rdb.BZPopMin(context.Background(), 0, mgr.taskKey)

		str, ok := set.Val().Member.(string)
		if !ok {
			continue
		}

		var mem member
		if err := sonic.UnmarshalString(str, &mem); err != nil {
			continue
		}

		after := time.Duration(time.Now().Unix()) - mem.At
		if after < 0 {
			continue
		}

		one := &task{key: mem.Key, after: after, cancel: make(chan struct{}), fn: mgr.runners[mem.Runner]}

		mgr.kv[mem.Key] = one

		mgr.chTask <- one
	}
}

type member struct {
	Key    string
	Runner string
	At     time.Duration
}

// AddTask add task
func (mgr *TaskMGR) AddTask(key, runner string, at int64) error {
	mem := &member{At: time.Duration(at), Key: key, Runner: runner}
	bs, err := sonic.Marshal(mem)
	if err != nil {
		return err
	}
	return mgr.rdb.ZAdd(context.Background(), mgr.taskKey, redis.Z{Score: float64(mem.At), Member: string(bs)}).Err()
}

// CancelTask cancel task
func (mgr *TaskMGR) CancelTask(key string) error {
	return mgr.rdb.Publish(context.Background(), mgr.cancelKey, key).Err()
}

// LoadRunner add task runner, set runner first
func (mgr *TaskMGR) LoadRunner(name string, runner TaskAny) {
	mgr.runners[name] = runner
}

// NewTaskMGR create a TaskMGR task manager
func NewTaskMGR(size int, cancelKey, taskKey string, rdb redis.UniversalClient) *TaskMGR {

	mgr := &TaskMGR{
		rdb:       rdb,
		taskKey:   taskKey,
		cancelKey: cancelKey,
		size:      int32(size),
		chCancel:  make(chan string),
		chTask:    make(chan *task, size),
		kv:        make(map[string]*task),
		runners:   make(map[string]TaskAny),
	}

	go mgr.run()
	go mgr.cancel()
	go mgr.kvClean()
	go mgr.consumer()

	return mgr
}
