package lazy

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type dummyCloser struct {
	mu sync.Mutex
	v  int
}

func (d *dummyCloser) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.v = 0
	return nil
}

func TestLazyHolderRaceCondition(t *testing.T) {
	// Provider function for the lazyHolder.
	provider := func() (*dummyCloser, error) {
		time.Sleep(10 * time.Millisecond) // Simulate some delay
		return &dummyCloser{v: 42}, nil
	}

	// Test for race conditions
	holder := NewCloserHolder(provider)

	var wg sync.WaitGroup

	// Concurrently call Get.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := holder.Get()
			if err != nil {
				t.Errorf("Get() error: %v", err)
			}
			if v == nil {
				t.Errorf("Get() returned nil value")
			}
		}()
	}

	// Concurrently call Reset.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := holder.Reset()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Errorf("Reset() error: %v", err)
			}
		}()
	}

	wg.Wait()
}
