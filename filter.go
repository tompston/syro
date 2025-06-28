package syro

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type TimeseriesFilter struct {
	From  time.Time
	To    time.Time
	Limit int64
	Skip  int64
}

// parse the to, from, limit and skip parameters from the URL, if they exist and are valid values.
func parseUrlToTimeseriesParams(vals url.Values) (*TimeseriesFilter, error) {
	filter := TimeseriesFilter{}

	// Parse "from" time
	if from := vals.Get("from"); from != "" {
		parsedFrom, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return nil, fmt.Errorf("invalid 'from' time format: %v", err)
		}

		filter.From = parsedFrom
	}

	// Parse "to" time
	if to := vals.Get("to"); to != "" {
		parsedTo, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return nil, fmt.Errorf("invalid 'to' time format: %v", err)
		}

		filter.To = parsedTo
	}

	// Parse "limit"
	if limit := vals.Get("limit"); limit != "" {
		parsedLimit, err := strconv.ParseInt(limit, 10, 64)
		if err != nil || parsedLimit < 0 {
			return nil, errors.New("invalid 'limit' value")
		}

		filter.Limit = parsedLimit
	}

	// Parse "skip"
	if skip := vals.Get("skip"); skip != "" {
		parsedSkip, err := strconv.ParseInt(skip, 10, 64)
		if err != nil || parsedSkip < 0 {
			return nil, errors.New("invalid 'skip' value")
		}

		filter.Skip = parsedSkip
	}

	return &filter, nil
}

func QueryLogs(l Logger, maxLimit int64, urlPath string) ([]Log, error) {
	if l == nil {
		return nil, errors.New("logger is nil")
	}

	parsedURL, err := url.Parse(urlPath)
	if err != nil {
		return nil, errors.New("failed to parse URL")
	}

	// Extract query parameters
	params := parsedURL.Query()
	filter := LogFilter{}

	ts, err := parseUrlToTimeseriesParams(params)
	if err != nil {
		return nil, err
	}

	filter.TimeseriesFilter = *ts
	filter.Source = params.Get("source")
	filter.Event = params.Get("event")
	filter.EventID = params.Get("event_id")

	if parsedLevel, err := strconv.Atoi(params.Get("level")); err == nil {
		logLevel := LogLevel(parsedLevel)
		filter.Level = &logLevel
	}

	return l.FindLogs(filter, maxLimit)
}

func QueryCrons(s CronStorage, urlPath string) ([]CronJob, error) {
	if s == nil {
		return nil, errors.New("storage is nil")
	}

	return s.FindCronJobs()
}

func QueryCronExecutions(s CronStorage, urlPath string) ([]CronExecLog, error) {
	if s == nil {
		return nil, errors.New("storage is nil")
	}

	parsedURL, err := url.Parse(urlPath)
	if err != nil {
		return nil, errors.New("failed to parse URL")
	}

	// Extract query parameters
	params := parsedURL.Query()

	filter := CronExecFilter{}
	ts, err := parseUrlToTimeseriesParams(params)
	if err != nil {
		return nil, err
	}

	filter.TimeseriesFilter = *ts
	filter.Source = params.Get("source")
	filter.Name = params.Get("name")

	return s.FindExecutions(filter)
}
