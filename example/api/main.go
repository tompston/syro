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

func setupConn() (*mongo.Client, error) {
	url := "mongodb://localhost:27017"

	opt := options.Client().
		SetMaxPoolSize(20).                   // Set the maximum number of connections in the connection pool
		SetMaxConnIdleTime(10 * time.Minute). // Close idle connections after the specified time
		ApplyURI(url)

	// conn, err := mongo.Connect(context.Background(), opt)
	// if err != nil {
	// 	return nil, err
	// }
	// defer conn.Disconnect(context.Background())

	return mongo.Connect(context.Background(), opt)
}

func ExposeTestServer() {

	conn, err := setupConn()
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer conn.Disconnect(context.Background())

	coll := conn.Database(dbName).Collection("test_collection_logs")
	logger := syro.NewMongoLogger(coll, nil)

	if err := logger.CreateIndexes(); err != nil {
		log.Fatalf("failed to create indexes: %v", err)
	}

	go startRandomLogging(logger)
	startServer(logger)
}

func doRandomLogging(logger *syro.MongoLogger) {
	levels := syro.LogLevels
	randLevel := levels[rand.Intn(len(levels))]
	logger.Log(randLevel, RandomString(RandomInt(25, 500)))
}

func RandomInt(min, max int) int {
	return rand.Intn(max-min) + min
}

func startServer(logger *syro.MongoLogger) {
	finder := syro.Finder()

	http.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "false")
		w.Header().Set("Content-Type", "application/json")

		const maxLimit = 1000
		data, err := finder.Logs(logger, maxLimit, r.URL.String())
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

// func RandomElement[T any](slice []T) T {
// 	return slice[rand.Intn(len(slice))]
// }

func RandomString(n int) string {
	const letterBytes = " abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
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
