package main

import (
	"binance-pooler/internal/pooler/services/binance_service"
	"binance-pooler/pkg/app"
	"binance-pooler/pkg/providers/binance"
	"binance-pooler/pkg/syro"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// go run cmd/pooler/main.go
func main() {
	loc, err := time.LoadLocation("Europe/Riga")
	if err != nil {
		log.Fatalf(err.Error())
	}

	app, err := app.New(context.Background())
	if err != nil {
		msg := fmt.Sprintf("failed to create app in go pooler: %v", err.Error())
		log.Fatalf(msg)
	}
	defer app.Exit(context.Background())

	scheduler, err := InitializeScheduler(app, loc)
	if err != nil {
		log.Fatalf(err.Error())
	}

	// fmt.Printf("scheduler.Jobs: %#v\n", scheduler.Jobs)
	// fmt.Printf("format string")
	scheduler.Start()
	select {} // run forever

	// for _, job := range scheduler.Jobs {
	// 	job.Func()
	// }
}

func InitializeScheduler(app *app.App, loc *time.Location) (*syro.CronScheduler, error) {
	cron := cron.New(cron.WithLocation(loc))

	scheduler := syro.NewCronScheduler(cron, "go-pooler").
		WithStorage(app.CronStorage())

	timeframes := []binance.Timeframe{
		binance.Timeframe1M,
		binance.Timeframe5M,
		binance.Timeframe15M,
	}

	if err := binance_service.New(app, 3, timeframes).
		WithSleepDuration(100 * time.Millisecond).
		WithDebugMode().
		AddJobs(scheduler); err != nil {
		return nil, fmt.Errorf("failed to add binance jobs to scheduler: %v", err)
	}

	return scheduler, nil
}
