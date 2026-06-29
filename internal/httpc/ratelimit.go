package httpc

import "time"

// Ratelimit is a token bucket: it starts full and refills one token every
// refillSeconds/capacity seconds, exactly like the original RatelimitBucket.
type Ratelimit struct {
	tokens chan struct{}
	stop   chan struct{}
}

func NewRatelimit(capacity, refillSeconds int) *Ratelimit {
	if capacity <= 0 || refillSeconds <= 0 {
		return nil
	}
	r := &Ratelimit{
		tokens: make(chan struct{}, capacity),
		stop:   make(chan struct{}),
	}
	for i := 0; i < capacity; i++ {
		r.tokens <- struct{}{}
	}
	fillDelay := time.Duration(float64(time.Second) * float64(refillSeconds) / float64(capacity))
	go func() {
		t := time.NewTicker(fillDelay)
		defer t.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-t.C:
				select {
				case r.tokens <- struct{}{}: // add a token if bucket not full
				default:
				}
			}
		}
	}()
	return r
}

// Wait blocks until a token is available and consumes it.
func (r *Ratelimit) Wait() {
	if r == nil {
		return
	}
	<-r.tokens
}

// Stop halts the refill goroutine.
func (r *Ratelimit) Stop() {
	if r == nil {
		return
	}
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}
