package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/tompston/syro"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	logger := NewMongoLogger(coll, nil)

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

func startRandomLogging(logger *MongoLogger) {
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

// ------------- Mongo implementation

type MongoLogger struct {
	Coll     *mongo.Collection
	Settings *syro.LoggerSettings
	Source   string
	Event    string
	EventID  string
}

func NewMongoLogger(coll *mongo.Collection, settings *syro.LoggerSettings) *MongoLogger {
	return &MongoLogger{Coll: coll, Settings: settings}
}

func (lg *MongoLogger) CreateIndexes() error {
	return NewMongoIndexes().
		Add("ts", "level").
		Add("source", "event").
		Add("event_id").
		Create(lg.Coll)
}

func (lg *MongoLogger) GetTableName() string {
	return lg.Coll.Name()
}

func (lg *MongoLogger) GetProps() syro.LoggerProps {
	return syro.LoggerProps{
		Settings: lg.Settings,
		Source:   lg.Source,
		Event:    lg.Event,
		EventID:  lg.EventID,
	}
}

func (lg *MongoLogger) Name() string {
	return "mongo"
}

func (lg *MongoLogger) WithSource(v string) syro.Logger {
	lg.Source = v
	return lg
}

func (lg *MongoLogger) WithEvent(v string) syro.Logger {
	lg.Event = v
	return lg
}

func (lg *MongoLogger) WithEventID(v string) syro.Logger {
	lg.EventID = v
	return lg
}

func (lg *MongoLogger) Log(level syro.LogLevel, msg string, lf ...syro.LogFields) error {
	log := syro.NewLog(level, msg, lg.Source, lg.Event, lg.EventID, lf...)

	// a custom set is defined because just using an InsertOne on the log
	// struct will break the _id field. omitempty does not work, if
	// the field has a string type.

	set := bson.M{
		"ts":    log.Timestamp,
		"level": log.Level,
		"msg":   log.Message,
	}

	if log.Source != "" {
		set["source"] = log.Source
	}

	if log.Event != "" {
		set["event"] = log.Event
	}

	if log.EventID != "" {
		set["event_id"] = log.EventID
	}

	if len(log.Fields) > 0 {
		set["fields"] = log.Fields
	}

	_, err := lg.Coll.InsertOne(context.Background(), set)

	// Log only if the
	// 	- settings are provided and console logging is not disabled
	//  - settings are not provided (assumes that should log)
	if lg.Settings != nil && !lg.Settings.DisableConsole {
		fmt.Print(log.String(lg))
	} else {
		if lg.Settings == nil {
			fmt.Print(log.String(lg))
		}
	}

	return err
}

func (lg *MongoLogger) LogExists(filter any) (bool, error) {
	if _, ok := filter.(bson.M); !ok {
		return false, errors.New("filter must have a bson.M type")
	}

	var log syro.Log
	if err := lg.Coll.FindOne(ctx, filter).Decode(&log); err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		return false, err
	}

	return !log.Timestamp.IsZero(), nil
}

func (lg *MongoLogger) Debug(msg string, lf ...syro.LogFields) error {
	return lg.Log(syro.DEBUG, msg, lf...)
}
func (lg *MongoLogger) Trace(msg string, lf ...syro.LogFields) error {
	return lg.Log(syro.TRACE, msg, lf...)
}
func (lg *MongoLogger) Error(msg string, lf ...syro.LogFields) error {
	return lg.Log(syro.ERROR, msg, lf...)
}
func (lg *MongoLogger) Info(msg string, lf ...syro.LogFields) error {
	return lg.Log(syro.INFO, msg, lf...)
}
func (lg *MongoLogger) Warn(msg string, lf ...syro.LogFields) error {
	return lg.Log(syro.WARN, msg, lf...)
}
func (lg *MongoLogger) Fatal(msg string, lf ...syro.LogFields) error {
	return lg.Log(syro.FATAL, msg, lf...)
}

// FindLogs returns logs that match the filter
func (lg *MongoLogger) FindLogs(filter syro.LogFilter, maxLimit int64) ([]syro.Log, error) {

	queryFilter := bson.M{}

	// if the from and to fields are not zero, add them to the query filter
	if !filter.From.IsZero() && !filter.To.IsZero() {
		if filter.From.After(filter.To) {
			return nil, errors.New("'from' date cannot be after 'to' date")
		}

		queryFilter["ts"] = bson.M{"$gte": filter.From, "$lte": filter.To}
	}

	level := filter.Level
	if level != nil && *level >= syro.TRACE && *level <= syro.FATAL {
		queryFilter["level"] = *level
	}

	if filter.Source != "" {
		queryFilter["source"] = filter.Source
	}

	if filter.Event != "" {
		queryFilter["event"] = filter.Event
	}

	if filter.EventID != "" {
		queryFilter["event_id"] = filter.EventID
	}

	userLimit := filter.TimeseriesFilter.Limit
	if userLimit > maxLimit {
		userLimit = maxLimit
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "ts", Value: -1}}). // sort by time field in descending order
		SetLimit(userLimit).
		SetSkip(filter.TimeseriesFilter.Skip)

	var docs []syro.Log
	err := mongoGetDocuments(lg.Coll, queryFilter, opts, &docs)
	return docs, err
}

