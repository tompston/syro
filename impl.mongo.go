package syro

import (
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoLogger struct {
	Coll     *mongo.Collection
	Settings *LoggerSettings
	Source   string
	Event    string
	EventID  string
}

func NewMongoLogger(coll *mongo.Collection, settings *LoggerSettings) *MongoLogger {
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

func (lg *MongoLogger) GetProps() LoggerProps {
	return LoggerProps{
		Settings: lg.Settings,
		Source:   lg.Source,
		Event:    lg.Event,
		EventID:  lg.EventID,
	}
}

func (lg *MongoLogger) Name() string {
	return "mongo"
}

func (lg *MongoLogger) WithSource(v string) Logger {
	lg.Source = v
	return lg
}

func (lg *MongoLogger) WithEvent(v string) Logger {
	lg.Event = v
	return lg
}

func (lg *MongoLogger) WithEventID(v string) Logger {
	lg.EventID = v
	return lg
}

func (lg *MongoLogger) Log(level LogLevel, msg string, lf ...LogFields) error {
	log := NewLog(level, msg, lg.Source, lg.Event, lg.EventID, lf...)

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

	_, err := lg.Coll.InsertOne(ctx, set)
	fmt.Print(log.String(lg))
	return err
}

func (lg *MongoLogger) LogExists(filter any) (bool, error) {
	if _, ok := filter.(bson.M); !ok {
		return false, errors.New("filter must have a bson.M type")
	}

	var log Log
	if err := lg.Coll.FindOne(ctx, filter).Decode(&log); err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		return false, err
	}

	return !log.Timestamp.IsZero(), nil
}

func (lg *MongoLogger) Debug(msg string, lf ...LogFields) error { return lg.Log(DEBUG, msg, lf...) }
func (lg *MongoLogger) Trace(msg string, lf ...LogFields) error { return lg.Log(TRACE, msg, lf...) }
func (lg *MongoLogger) Error(msg string, lf ...LogFields) error { return lg.Log(ERROR, msg, lf...) }
func (lg *MongoLogger) Info(msg string, lf ...LogFields) error  { return lg.Log(INFO, msg, lf...) }
func (lg *MongoLogger) Warn(msg string, lf ...LogFields) error  { return lg.Log(WARN, msg, lf...) }
func (lg *MongoLogger) Fatal(msg string, lf ...LogFields) error { return lg.Log(FATAL, msg, lf...) }

// FindLogs returns logs that match the filter
func (lg *MongoLogger) FindLogs(filter LogFilter, maxLimit int64) ([]Log, error) {

	queryFilter := bson.M{}

	// if the from and to fields are not zero, add them to the query filter
	if !filter.From.IsZero() && !filter.To.IsZero() {
		if filter.From.After(filter.To) {
			return nil, errors.New("'from' date cannot be after 'to' date")
		}

		queryFilter["ts"] = bson.M{"$gte": filter.From, "$lte": filter.To}
	}

	level := filter.Level
	if level != nil && *level >= TRACE && *level <= FATAL {
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

	var docs []Log
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
func (m *MongoCronStorage) FindCronJobs() ([]CronJob, error) {
	var docs []CronJob
	err := mongoGetDocuments(m.cronListColl, bson.M{}, nil, &docs)
	return docs, err
}

// TODO: test this function + remember about the list of current jobs and the previous jobs which are not included in the list
func (m *MongoCronStorage) SetStatusForJobs(source string, status JobStatus) error {
	filter := bson.M{"source": source}
	update := bson.M{"$set": bson.M{"status": status}}
	_, err := m.cronListColl.UpdateMany(ctx, filter, update)
	return err
}

// RegisterJob upserts the job name in the database based on the source
// and the job name. If the job does not exist, set the created_at
// field to the current time. If the job already exists,
// update the updated_at field to the current time.
func (m *MongoCronStorage) RegisterJob(source, name, sched, descr string, status JobStatus, fnErr error) error {
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

	if status == JobStatusDone {
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
