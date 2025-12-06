package syro

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ctx = context.Background()

type Logger interface {
	Log(level LogLevel, msg string, lf ...LogFields) error // Log method to log a message with a specific level and optional fields
	Error(msg string, lf ...LogFields) error
	Info(msg string, lf ...LogFields) error
	Debug(msg string, lf ...LogFields) error
	Warn(msg string, lf ...LogFields) error
	Trace(msg string, lf ...LogFields) error
	Fatal(msg string, lf ...LogFields) error

	Name() string                                             // Util method to figure out which implementation is used
	GetTableName() string                                     // GetTableName returns the name of the table where the logs are stored
	FindLogs(filter LogFilter, maxLimit int64) ([]Log, error) // FindLogs returns the logs that match the provided filter
	LogExists(filter any) (bool, error)                       // LogExists checks if the log with the provided filter exists.
	GetProps() LoggerProps                                    // GetProps returns the properties of the logger
	WithSource(v string) Logger                               // WithSource sets the source of the log
	WithEvent(v string) Logger                                // WithEvent sets the event of the log
	WithEventID(v string) Logger                              // WithEventID sets the event id of the log
}

type Log struct {
	Timestamp time.Time `json:"ts" bson:"ts"`                                 // Time of the log (UTC)
	Fields    LogFields `json:"fields,omitempty" bson:"fields,omitempty"`     // Optional fields
	ID        string    `json:"_id" bson:"_id"`                               // (not logged to the console)
	Message   string    `json:"msg" bson:"msg"`                               // Logged message
	Source    string    `json:"source,omitempty" bson:"source,omitempty"`     // Source of the log (api, pooler, etc.)
	Event     string    `json:"event,omitempty" bson:"event,omitempty"`       // Event of the log (api-auth-request, binance-eth-pooler, etc.)
	EventID   string    `json:"event_id,omitempty" bson:"event_id,omitempty"` // (not logged to the console)
	Level     LogLevel  `json:"level" bson:"level"`                           // Log level
}

type LogFields map[string]any

type LogLevel int

const (
	// Log levels start from 1, because of the default value problem.
	TRACE LogLevel = 1
	DEBUG LogLevel = 2
	INFO  LogLevel = 3
	WARN  LogLevel = 4
	ERROR LogLevel = 5
	FATAL LogLevel = 6
)

// Fixed set of log levels
var LogLevels = [...]LogLevel{TRACE, DEBUG, INFO, WARN, ERROR, FATAL}

// LoggerSettings struct for storing the settings for the logger which are
// used when printing the log to the console.
type LoggerSettings struct {
	Location   *time.Location
	TimeFormat string
}

const defaultTimeFormat = "2006-01-02 15:04:05"

// LoggerSettingsDefault are the default settings for the logger, used if the
// settings are not provided or location is nil.
var LoggerSettingsDefault = &LoggerSettings{
	Location:   time.UTC,
	TimeFormat: defaultTimeFormat,
	// TODO: optional disable for console?
}

type LogFilter struct {
	Level            *LogLevel `json:"level"`
	Source           string    `json:"source"`
	Event            string    `json:"event"`
	EventID          string    `json:"event_id"`
	TimeseriesFilter `json:"timeseries_filter"`
}

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

func NewLog(level LogLevel, msg, source, event, eventID string, fields ...LogFields) Log {
	log := Log{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   msg,
		Source:    source,
		Event:     event,
		EventID:   eventID,
	}

	if len(fields) == 1 {
		log.Fields = fields[0]
	}

	return log
}

// String method converts the log to a string, using the provided logger settings.
func (log Log) String(logger Logger) string {
	// Use the default settings by default if the settings are not correct
	settings := LoggerSettingsDefault

	// if the logger is not nil and has it has settings with a defined location, use them
	if logger != nil {
		props := logger.GetProps()

		if props.Settings != nil && props.Settings.Location != nil {
			settings = props.Settings
		}
	}

	var b strings.Builder
	b.Grow(256) // Pre-allocate reasonable capacity

	timeformat := settings.TimeFormat
	if timeformat == "" {
		timeformat = defaultTimeFormat
	}

	b.WriteString(log.Timestamp.In(settings.Location).Format(timeformat))
	b.WriteString("  ")
	b.WriteString(fmt.Sprintf("%-6s", log.Level.String()))
	b.WriteString("  ")
	b.WriteString(fmt.Sprintf("%-12s", log.Source))
	b.WriteString(fmt.Sprintf("%-12s", log.Event))
	b.WriteString("  ")
	b.WriteString(log.Message)

	// if log.Fields != nil {
	// 	b.WriteString("  ")
	// 	for k, v := range log.Fields {
	// 		b.WriteString(" ")
	// 		b.WriteString(k)
	// 		b.WriteString("=")
	// 		b.WriteString(fmt.Sprintf("%v", v))
	// 	}
	// }

	// Faster than the upper no-reflection logic
	if len(log.Fields) > 0 {
		b.WriteString("  ")

		for k, v := range log.Fields {
			b.WriteByte(' ')
			b.WriteString(k)
			b.WriteByte('=')

			switch val := v.(type) {
			case string:
				b.WriteString(val)
			case int:
				b.WriteString(strconv.Itoa(val))
			case int64:
				b.WriteString(strconv.FormatInt(val, 10))
			case float64:
				b.WriteString(strconv.FormatFloat(val, 'f', -1, 64))
			case bool:
				b.WriteString(strconv.FormatBool(val))
			default:
				b.WriteString(fmt.Sprintf("%v", v))
			}
		}
	}

	b.WriteString("\n")

	return b.String()
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

func (lg *ConsoleLogger) Name() string { return "console" }

func (lg *ConsoleLogger) GetTableName() string { return "" }

func (lg *ConsoleLogger) Log(level LogLevel, msg string, lf ...LogFields) error {
	log := NewLog(level, msg, lg.Source, lg.Event, lg.EventID, lf...)
	_, err := fmt.Print(log.String(lg))
	return err
}

func (lg *ConsoleLogger) WithSource(v string) Logger {
	lg.Source = v
	return lg
}

func (lg *ConsoleLogger) WithEvent(v string) Logger {
	lg.Event = v
	return lg
}

func (lg *ConsoleLogger) WithEventID(v string) Logger {
	lg.EventID = v
	return lg
}

func (lg *ConsoleLogger) Debug(msg string, lf ...LogFields) error { return lg.Log(DEBUG, msg, lf...) }
func (lg *ConsoleLogger) Trace(msg string, lf ...LogFields) error { return lg.Log(TRACE, msg, lf...) }
func (lg *ConsoleLogger) Error(msg string, lf ...LogFields) error { return lg.Log(ERROR, msg, lf...) }
func (lg *ConsoleLogger) Info(msg string, lf ...LogFields) error  { return lg.Log(INFO, msg, lf...) }
func (lg *ConsoleLogger) Warn(msg string, lf ...LogFields) error  { return lg.Log(WARN, msg, lf...) }
func (lg *ConsoleLogger) Fatal(msg string, lf ...LogFields) error { return lg.Log(FATAL, msg, lf...) }

func (lg *ConsoleLogger) LogExists(filter any) (bool, error) {
	return false, fmt.Errorf("method cannot be used with ConsoleLogger")
}

func (lg *ConsoleLogger) FindLogs(filter LogFilter, maxLimit int64) ([]Log, error) {
	return nil, fmt.Errorf("method cannot be used with ConsoleLogger")
}
