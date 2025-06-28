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

func ExposeTestServer() {
	url := "mongodb://localhost:27017"

	const (
		dbName = "test"
		apiUrl = "localhost:3094"
	)

	opt := options.Client().
		SetMaxPoolSize(20).                   // Set the maximum number of connections in the connection pool
		SetMaxConnIdleTime(10 * time.Minute). // Close idle connections after the specified time
		ApplyURI(url)

	conn, err := mongo.Connect(context.Background(), opt)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer conn.Disconnect(context.Background())

	coll := conn.Database(dbName).Collection("test_collection_logs")
	logger := syro.NewMongoLogger(coll, nil)
	queries := syro.Query()

	// HTTP handler for /logs
	http.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "false")
		w.Header().Set("Content-Type", "application/json")

		const maxLimit = 1000

		data, err := queries.Logs(logger, maxLimit, r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	})

	if err := http.ListenAndServe(apiUrl, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func doRandomLogging(logger *syro.MongoLogger) {
	randomLevel := RandomElement(syro.LogLevels)
	logger.Log(randomLevel, "qweqwe")
}

func RandomElement[T any](slice []T) T {
	if len(slice) == 0 {
		panic("cannot select random element from empty slice")
	}
	rand.Seed(time.Now().UnixNano())
	return slice[rand.Intn(len(slice))]
}
