package syro

import "sync"

// Atomic provides concurrent-safe access to a value of any type
type Atomic[T any] struct {
	value T
	mu    sync.Mutex
}

// NewAtomic creates a new Atomic with an optional initial value
func NewAtomic[T any](val T) *Atomic[T] {
	return &Atomic[T]{value: val}
}

// Set safely updates the value
func (a *Atomic[T]) Set(v T) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.value = v
}

// Get safely retrieves the value
func (a *Atomic[T]) Get() T {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.value
}

// ErrorHandlingOption defines how to handle errors
type ErrorHandlingOption int

const (
	ExecFuncBreakOnFirstError    ErrorHandlingOption = 1
	ExecFuncCollectAllErrors     ErrorHandlingOption = 2
	ExecFuncIgnoreAllErrors      ErrorHandlingOption = 3
	ExecFuncReturnNoneIfAnyError ErrorHandlingOption = 4
)

// ExecFuncs runs an array of functions with a defined parallelism and error-handling strategy
func ExecFuncs(funcs []func() error, parallelism int, handling ErrorHandlingOption) []error {
	if parallelism <= 0 {
		parallelism = 1
	}

	var (
		wg       sync.WaitGroup
		sem      = make(chan struct{}, parallelism)
		errsLock sync.Mutex
		errs     []error
		once     sync.Once
		stopChan = make(chan struct{})
	)

	for _, fn := range funcs {
		select {
		case <-stopChan:
			continue
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(f func() error) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := f(); err != nil {
				switch handling {
				case ExecFuncBreakOnFirstError:
					once.Do(func() { close(stopChan) })
					errsLock.Lock()
					errs = append(errs, err)
					errsLock.Unlock()

				case ExecFuncCollectAllErrors:
					errsLock.Lock()
					errs = append(errs, err)
					errsLock.Unlock()

				case ExecFuncReturnNoneIfAnyError:
					once.Do(func() { close(stopChan) })
					errsLock.Lock()
					errs = append(errs, err)
					errsLock.Unlock()

				case ExecFuncIgnoreAllErrors:
					// Do nothing
				}
			}
		}(fn)
	}

	wg.Wait()

	if handling == ExecFuncReturnNoneIfAnyError && len(errs) > 0 {
		return nil
	}

	return errs
}

type Num interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// SafeNum is a concurrency-safe struct that holds a numeric quantity
type SafeNum[T Num] struct {
	mu sync.RWMutex
	v  T
}

// NewSafeNum creates concurrency safe number
func NewSafeNum[T Num](initialQty T) *SafeNum[T] {
	return &SafeNum[T]{
		v: initialQty,
	}
}

// Get returns the current quantity (read-lock)
func (n *SafeNum[T]) Get() T {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.v
}

// Set updates the value to a new value (write-lock)
func (n *SafeNum[T]) Set(v T) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.v = v
}

// Add adds a delta to the current value (write-lock)
func (n *SafeNum[T]) Add(delta T) T {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.v += delta
	return n.v
}
