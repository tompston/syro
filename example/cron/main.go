package main

import (
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tompston/syro"
)

func main() {
	loc, err := time.LoadLocation("Europe/Riga")
	if err != nil {
		log.Fatal(err)
	}

	cron := cron.New(cron.WithLocation(loc))

	scheduler := syro.NewCronScheduler(cron, "my-cron-app")

	if err := addCrons(scheduler); err != nil {
		log.Fatal(err)
	}

	for _, j := range scheduler.Jobs {
		fmt.Printf("sched: %v, name: %v\n", j.Schedule, j.Name)
	}

	scheduler.Start()
	select {} // run forever
}

func addCrons(sched *syro.CronScheduler) error {
	if err := sched.Register(
		&syro.Job{
			Name:     "print-cron-job",
			Schedule: "@every 1s",
			Func: func() error {
				fmt.Printf("%v this is my cron\n", time.Now().Unix())
				time.Sleep(time.Second * 3) // sleep for 3 sec in a 1s cron to show the lock
				return nil
			},
		},
	); err != nil {
		return err
	}

	if err := sched.Register(
		&syro.Job{
			Name:     "my-other-cron",
			Schedule: "@every 1h",
			Func: func() error {
				fmt.Printf("%v this is my hourly cron\n", time.Now().Unix())
				return nil
			},
		},
	); err != nil {
		return err
	}

	return nil
}
