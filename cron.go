package syro

import (
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// CronScheduler is a wrapper around the robfig/cron package that allows for the
// registration of jobs and the optional storage of job status and
// execution logs using the CronStorage interface.
type CronScheduler struct {
	cron    *cron.Cron          // cron is the cron CronScheduler which will run the jobs
	Source  string              // Source is used to identify the source of the job
	Jobs    []*Job              // Jobs is a list of all registered jobs
	storage CronStorage         // storage is an optional storage interface for the CronScheduler (unexported, so that can be accesed with a safe method)
	m       map[string]struct{} // store the name of the registered jobs in the map to check if the job with the same name has been registered
}

type CronStorage interface {
	// FindCronJobs returns a list of all registered jobs
	FindCronJobs() ([]CronJob, error)
	// RegisterJob registers the details of the selected job
	RegisterJob(source, name, sched, descr string, status JobStatus, err error) error
	// RegisterExecution registers the execution of a job if the storage is specified
	RegisterExecution(*CronExecLog) error
	// FindExecutions returns a list of job executions that match the filter
	FindExecutions(filter CronExecFilter, maxLimit int64) ([]CronExecLog, error)
	// SetJobsToInactive updates the status of the jobs for the given source. Useful when the app exits.
	// TODO: refactor to pass in the job status type, so that on startup a batch update can be done.
	SetJobsToInactive(source string) error
}

func NewCronScheduler(cron *cron.Cron, source string) *CronScheduler {
	return &CronScheduler{
		cron:   cron,
		Source: source,
		Jobs:   []*Job{},
		m:      make(map[string]struct{}),
	}
}

// WithStorage sets the storage for the CronScheduler.
func (s *CronScheduler) WithStorage(storage CronStorage) *CronScheduler {
	s.storage = storage
	return s
}

// NOTE: there is a slight inefficiency in the data that is written by
// the query because the (source, name, schedule, descr) params are
// written each time in order to update the status.
func (c *CronScheduler) updateJobStatus(j *Job, status JobStatus, err error) error {
	if c.storage != nil {
		return c.storage.RegisterJob(c.Source, j.Name, j.Schedule, j.Description, status, err)
	}
	return nil
}

func (c *CronScheduler) Storage() (CronStorage, error) {
	if c.storage == nil {
		return nil, fmt.Errorf("storage is nil")
	}

	return c.storage, nil
}

// Register adds a new job to the cron CronScheduler and wraps the job function with a
// mutex lock to prevent the execution of the job if it is already running.
// If a storage interface is provided, the job and job execution logs
// will be stored using it
func (c *CronScheduler) Register(j *Job) error {
	if j == nil {
		return fmt.Errorf("job cannot be nil")
	}

	if c.cron == nil {
		return fmt.Errorf("cron cannot be nil")
	}

	name := j.Name
	schedule := j.Schedule
	source := c.Source

	if schedule == "" {
		return fmt.Errorf("schedule has to be specified")
	}

	if name == "" {
		return fmt.Errorf("name has to be specified")
	}

	if j.Func == nil {
		return fmt.Errorf("job function cannot be nil")
	}

	// if the name of the job is already taken, return an error
	if _, exists := c.m[name]; !exists {
		c.m[name] = struct{}{}
	} else {
		return fmt.Errorf("job with the name of %v already is registered", name)
	}

	c.updateJobStatus(j, JobStatusInitialized, nil)

	joblock := newJobLock(func() {

		c.updateJobStatus(j, JobStatusRunning, nil) // TODO: what should be done with error?
		jobStartTime := time.Now()
		jobErr := j.Func()                          // Passed in job function which should be executed by the cron job
		c.updateJobStatus(j, JobStatusDone, jobErr) // TODO: what should be done with error?

		if j.OnComplete != nil {
			j.OnComplete(jobErr)
		}

		if c.storage != nil {
			c.storage.RegisterExecution(newCronExecutionLog(source, name, jobStartTime, jobErr))
		}

	}, name)

	if _, err := c.cron.AddJob(schedule, joblock); err != nil {
		return err
	}

	// Add the job to the list of registered jobs
	c.Jobs = append(c.Jobs, j)

	return nil
}

// Start the cron CronScheduler. Need to specify for how long
// the CronScheduler should run after calling this function
// (e.g. time.Sleep(1 * time.Hour) or forever)
//
// TODO: based on the source, the cron jobs which are not in the current list should be set to disbaled.
func (c *CronScheduler) Start() {
	c.cron.Start()
}

// Job represents a cron job that can be registered with the CronScheduler.
// TODO: test callbacks
// TODO: add a context input for callbacks? so that it would be possible to optionally cancel the job if it takes longer than x to run
// TODO: add retrys logic? + additional pause between them?
type Job struct {
	Source      string       // Source of the job (like the name of application which registered the job)
	Schedule    string       // Schedule of the job (e.g. "0 0 * * *" or "@every 1h")
	Name        string       // Name of the job
	Func        func() error // Function to be executed by the job
	Description string       // Optional. Description of the job
	OnComplete  func(error)  // Optional. Function to be executed when the job is completed.
}

// CronJob stores information about the registered job
type CronJob struct {
	// ID              string     `json:"_id" bson:"_id"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" bson:"updated_at"`
	FinishedAt  *time.Time `json:"finished_at" bson:"finished_at"`
	Source      string     `json:"source" bson:"source"`
	Name        string     `json:"name" bson:"name"`
	Status      string     `json:"status" bson:"status"`
	Schedule    string     `json:"sched" bson:"sched"`
	Description string     `json:"descr" bson:"descr"`
	Error       string     `json:"error" bson:"error"`
	ExitWithErr bool       `json:"exit_with_err" bson:"exit_with_err"`
}

// CronExecLog stores information about the job execution
// TODO: should this just be a syro.Log? source -> source, name -> event id, all other fields are field
type CronExecLog struct {
	Source        string        `json:"source" bson:"source"`
	Name          string        `json:"name" bson:"name"`
	InitializedAt time.Time     `json:"initialized_at" bson:"initialized_at"`
	FinishedAt    time.Time     `json:"finished_at" bson:"finished_at"`
	ExecutionTime time.Duration `json:"execution_time" bson:"execution_time"`
	Error         string        `json:"error" bson:"error"`
}

type CronExecFilter struct {
	TimeseriesFilter `json:"timeseries_filter" bson:"timeseries_filter"`
	Source           string        `json:"source" bson:"source"`
	Name             string        `json:"name" bson:"name"`
	ExecutionTime    time.Duration `json:"execution_time" bson:"execution_time"`
}

func newCronExecutionLog(source, name string, initializedAt time.Time, err error) *CronExecLog {
	log := &CronExecLog{
		Source:        source,
		Name:          name,
		InitializedAt: initializedAt,
		FinishedAt:    time.Now().UTC(),
		ExecutionTime: time.Since(initializedAt),
	}

	// Avoid panics if the error is nil
	if err != nil {
		log.Error = err.Error()
	}

	return log
}

type JobStatus string

const (
	JobStatusInitialized JobStatus = "initialized" // status set when the cron is added, but has not been run yet
	JobStatusRunning     JobStatus = "running"     // crons which are currently running
	JobStatusDone        JobStatus = "done"        // crons which are finished
	JobStatusInactive    JobStatus = "inactive"    // crons which are not running
	JobStatusRemoved     JobStatus = "removed"     // crons which are not present in the current list for the source
)

// jobLock is a mutex lock that prevents the execution of a job if it is already running.
type jobLock struct {
	fn   func()
	name string
	mu   sync.Mutex
}

func newJobLock(jobFunc func(), name string) *jobLock {
	return &jobLock{name: name, fn: jobFunc}
}

func (j *jobLock) Run() {
	if j.mu.TryLock() {
		defer j.mu.Unlock()
		j.fn()
	} else {
		fmt.Printf("job %v already running. Skipping...\n", j.name)
	}
}
