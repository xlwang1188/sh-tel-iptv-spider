package initialize

import (
	"iptv-spider-sh/global"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

var onceCron sync.Once

func InitCron() {
	if global.CRON == nil {
		onceCron.Do(func() {

			loc, err := time.LoadLocation("Asia/Shanghai")
			if err != nil {
				panic(err)
			}

			global.CRON = cron.New(
				cron.WithSeconds(),
				cron.WithLocation(loc),
			)

			global.CRON.Start()
		})
	}
}
