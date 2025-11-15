package syro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ctx = context.Background()

func TestLogger(t *testing.T) {
	t.Run("test-log-creation", func(t *testing.T) {
		log := NewLog(ERROR, "qweqwe", "my-source", "my-event", "my-event-id")

		decodedJSON, err := json.Marshal(log)
		if err != nil {
			t.Fatal(err)
		}

		jsonStr := string(decodedJSON)

		// parse the created_at field from the json string and check it the time is
		// within the last 2 seconds
		type parsed struct {
			CreatedAt time.Time `json:"ts" bson:"ts"`
		}

		t.Run("test-json-unmarshalling", func(t *testing.T) {
			if err := stringIncludes(jsonStr, []string{
				`"level":5`,
				`message":"qweqwe"`,
				`"source":"my-source"`,
				`"event":"my-event"`,
				`"event_id":"my-event-id"`,
				`"ts":`,
			}); err != nil {
				t.Fatal(err)
			}

			var v parsed
			if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
				t.Fatal(err)
			}

			if v.CreatedAt.Before(time.Now().Add(-2 * time.Second)) {
				t.Fatal("The time time is not within the last 2 seconds")
			}

			// Check the timezone of the created_at field
			if v.CreatedAt.Location().String() != "UTC" {
				t.Fatal("The created_at time is not in UTC")
			}
		})

		t.Run("test-string-method", func(t *testing.T) {
			logger := NewConsoleLogger(nil)
			str := log.String(logger)
			fmt.Printf("str: %v\n", str)

			now := time.Now().UTC()
			formattedTime := now.Format("2006-01-02 15:04:05")
			// NOTE: not sure if this will fail in some cases when running
			// remove the last 3 characters (seconds) from the formatted time
			formattedTime = formattedTime[:len(formattedTime)-3]
			if err := stringIncludes(str, []string{
				"error",
				"my-source",
				"my-event",
				"qweqwe",
				formattedTime, // check if the printed time is the same as the current time,
			}); err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("test-console-logger", func(t *testing.T) {
		if NewConsoleLogger(nil).GetProps().Settings != nil {
			t.Fatal("Settings should be nil")
		}

		_log := NewConsoleLogger(nil)
		_log.Info("my-message", LogFields{
			"field1": "val1",
			"field2": "val2",
		})

		if NewConsoleLogger(nil).WithEvent("my-event").GetProps().Event != "my-event" {
			t.Fatal("SetEvent failed")
		}

		lg := NewConsoleLogger(nil).
			WithSource("my-source").
			WithEventID("my-event-id")

		if lg.GetProps().Source != "my-source" && lg.GetProps().EventID != "my-event-id" {
			t.Fatal("WithEventID failed")
		}

		logExists, err := NewConsoleLogger(nil).LogExists(nil)
		if err == nil {
			t.Fatal("LogExists should always return an error")
		}

		if logExists {
			t.Fatal("LogExists should always return false for ConsoleLogger")
		}

		if err.Error() != "method cannot be used with ConsoleLogger" {
			t.Fatal("LogExists should always return a predefined error")
		}
	})
}

func TestErrGroup(t *testing.T) {
	t.Run("test-new-errgroup", func(t *testing.T) {
		eg := NewErrGroup()
		if eg == nil {
			t.Fatal("New() should not return nil")
		}

		if len(eg.Errors()) != 0 {
			t.Fatal("New() should initialize an empty ErrGroup")
		}
	})

	t.Run("test-add-error", func(t *testing.T) {
		eg := NewErrGroup()
		err1 := errors.New("first error")
		err2 := errors.New("second error")

		eg.Add(err1)
		if len(eg.Errors()) != 1 {
			t.Fatal("Add() did not properly add the first error")
		}

		eg.Add(err2)
		if len(eg.Errors()) != 2 {
			t.Fatal("Add() did not properly add the second error")
		}

		eg.Add(nil) // test adding nil error
		if len(eg.Errors()) != 2 {
			t.Fatal("Add() should not add nil errors")
		}
	})

	t.Run("test-errors", func(t *testing.T) {
		eg := NewErrGroup()

		eg.Add(errors.New("first error"))
		eg.Add(errors.New("second error"))

		expected := "first error; second error"
		if eg.Error() != expected {
			t.Fatalf("Error() returned %q, want %q", eg.Error(), expected)
		}

		eg = NewErrGroup() // test with no errors
		if eg.Error() != "" {
			t.Fatalf("Error() should return an empty string for an empty ErrGroup, got %q", eg.Error())
		}
	})

	t.Run("test-len", func(t *testing.T) {
		eg := NewErrGroup()
		if eg.Len() != 0 {
			t.Fatalf("Len() should return 0 for a new ErrGroup, got %d", eg.Len())
		}

		eg.Add(errors.New("first error"))
		if eg.Len() != 1 {
			t.Fatalf("Len() should return 1 after adding one error, got %d", eg.Len())
		}

		eg.Add(nil) // adding nil should not change the count
		if eg.Len() != 1 {
			t.Fatalf("Len() should still return 1 after adding nil, got %d", eg.Len())
		}
	})
}

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

	t.Run("cron-interface", func(t *testing.T) {
		listColl := conn.Database(dbName).Collection("test_syro_cron")
		if err := listColl.Drop(ctx); err != nil {
			t.Fatal(err)
		}

		historyColl := conn.Database(dbName).Collection("test_syro_cron_history")
		if err := historyColl.Drop(ctx); err != nil {
			t.Fatal(err)
		}

		cron := cron.New()

		store, err := NewMongoCronStorage(listColl, historyColl)
		if err != nil {
			t.Fatal(err)
		}

		// check if the store implements the CronStorage interface
		sched := NewCronScheduler(cron, "qweqw").WithStorage(store)
		_ = sched
	})

	t.Run("test-bson-unmarshalling", func(t *testing.T) {
		log := NewLog(ERROR, "qweqwe", "my-source", "my-event", "my-event-id")

		decodedBson, err := bson.MarshalExtJSON(&log, false, false)
		if err != nil {
			t.Fatal(err)
		}

		bsonStr := string(decodedBson)
		fmt.Printf("bsonStr: %v\n", bsonStr)

		if err := stringIncludes(bsonStr, []string{
			`"ts":{"$date":`,
			`message":"qweqwe"`,
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

		var parsedLog Log
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
		var log Log
		if err := coll.FindOne(ctx, bson.M{}).Decode(&log); err != nil {
			t.Fatal(err)
		}

		if log.Message != "qwe" {
			t.Fatal("The log message should be 'qwe'")
		}

		if log.Level != DEBUG {
			t.Fatal("The log level should be ", DEBUG)
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

		if err := logger.Debug("qwe", LogFields{
			"key1": "value1",
			"key2": 123,
			"asd":  asd,
		}); err != nil {
			t.Fatal(err)
		}

		var log Log
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
		test1, err := logger.FindLogs(LogFilter{
			TimeseriesFilter: TimeseriesFilter{Limit: 100, Skip: 0},
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
			if log.Level != DEBUG || log.Message != msg {
				t.Fatal("The logs are not correct")
			}
		}

		// ---- test the find logs method with a limit ----
		test2, err := logger.FindLogs(LogFilter{
			EventID:          "my-event-id",
			TimeseriesFilter: TimeseriesFilter{Limit: 5, Skip: 0},
		}, 5)

		if err != nil {
			t.Fatal(err)
		}

		if len(test2) != 5 {
			t.Fatalf("The number of logs should be %v", 5)
		}

		// ---- other filters ----
		test3, err := logger.FindLogs(LogFilter{
			EventID:          "this-event-does-not-exist",
			TimeseriesFilter: TimeseriesFilter{Limit: 100, Skip: 0},
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

func TestJobLocker(t *testing.T) {
	t.Run("test-job-run", func(t *testing.T) {
		counter := int32(0)
		j := newJobLock(func() {
			time.Sleep(100 * time.Millisecond)
			atomic.AddInt32(&counter, 1)
		}, "testJob")

		const maxRuns = 10

		var wg sync.WaitGroup
		wg.Add(maxRuns)
		for range maxRuns {
			go func() {
				defer wg.Done()
				j.Run()
			}()
		}
		wg.Wait()

		if atomic.LoadInt32(&counter) != 1 {
			t.Fatalf("Expected job function to run once, but it ran %d times", counter)
		}
	})

	t.Run("test-lock", func(t *testing.T) {
		counter := int32(0)
		j := newJobLock(func() {
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&counter, 1)
		}, "testJob")

		// first run
		go j.Run()

		// sleep for a moment to allow the first job to start running
		time.Sleep(10 * time.Millisecond)

		// second run
		go j.Run()

		// sleep for a moment to allow the second job to attempt to start
		time.Sleep(10 * time.Millisecond)

		// At this point, the second job should have attempted to start and failed,
		// so the counter should still be 0
		if atomic.LoadInt32(&counter) != 0 {
			t.Fatalf("Expected job function not to run, but it ran %d times", counter)
		}

		// Wait for the first job to complete
		time.Sleep(50 * time.Millisecond)

		// Now the counter should be 1
		if atomic.LoadInt32(&counter) != 1 {
			t.Fatalf("Expected job function to run once, but it ran %d times", counter)
		}
	})
}

func TestRequest(t *testing.T) {

	t.Run("with-headers-and-body", func(t *testing.T) {
		req := NewRequest("POST", "http://example.com").
			WithHeaders(map[string]string{"X-Test": "123"}).
			WithHeader("Content-Type", "application/json").
			WithBody([]byte(`{"a":1}`))

		if req.Headers["X-Test"] != "123" || req.Headers["Content-Type"] != "application/json" {
			t.Errorf("headers not set correctly: %+v", req.Headers)
		}
		if !bytes.Equal(req.Body, []byte(`{"a":1}`)) {
			t.Errorf("body mismatch: %s", string(req.Body))
		}
	})

	t.Run("request-curl-examples", func(t *testing.T) {
		t.Run("simple-get", func(t *testing.T) {
			req := NewRequest("POST", "http://example.com").
				WithHeader("X-Test", "123")
			curl := req.AsCURL()
			fmt.Printf("%v\n", curl)
		})
	})

	t.Run("with-ignore-status-codes", func(t *testing.T) {
		req := NewRequest("GET", "http://example.com").WithIgnoreStatusCodes(true)
		if !req.ignoreStatusCodes {
			t.Error("expected IgnoreStatusCodes to be true")
		}

		res, err := req.Do()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.StatusCode != 200 {
			t.Errorf("expected status code 200, got %d", res.StatusCode)
		}

		fmt.Printf("res.Body: %v\n", string(res.Body))
	})

	t.Run("fetch-missing-method-or-url", func(t *testing.T) {
		req := NewRequest("", "")
		_, err := req.Do()
		if err == nil || err.Error() != "request method is not set" {
			t.Errorf("expected error on missing method, got: %v", err)
		}

		req = NewRequest("GET", "")
		_, err = req.Do()
		if err == nil || err.Error() != "request URL is not set" {
			t.Errorf("expected error on missing URL, got: %v", err)
		}
	})

	t.Run("fetch-success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello world"))
		}))
		defer server.Close()

		fmt.Printf("server.URL: %v\n", server.URL)

		req := NewRequest("GET", server.URL)
		res, err := req.Do()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res.Body) != "hello world" {
			t.Errorf("unexpected body: %s", res.Body)
		}

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status code 200, got %d", res.StatusCode)
		}
	})

	t.Run("fetch-rejects-non-200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "fail", http.StatusTeapot)
		}))
		defer server.Close()

		req := NewRequest("GET", server.URL)

		_, err := req.Do()
		if err == nil || !contains(err.Error(), "status: 418") {
			t.Errorf("expected status error, got: %v", err)
		}
	})
}

