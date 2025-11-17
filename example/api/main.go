package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/tompston/syro"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ExposeTestServer()
}

const (
	dbName = "test"
	apiUrl = "localhost:3094"
)

var ctx = context.Background()

func setupConn() (*mongo.Client, error) {
	url := "mongodb://localhost:27017"

	opt := options.Client().
		SetMaxConnIdleTime(10 * time.Minute).
		SetMaxPoolSize(20).
		ApplyURI(url)

	return mongo.Connect(ctx, opt)
}

func ExposeTestServer() {

	conn, err := setupConn()
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer conn.Disconnect(ctx)

	coll := conn.Database(dbName).Collection("test_collection_logs")
	logger := syro.NewMongoLogger(coll, nil)

	if err := logger.CreateIndexes(); err != nil {
		log.Fatalf("failed to create indexes: %v", err)
	}

	go startRandomLogging(logger)
	startServer(logger)
}

func doRandomLogging(logger syro.Logger) {

	logFields := GenerateRandomLogFields(RandomInt(0, 8))

	levels := syro.LogLevels
	randLevel := levels[rand.Intn(len(levels))]
	logger.Log(randLevel, RandomString(RandomInt(25, 500)), logFields)
}

func RandomInt(min, max int) int {
	return rand.Intn(max-min) + min
}

func startServer(logger syro.Logger) {
	http.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "false")
		w.Header().Set("Content-Type", "application/json")

		const maxLimit = 1000
		data, err := syro.NewQueryHandler().Logs(logger, maxLimit, r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	})

	log.Printf("Server is listening at http://%s", apiUrl)
	if err := http.ListenAndServe(apiUrl, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func RandomString(n int) string {
	const letterBytes = "         abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func startRandomLogging(logger *syro.MongoLogger) {
	for {
		doRandomLogging(logger)
		time.Sleep(1 * time.Second)
	}
}

func GenerateRandomLogFields(n int) syro.LogFields {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	words := []string{
		"foo", "bar", "baz", "qux", "lorem", "ipsum", "dolor", "sit", "amet", "consectetur",
	}

	randomStringValue := func(long bool) string {
		if long {
			n := 5 + rand.Intn(10)
			s := ""
			for range n {
				s += words[rand.Intn(len(words))] + " "
			}
			return s
		}
		return words[rand.Intn(len(words))]
	}

	randomValue := func() any {
		choice := rand.Intn(3)
		long := rand.Float32() < 0.5

		switch choice {
		case 0:
			return randomStringValue(long)

		case 1:
			if long {
				return rand.Intn(10000)
			}
			return rand.Intn(100)
		case 2:
			f := rand.Float64() * 100
			if long {
				return f * 100
			}
			return f
		default:
			return nil
		}
	}

	randomKey := func(length int) string {
		b := make([]rune, length)
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}
		return string(b)
	}

	logFields := make(syro.LogFields)
	for range n {
		key := randomKey(3 + rand.Intn(3)) // 3-5 chars
		logFields[key] = randomValue()
	}

	return logFields
}
