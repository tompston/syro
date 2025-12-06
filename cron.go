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
	storage    CronStorage         // storage is an optional storage interface for the CronScheduler (unexported, so that can be accesed with a safe method)
	execLogger Logger              // logger for the cron executions (time, duration, error)
	cron       *cron.Cron          // cron is the cron CronScheduler which will run the jobs
	m          map[string]struct{} // internal map of the registered job names to check if the job with the same name has been registered
	Source     string              // Source is used to identify the source of the job
	Jobs       []*Job              // Jobs is a list of all registered jobs
}

type CronStorage interface {
	// FindCronJobs returns a list of all registered jobs
	FindCronJobs() ([]CronJob, error)
	// RegisterJob registers the details of the selected job
	RegisterJob(source, name, sched, descr string, status JobStatus, err error) error
	// SetStatusForJobs updates the status of the jobs for the given source
	SetStatusForJobs(source string, status JobStatus) error
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
	if storage != nil {
		s.storage = storage
	}
	return s
}

// WithExecLogger will use the passed in logger to log the executions of the cron job
// as structured logs, setting the source of the log to source of the scheduler,
// event as the name of the cron and populate the log fields with
// `time_init`, `time_finish` and `exec_dur` for further info.
// On successful execution, the log will have thestatus as
// info. If the passed in function returns an error,
// then it will be logged as an error log.
func (s *CronScheduler) WithExecLogger(l Logger) *CronScheduler {
	if l != nil {
		s.execLogger = l
	}
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

	schedule := j.Schedule
	name := j.Name

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

	// c.updateJobStatus(j, JobStatusInitialized, nil) // done in the Start method, so that there is one batch update query

	joblock := newJobLock(func() {

		c.updateJobStatus(j, JobStatusRunning, nil) // TODO: what should be done with error?
		jobStartTime := time.Now()
		jobErr := j.Func()                          // Passed in job function which should be executed by the cron job
		c.updateJobStatus(j, JobStatusDone, jobErr) // TODO: what should be done with error?

		if j.OnComplete != nil {
			j.OnComplete(jobErr)
		}

		if c.execLogger != nil {
			lg := c.execLogger.WithSource(c.Source).WithEvent(j.Name)

			fields := LogFields{
				"time_init":   jobStartTime,
				"time_finish": time.Now().UTC(),
				"exec_dur":    time.Since(jobStartTime),
			}

			if jobErr != nil {
				lg.Error(jobErr.Error(), fields)
			} else {
				lg.Info("done", fields)
			}
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
func (c *CronScheduler) Start() {
	if c.storage != nil {
		c.storage.SetStatusForJobs(c.Source, JobStatusInitialized)
	}

	c.cron.Start()
}

// Job represents a cron job that can be registered with the CronScheduler.
// TODO: add a context input for callbacks? so that it would be possible to optionally cancel the job if it takes longer than x to run
// TODO: add retry logic? + additional pause between them?
type Job struct {
	Func        func() error // Function to be executed by the job
	OnComplete  func(error)  // Optional. Function to be executed when the job is completed.
	Schedule    string       // Schedule of the job (e.g. "0 0 * * *" or "@every 1h")
	Name        string       // Name of the job
	Description string       // Optional. Description of the job
}

// CronJob stores information about the registered job
type CronJob struct {
	ID          string     `json:"_id" bson:"_id"`
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

type JobStatus string

const (
	JobStatusInitialized JobStatus = "init"     // status set when the cron is added, but has not been run yet
	JobStatusRunning     JobStatus = "running"  // crons which are currently running
	JobStatusDone        JobStatus = "done"     // crons which are finished
	JobStatusInactive    JobStatus = "inactive" // crons which are not running
	JobStatusRemoved     JobStatus = "removed"  // crons which are not present in the current list for the source
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
