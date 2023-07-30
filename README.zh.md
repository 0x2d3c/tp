
# TP

[English](./README.md) | 中文
依赖于redis的分布式任务管理器，目前是初始版本。
## 安装
```bash
go get -u github.com/0x2d3c/tp
```
## 支持
1.支持单个非循环任务的消费以及撤销
### 例子
```go
package main


import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/0x2d3c/tp"
	"github.com/redis/go-redis/v9"
)

type OneTask struct {
	// db connection or other options
}

func (ot *OneTask) Execute(ctx context.Context, cacheKey string) {
	fmt.Println(cacheKey)
}

func rdbGen() redis.UniversalClient {
	return redis.NewUniversalClient(&redis.UniversalOptions{Addrs: []string{":6379"}})
}

func main() {
	mgr := tp.NewTaskMGR(1, "cancelKey", "totalTaskKey", rdbGen())

	now := time.Now().Unix()

	mgr.LoadRunner("OneTask", &OneTask{})

	// maybe key style is runner:id
	for i := 0; i < 10; i++ {
		key := "OneTask:" + strconv.FormatInt(now, 10)

		go mgr.CancelTask(key)

		go mgr.AddTask(key, "OneTask", now)
	}

	time.Sleep(time.Minute)
}
```