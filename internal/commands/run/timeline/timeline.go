package timeline

import (
	"context"
	"sync"
)

type Controller struct {
	shutdownSignal chan struct{}
	wg             sync.WaitGroup
}

func NewController() *Controller {
	return &Controller{
		shutdownSignal: make(chan struct{}),
		wg:             sync.WaitGroup{},
	}
}

func (c *Controller) Start() error {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		select {
		case <-c.shutdownSignal:
			return
		}
	}()

	return nil
}

func (c *Controller) Shutdown(ctx context.Context) error {
	// close channel to notify goroutine it's time to exit
	close(c.shutdownSignal)

	// wait for goroutine to finish
	c.wg.Wait()

	return nil
}
