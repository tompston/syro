# syro

Aka a bunch or util things that can be used between projects

> NOTE: there is 0 obligation from my side that the exposed api will not change in the future. At least not ini the beginging

This includes

- structured logging, based around interfaces (with implementation for console and mongodb logger)
- wrapper around `robfig/cron/v3` cron job scheduler with
  - locked execution (next cron won't trigger if previous has not finished)
  - optional `OnError`, `OnComplete` callbacks
  - optional storage specifier for cron job list and executions (NOT FINISHED)
- error group: accumulate errors under a single struct
- http request util for making requests in a simple way
  - comes with an optional debug method for sharing the sent requests (as curl) and response data
- simple atomic wrapper for types, based on generics
  - provides wrapping a value to be concurrecy safe, with 2 simple methods, `Set()` and `Get()`
- util for executing functions concurrently
  - can set how many of them can run concurrently
  - can define how they should execute (return on first error, collect all errors etc etc)

### Logger

Logging is implemented with structured logging in mind. The logs come with 3 optional fields that can be used to easily scope and then filter the logs. The optional fields are

- source: eg api, pooler, cron-job
- event: eg users-api, auth-api
- event_id: user-get-status, pool-binance-spot-data

There is also an optional LogFields map[string]any value that can be provided when logging, for unstructured / dynamic information.

##### Example

```go
func main() {
	logger := syro.NewConsoleLogger(nil).
		WithSource("my-event").
		WithEvent("my-source")

	logger.Info("Hello World")
}

// 2025-11-15 13:30:32  info    my-event    my-source     Hello World
```

The time zone and time format shown in the console can be modified to the users preferences, by specifying them when creating the logger.

### Cron jobs

Example of setting up a cron scheduler with one cron. Full example can be found at [binance-pooler](https://github.com/tompston/binance-pooler) repo

```go
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

	if err := addCron(scheduler); err != nil {
		log.Fatal(err)
	}

	scheduler.Start()
	select {} // run forever
}

func addCron(sched *syro.CronScheduler) error {
	if err := sched.Register(
		&syro.Job{
			Name:     "print-cron-job",
			Schedule: "@every 1s",
			Func: func() error {
				fmt.Printf("%v this is my cron\n", time.Now().Unix())
				time.Sleep(time.Second * 3) // sleep for 3sec in a 1sec cron to show the lock
				return nil
			},
		},
	); err != nil {
		return err
	}

	return nil
}

/** Output

1763215094 this is my cron
job print-cron-job already running. Skipping...
job print-cron-job already running. Skipping...
1763215097 this is my cron
job print-cron-job already running. Skipping...
job print-cron-job already running. Skipping...
1763215100 this is my cron
job print-cron-job already running. Skipping...
job print-cron-job already running. Skipping...
job print-cron-job already running. Skipping...

*/

```

### Http Requests

You can build requests with the `syro.NewRequest()` function. It has multiple methods that can be used to further extend the request data.

#### Example

```go
func main() {
	res, err := syro.NewRequest("GET", "https://httpbin.org/get").Do()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%v\n", res.Inspect())
}

/** Output

───── REQUEST ─────────────────────────
curl "https://httpbin.org/get"

───── RESPONSE ────────────────────────
Status: 200
Duration: 1.505626333s

Headers:
  Server: gunicorn/19.9.0
  Access-Control-Allow-Origin: *
  Access-Control-Allow-Credentials: true
  Date: Sat, 15 Nov 2025 13:51:38 GMT
  Content-Type: application/json
  Content-Length: 272

Body:
   {
     "args": {},
     "headers": {
       "Accept-Encoding": "gzip",
       "Host": "httpbin.org",
       "User-Agent": "Go-http-client/2.0",
     },
     "origin": "87.110.57.111",
     "url": "https://httpbin.org/get"
   }
*/
```

### Notes

- the logger is based around interfaces that have GetX and FindX type of methods, which can be used to retrieve the logs from the database. Simple queries can be done in this way, but once you need complex queries for the logs, then the interface methods are not quite the best approach.
- This package does come with a dependency for `go.mongodb.org/mongo-driver`, as there is an implementation for the logger and cron job storage interface with it. The correct way would be to split this into two seperate packages, but an overkill of complexity at the start.
- yes, syro is a reference to the aphex twin album
