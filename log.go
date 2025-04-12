package syro

import (
	"fmt"
	"strings"
	"time"
)

// Logger interface implements the methods for logging
type Logger interface {
	Error(msg string, lf ...LogFields) error
	Info(msg string, lf ...LogFields) error
	Debug(msg string, lf ...LogFields) error
	Warn(msg string, lf ...LogFields) error
	Trace(msg string, lf ...LogFields) error
	Fatal(msg string, lf ...LogFields) error

	GetTableName() string                     // GetTableName returns the name of the table where the logs are stored
	FindLogs(filter LogFilter) ([]Log, error) // FindLogs returns the logs that match the provided filter
	LogExists(filter any) (bool, error)       // LogExists checks if the log with the provided filter exists.
	GetProps() LoggerProps                    // GetProps returns the properties of the logger
	SetSource(v string) Logger                // SetSource sets the source of the log
	SetEvent(v string) Logger                 // SetEvent sets the event of the log
	SetEventID(v string) Logger               // SetEventID sets the event id of the log
}

// Log struct for storing the log data. Event, EventID, and Fields are optional.
type Log struct {
	ID      string    `json:"_id" bson:"_id"`                               // (not logged to the console)
	Time    time.Time `json:"time" bson:"time"`                             // Time of the log (UTC)
	Level   LogLevel  `json:"level" bson:"level"`                           // Log level
	Message string    `json:"message" bson:"message"`                       // Logged message
	Source  string    `json:"source,omitempty" bson:"source,omitempty"`     // Source of the log (api, pooler, etc.)
	Event   string    `json:"event,omitempty" bson:"event,omitempty"`       // Event of the log (api-auth-request, binance-eth-pooler, etc.)
	EventID string    `json:"event_id,omitempty" bson:"event_id,omitempty"` // (not logged to the console)
	Fields  LogFields `json:"fields,omitempty" bson:"fields,omitempty"`     // Optional fields
}

type LogFilter struct {
	TimeseriesFilter `json:"timeseries_filter"`
	Source           string    `json:"source"`
	Event            string    `json:"event"`
	EventID          string    `json:"event_id"`
	Level            *LogLevel `json:"level"`
}

type LogLevel int8

const (
	TRACE LogLevel = 0
	DEBUG LogLevel = 1
	INFO  LogLevel = 2
	WARN  LogLevel = 3
	ERROR LogLevel = 4
	FATAL LogLevel = 5
)

func (l LogLevel) String() string {
	switch l {
	case ERROR:
		return "error"
	case INFO:
		return "info"
	case DEBUG:
		return "debug"
	case WARN:
		return "warn"
	case TRACE:
		return "trace"
	case FATAL:
		return "fatal"
	default:
		return "unknown"
	}
}

type LogFields map[string]interface{}

func newLog(level LogLevel, msg, source, event, eventID string, fields ...LogFields) Log {
	log := Log{
		Time:    time.Now().UTC(),
		Level:   level,
		Message: msg,
		Source:  source,
		Event:   event,
		EventID: eventID,
	}

	if len(fields) == 1 {
		log.Fields = fields[0]
	}

	return log
}

// String method converts the log to a string, using the provided logger settings.
func (log Log) String(logger Logger) string {
	// Use the default settings by default if the settings are not correct
	settings := DefaultLoggerSettings

	// if the logger is not nil and has it has settings with a defined location, use them
	if logger != nil {
		props := logger.GetProps()

		if props.Settings != nil && props.Settings.Location != nil {
			settings = props.Settings
		}
	}

	var b strings.Builder

	b.WriteString(log.Time.In(settings.Location).Format(settings.TimeFormat))
	b.WriteString("  ")
	b.WriteString(fmt.Sprintf("%-6s", log.Level.String()))
	b.WriteString("  ")
	b.WriteString(fmt.Sprintf("%-12s", log.Source))
	b.WriteString(fmt.Sprintf("%-12s", log.Event))
	b.WriteString("  ")
	b.WriteString(log.Message)

	if log.Fields != nil {
		for k, v := range log.Fields {
			b.WriteString(" ")
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(fmt.Sprintf("%v", v))
		}
	}

	b.WriteString("\n")

	return b.String()
}

// LoggerSettings struct for storing the settings for the logger which are
// used when printing the log to the console.
type LoggerSettings struct {
	Location   *time.Location
	TimeFormat string
}

// DefaultLoggerSettings are the default settings for the logger, used if the
// settings are not provided or location is nil.
var DefaultLoggerSettings = &LoggerSettings{
	Location:   time.UTC,
	TimeFormat: "2006-01-02 15:04:05",
}

type LoggerProps struct {
	Settings *LoggerSettings
	Source   string
	Event    string
	EventID  string
}

// ----------- Logger implementation for console -----------

type ConsoleLogger struct {
	Settings *LoggerSettings
	Source   string
	Event    string
	EventID  string
}

func NewConsoleLogger(s *LoggerSettings) *ConsoleLogger { return &ConsoleLogger{Settings: s} }
func (lg *ConsoleLogger) GetProps() LoggerProps {
	return LoggerProps{
		Settings: lg.Settings,
		Source:   lg.Source,
		Event:    lg.Event,
		EventID:  lg.EventID,
	}
}

func (lg *ConsoleLogger) GetTableName() string { return "" }

func (lg *ConsoleLogger) log(level LogLevel, msg string, lf ...LogFields) error {
	log := newLog(level, msg, lg.Source, lg.Event, lg.EventID, lf...)
	_, err := fmt.Print(log.String(lg))
	return err
}

func (lg *ConsoleLogger) SetSource(v string) Logger {
	lg.Source = v
	return lg
}

func (lg *ConsoleLogger) SetEvent(v string) Logger {
	lg.Event = v
	return lg
}

func (lg *ConsoleLogger) SetEventID(v string) Logger {
	lg.EventID = v
	return lg
}

func (lg *ConsoleLogger) Debug(msg string, lf ...LogFields) error { return lg.log(DEBUG, msg, lf...) }
func (lg *ConsoleLogger) Trace(msg string, lf ...LogFields) error { return lg.log(TRACE, msg, lf...) }
func (lg *ConsoleLogger) Error(msg string, lf ...LogFields) error { return lg.log(ERROR, msg, lf...) }
func (lg *ConsoleLogger) Info(msg string, lf ...LogFields) error  { return lg.log(INFO, msg, lf...) }
func (lg *ConsoleLogger) Warn(msg string, lf ...LogFields) error  { return lg.log(WARN, msg, lf...) }
func (lg *ConsoleLogger) Fatal(msg string, lf ...LogFields) error { return lg.log(FATAL, msg, lf...) }

func (lg *ConsoleLogger) LogExists(filter any) (bool, error) {
	return false, fmt.Errorf("method cannot be used with ConsoleLogger")
}

func (lg *ConsoleLogger) FindLogs(filter LogFilter) ([]Log, error) {
	return nil, fmt.Errorf("method cannot be used with ConsoleLogger")
}