// --------------- Cron Job Logic ---------------

// MongoStorage implementation of the Storage interface
type MongoCronStorage struct {
	cronListColl *mongo.Collection
}

func NewMongoCronStorage(cronListColl *mongo.Collection) (*MongoCronStorage, error) {
	if cronListColl == nil {
		return nil, fmt.Errorf("cron list collections cannot be nil")
	}

	return &MongoCronStorage{
		cronListColl: cronListColl,
	}, nil
}

func (m *MongoCronStorage) CreateIndexes() error {
	return NewMongoIndexes().Add("source", "name").Add("status").Create(m.cronListColl)
}

// TODO: refactor so that filter is a variadic parameter
func (m *MongoCronStorage) FindCronJobs() ([]syro.CronJob, error) {
	var docs []syro.CronJob
	err := mongoGetDocuments(m.cronListColl, bson.M{}, nil, &docs)
	return docs, err
}

// TODO: test this function + remember about the list of current jobs and the previous jobs which are not included in the list
func (m *MongoCronStorage) SetStatusForJobs(source string, status syro.JobStatus) error {
	filter := bson.M{"source": source}
	update := bson.M{"$set": bson.M{"status": status}}
	_, err := m.cronListColl.UpdateMany(ctx, filter, update)
	return err
}

// RegisterJob upserts the job name in the database based on the source
// and the job name. If the job does not exist, set the created_at
// field to the current time. If the job already exists,
// update the updated_at field to the current time.
func (m *MongoCronStorage) RegisterJob(source, name, sched, descr string, status syro.JobStatus, fnErr error) error {
	filter := bson.M{
		"source": source,
		"name":   name,
	}

	set := bson.M{
		"sched":      sched,
		"status":     status,
		"descr":      descr,
		"updated_at": time.Now().UTC(),
	}

	if fnErr != nil {
		set["exit_with_err"] = true
		set["error"] = fnErr.Error()
	} else {
		set["exit_with_err"] = false
		set["error"] = ""
	}

	if status == syro.JobStatusDone {
		set["finished_at"] = time.Now().UTC()
	}

	_, err := m.cronListColl.UpdateOne(ctx, filter, bson.M{
		"$set":         set,
		"$setOnInsert": bson.M{"created_at": time.Now().UTC()},
	}, mongoUpsertOpt)

	return err
}

// unexposed mongo specific utility function
func mongoGetDocuments[T any](coll *mongo.Collection, filter primitive.M, options *options.FindOptions, results *[]T) error {
	cur, err := coll.Find(ctx, filter, options)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	return cur.All(ctx, results)
}

var mongoUpsertOpt = options.Update().SetUpsert(true)

// mongoIndexBuilder is a helper for creating indexes for a MongoDB collection
// in a more reusable way.
type MongoIndexBuilder struct {
	indexes []mongo.IndexModel
}

func NewMongoIndexes() *MongoIndexBuilder { return &MongoIndexBuilder{} }

// Add adds a new index to the mongoIndexBuilder. It supports both single and compound
// indexes. All indexes are created in descending order.
func (ib *MongoIndexBuilder) Add(keys ...string) *MongoIndexBuilder {
	indexKeys := bson.D{}
	for _, key := range keys {
		indexKeys = append(indexKeys, bson.E{Key: key, Value: -1})
	}
	indexModel := mongo.IndexModel{Keys: indexKeys}
	ib.indexes = append(ib.indexes, indexModel)
	return ib
}

// Create creates all the indexes that have been added to the mongoIndexBuilder.
func (ib *MongoIndexBuilder) Create(coll *mongo.Collection) error {
	if ib == nil {
		return fmt.Errorf("ib is nil")
	}

	if coll == nil {
		return fmt.Errorf("coll is nil")
	}

	if len(ib.indexes) == 0 {
		return fmt.Errorf("no indexes to create")
	}

	if _, err := coll.Indexes().CreateMany(ctx, ib.indexes); err != nil {
		return fmt.Errorf("failed to create indexes for %v collection: %v", coll.Name(), err)
	}

	return nil
}