func TestRequestsWithMockServer(t *testing.T) {

	// Create a simple echo server that reflects request data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("error reading body: %v", err)
		}
		defer r.Body.Close()

		// Build response that reflects the request
		response := map[string]any{
			"method":  r.Method,
			"path":    r.URL.Path,
			"query":   r.URL.RawQuery,
			"headers": r.Header,
		}

		// Include body if present
		if len(body) > 0 {
			response["body"] = string(body)
			// Try to parse as JSON
			var jsonBody any
			if err := json.Unmarshal(body, &jsonBody); err == nil {
				response["body_parsed"] = jsonBody
			}
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	url := server.URL

	t.Run("simple-get", func(t *testing.T) {
		res, err := NewRequest("GET", url).WithHeader("X-Test", "123").Do()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// fmt.Printf("res.Body: %v\n", string(res.Body))

		fmt.Printf("%v\n", res.Inspect())
	})
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestExecFuncs(t *testing.T) {
	alwaysOK := func() error { return nil }
	alwaysErr := func() error { return errors.New("fail") }

	// helper to generate repeated function slices
	repeat := func(fn func() error, n int) []func() error {
		funcs := make([]func() error, n)
		for i := 0; i < n; i++ {
			funcs[i] = fn
		}
		return funcs
	}

	tests := []struct {
		name        string
		funcs       []func() error
		parallelism int
		handling    ErrorHandlingOption
		wantErrs    int  // number of errors expected
		wantNil     bool // if we expect the result to be nil (e.g. ReturnNoneIfAnyError and any error occurs)
	}{
		{
			name:        "all pass - collect all",
			funcs:       repeat(alwaysOK, 5),
			parallelism: 2,
			handling:    ExecFuncCollectAllErrors,
			wantErrs:    0,
		},
		{
			name:        "some fail - collect all",
			funcs:       append(repeat(alwaysOK, 3), repeat(alwaysErr, 2)...),
			parallelism: 3,
			handling:    ExecFuncCollectAllErrors,
			wantErrs:    2,
		},
		{
			name:        "first fails - break on first",
			funcs:       append([]func() error{alwaysErr}, repeat(alwaysOK, 4)...),
			parallelism: 2,
			handling:    ExecFuncBreakOnFirstError,
			wantErrs:    1,
		},
		{
			name:        "some fail - return none if any",
			funcs:       append(repeat(alwaysOK, 4), alwaysErr),
			parallelism: 3,
			handling:    ExecFuncReturnNoneIfAnyError,
			wantNil:     true,
		},
		{
			name:        "all fail - ignore all",
			funcs:       repeat(alwaysErr, 5),
			parallelism: 2,
			handling:    ExecFuncIgnoreAllErrors,
			wantErrs:    0,
		},
		{
			name:        "negative parallelism - collect all",
			funcs:       repeat(alwaysErr, 3),
			parallelism: -1,
			handling:    ExecFuncCollectAllErrors,
			wantErrs:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			errs := ExecFuncs(tt.funcs, tt.parallelism, tt.handling)
			elapsed := time.Since(start)

			if tt.wantNil && errs != nil {
				t.Errorf("expected nil result, got %v", errs)
			}
			if !tt.wantNil && len(errs) != tt.wantErrs {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrs, len(errs), errs)
			}

			// Print timing info to verify parallelism roughly works
			fmt.Printf("[%s] Took %s\n", tt.name, elapsed)
		})
	}
}

func BenchmarkLogString(b *testing.B) {
	log := Log{
		Timestamp: time.Now().UTC(),
		Level:     INFO,
		Message:   "Test message for benchmarking",
		Source:    "api",
		Event:     "request",
		EventID:   "evt_123456",
		Fields: LogFields{
			"user_id": "12345",
			"action":  "login",
			"ip":      "192.168.1.1",
		},
	}

	logger := NewConsoleLogger(LoggerSettingsDefault)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = log.String(logger)
	}
}
