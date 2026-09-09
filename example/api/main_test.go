package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tompston/syro"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoImpl(t *testing.T) {

	url := "mongodb://localhost:27017"

	const (
		dbName = "test"
	)

	opt := options.Client().
		SetMaxPoolSize(20).                   // Set the maximum number of connections in the connection pool
		SetMaxConnIdleTime(10 * time.Minute). // Close idle connections after the specified time
		ApplyURI(url)

	conn, err := mongo.Connect(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Disconnect(ctx)

	cron := cron.New()

	jobLogsColl := conn.Database(dbName).Collection("test_syro_cron_exec_logs")
	if err := jobLogsColl.Drop(ctx); err != nil {
		t.Fatal(err)
	}

	logger := NewMongoLogger(jobLogsColl, nil)

	jobListColl := conn.Database(dbName).Collection("test_syro_cron")
	if err := jobListColl.Drop(ctx); err != nil {
		t.Fatal(err)
	}

	store, err := NewMongoCronStorage(jobListColl)
	if err != nil {
		t.Fatal(err)
	}

	var stringChangedOnErrorOfCronCallback = ""
	const STRING_POST_CRON_EXEC = "hello there"

	var myCustomErr = errors.New("always return an error")

	job1 := &syro.Job{
		Schedule: "@every 1s",
		Name:     "my-first-cron",
		Func: func() error {
			return fmt.Errorf("always return an error")
		},
		OnComplete: func(err error) {
			if err != nil {
				stringChangedOnErrorOfCronCallback = STRING_POST_CRON_EXEC
			}
		},
	}

	// check if the store implements the CronStorage interface
	scheduler := syro.NewCronScheduler(cron, "my-cron-app").WithStorage(store).WithExecLogger(logger)

	t.Run("cronjob-tests", func(t *testing.T) {

		t.Run("cronjob-callbacks-work", func(t *testing.T) {

			if err := scheduler.Register(job1); err != nil {
				t.Fatal(err)
			}

			scheduler.Start()
			time.Sleep(time.Second * 3) // run for 3sec, so that the cron job executes
			if stringChangedOnErrorOfCronCallback != STRING_POST_CRON_EXEC {
				t.Fatalf("expected to have a mutation of string when the cron executes")
			}

			logs, err := logger.FindLogs(syro.LogFilter{}, 100)
			if err != nil {
				log.Fatal(err)
			}

			if len(logs) == 0 {
				t.Fatal("expected to find logs in the db after specifying the logger for the scheduler")
			}

			for _, l := range logs {
				if l.Source != scheduler.Source {
					t.Fatalf("expected the logs in the db to have the source of %v, got %v", scheduler.Source, l.Source)
				}

				if l.Event != job1.Name {
					t.Fatalf("expected the logs in the db to have the event of %v, got %v", job1.Name, l.Event)
				}

				expectedKeysOfFields := []string{"time_init", "time_finish", "exec_dur"}

				for _, expectedKey := range expectedKeysOfFields {
					_, exists := l.Fields[expectedKey]
					if !exists {
						t.Fatalf("log fields did not have the expected value of %v", expectedKey)
					}
				}

				if l.Message != myCustomErr.Error() {
					t.Fatalf("expected the msg field to have the value of the error returned from the cron function (%v), found %v", myCustomErr, l.Message)
				}

				// get all crons
				storage, err := scheduler.Storage()
				if err != nil {
					t.Fatal(err)
				}

				crons, err := storage.FindCronJobs()
				if err != nil {
					t.Fatal(err)
				}

				if len(crons) != 1 {
					t.Fatalf("expected to find 1 cron in the collection, found %v", len(crons))
				}

				for _, c := range crons {
					fmt.Printf("id: %v, source: %v, name: %v, sched: %v\n", c.ID, c.Source, c.Name, c.Schedule)

					if _, err := primitive.ObjectIDFromHex(c.ID); err != nil {
						t.Fatalf("unmarshalling the db cron entry id to an id got an err: %v", err)
					}
				}
			}
		})
	})

	t.Run("test-bson-unmarshalling", func(t *testing.T) {
		log := syro.NewLog(syro.ERROR, "qweqwe", "my-source", "my-event", "my-event-id")

		decodedBson, err := bson.MarshalExtJSON(&log, false, false)
		if err != nil {
			t.Fatal(err)
		}

		bsonStr := string(decodedBson)
		fmt.Printf("bsonStr: %v\n", bsonStr)

		if err := stringIncludes(bsonStr, []string{
			`"ts":{"$date":`,
			`msg":"qweqwe"`,
			`"source":"my-source"`,
			`"event":"my-event"`,
			`"event_id":"my-event-id"`,
		}); err != nil {
			t.Fatal(err)
		}

		bsonBytes, err := bson.Marshal(log)
		if err != nil {
			t.Fatal(err)
		}

		var parsedLog syro.Log
		if err := bson.Unmarshal(bsonBytes, &parsedLog); err != nil {
			t.Fatalf("BSON Unmarshal failed with error: %v", err)
		}

		if parsedLog.Timestamp.Before(time.Now().Add(-2 * time.Second)) {
			t.Fatal("The created_at time is not within the last 2 seconds")
		}
	})

	t.Run("test log creation", func(t *testing.T) {
		coll := conn.Database(dbName).Collection("test_syro_mongo_logger")
		// Remove the previous data
		if err := coll.Drop(ctx); err != nil {
			t.Fatal(err)
		}

		logger := NewMongoLogger(coll, nil)
		if logger == nil {
			t.Error("NewMongoLogger should not return nil")
		}

		if err := logger.Debug("qwe"); err != nil {
			t.Fatal(err)
		}

		// find the log in the collection
		var log syro.Log
		if err := coll.FindOne(ctx, bson.M{}).Decode(&log); err != nil {
			t.Fatal(err)
		}

		if log.Message != "qwe" {
			t.Fatal("The log message should be 'qwe'")
		}

		if log.Level != syro.DEBUG {
			t.Fatal("The log level should be ", syro.DEBUG)
		}

		if log.Source != "" {
			t.Fatal("The log source should be empty")
		}

		if log.Source != "" {
			t.Fatal("The log source should be empty")
		}

		if log.Event != "" {
			t.Fatal("The log event should be empty")
		}

		if log.EventID != "" {
			t.Fatal("The log event_id should be empty")
		}

		// if the time is not within the last 2 seconds
		if log.Timestamp.Before(time.Now().Add(-2 * time.Second)) {
			t.Fatal("The created_at time is not within the last 2 seconds")
		}
	})

	t.Run("test log fields", func(t *testing.T) {
		coll := conn.Database(dbName).Collection("test_mongo_logger_with_fields")
		if err := coll.Drop(ctx); err != nil {
			t.Fatal(err)
		}

		logger := NewMongoLogger(coll, nil)

		var asd error

		if err := logger.Debug("qwe", syro.LogFields{
			"key1": "value1",
			"key2": 123,
			"asd":  asd,
		}); err != nil {
			t.Fatal(err)
		}

		var log syro.Log
		if err := coll.FindOne(ctx, bson.M{}).Decode(&log); err != nil {
			t.Fatal(err)
		}

		fmt.Printf("log.Fields: %v\n", log.Fields)
		for k, v := range log.Fields {
			fmt.Printf("k: %-10v v: %-10v type: %-10T\n", k, v, v)
		}

		// test if the expected fields are in the log
		if log.Fields["key1"] != "value1" {
			t.Error("The key1 field should be 'value1', got: ", log.Fields["key1"])
		}

		// NOTE: i'm not sure what to do in this case, tests fail without the int32 type
		if log.Fields["key2"] != int32(123) {
			t.Error("The key2 field should be 123, got: ", log.Fields["key2"])
		}

		if log.Fields["asd"] != nil {
			t.Error("The asd field should be the same as the asd variable")
		}
	})

	t.Run("test log creation", func(t *testing.T) {

		coll := conn.Database(dbName).Collection("test_mongo_logger_with_source")
		if err := coll.Drop(ctx); err != nil {
			t.Fatal(err)
		}

		logger := NewMongoLogger(coll, nil).WithEventID("my-event-id")

		if err := logger.Info("my unique info event"); err != nil {
			t.Fatal(err)
		}

		t.Run("check if a created log exists", func(t *testing.T) {
			filter := bson.M{"event_id": "my-event-id"}
			exists, err := logger.LogExists(filter)
			if err != nil {
				t.Fatal(err)
			}

			if !exists {
				t.Fatal("The log should exist")
			}
		})

		t.Run("check if a non existent log does not exitst", func(t *testing.T) {
			filter := bson.M{"event_id": "this does not exist"}
			exists, err := logger.LogExists(filter)
			if err != nil {
				t.Fatal(err)
			}

			if exists {
				t.Fatal("The log should not exist")
			}
		})
	})

	t.Run("test find logs", func(t *testing.T) {
		coll := conn.Database(dbName).Collection("test_mongo_logger_find_logs")
		if err := coll.Drop(ctx); err != nil {
			t.Fatal(err)
		}

		msg := "this is a test"
		numLogs := 10

		logger := NewMongoLogger(coll, nil).WithEventID("my-event-id")
		for range numLogs {
			logger.Debug(msg)
		}

		// ---- test the find logs method ----
		test1, err := logger.FindLogs(syro.LogFilter{
			TimeseriesFilter: syro.TimeseriesFilter{Limit: 100, Skip: 0},
			EventID:          "my-event-id",
		}, 1000)

		if err != nil {
			t.Fatal(err)
		}

		if len(test1) != numLogs {
			t.Fatalf("The number of logs should be %v", numLogs)
		}

		// if all of the logs are not debug level and the data is not msg
		// then the test failed
		for _, log := range test1 {
			if log.Level != syro.DEBUG || log.Message != msg {
				t.Fatal("The logs are not correct")
			}
		}

		// ---- test the find logs method with a limit ----
		test2, err := logger.FindLogs(syro.LogFilter{
			EventID:          "my-event-id",
			TimeseriesFilter: syro.TimeseriesFilter{Limit: 5, Skip: 0},
		}, 5)

		if err != nil {
			t.Fatal(err)
		}

		if len(test2) != 5 {
			t.Fatalf("The number of logs should be %v", 5)
		}

		// ---- other filters ----
		test3, err := logger.FindLogs(syro.LogFilter{
			EventID:          "this-event-does-not-exist",
			TimeseriesFilter: syro.TimeseriesFilter{Limit: 100, Skip: 0},
		}, 1_000)

		if err != nil {
			t.Fatal(err)
		}

		if len(test3) != 0 {
			t.Fatalf("The number of logs should be %v", 0)
		}
	})
}

// stringIncludes checks if the input string contains all of the strings in the array.
func stringIncludes(s string, arr []string) error {
	for _, str := range arr {
		if !strings.Contains(s, str) {
			return fmt.Errorf("input string '%s' does not include '%s'", s, str)
		}
	}
	return nil
}
