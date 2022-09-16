package shell

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

//
// The output is buffered in the outputBuffer as it comes in from the TTY
// device.
//
// Ideally, we should strive to flush the output to the logfile as an event
// when there are enough data to be sent. "Enough data" in this context
// should satisfy the following criteria:
//
// - If there is more than 100 characters in the buffer
//
// - If there is less than 100 characters in the buffer, but they were in
//   the buffer for more than 100 milliseconds.
//
// - If the UTF-8 sequence is complete. Cutting the UTF-8 sequence in half
//   leads to undefined (?) characters in the UI.
//

const OutputBufferMaxTimeSinceLastAppend = 100 * time.Millisecond
const OutputBufferDefaultCutLength = 100

type OutputBuffer struct {
	Consumer   func(string)
	bytes      []byte
	mu         sync.Mutex
	done       bool
	lastAppend *time.Time
}

func NewOutputBuffer(consumer func(string)) (*OutputBuffer, error) {
	if consumer == nil {
		return nil, fmt.Errorf("output buffer requires a consumer")
	}

	b := &OutputBuffer{
		Consumer: consumer,
		bytes:    []byte{},
	}

	go b.Flush()

	return b, nil
}

func (b *OutputBuffer) Append(bytes []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastAppend = &now
	b.bytes = append(b.bytes, bytes...)
}

func (b *OutputBuffer) IsEmpty() bool {
	return len(b.bytes) == 0
}

func (b *OutputBuffer) Flush() {
	backoffStrategy := b.exponentialBackoff()

	for {
		if b.done {
			log.Debugf("The output buffer was closed - stopping")
			break
		}

		/*
		 * The exponential backoff strategy for ticks is only used
		 * when the buffer is empty, to make sure we don't continuously
		 * check the buffer if it has been empty for a while.
		 */
		if b.IsEmpty() {
			delay := backoffStrategy.NextBackOff()
			log.Debugf("Empty buffer - waiting %v until next tick", delay)
			time.Sleep(delay)
			continue
		}

		b.flush()
		backoffStrategy.Reset()
	}
}

func (b *OutputBuffer) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()

	timeSinceLastAppend := b.timeSinceLastAppend()

	/*
	 * If there's recent, but not enough data in the buffer, we don't yet flush.
	 * Here, we don't want to use the exponential backoff strategy while waiting,
	 * because we should respect the maximum of 100ms for data in the buffer.
	 */
	if len(b.bytes) < OutputBufferDefaultCutLength && timeSinceLastAppend < OutputBufferMaxTimeSinceLastAppend {
		log.Debugf("The output buffer has only %d bytes and the flush was %v ago - waiting...", len(b.bytes), timeSinceLastAppend)
		time.Sleep(10 * time.Millisecond)
		return
	}

	log.Debugf("%d bytes in the buffer - flushing...", len(b.bytes))

	/*
	 * First we determine how much to cut.
	 *
	 * We don't want to flush too much in any iteration, but neither we want to
	 * flush too little.
	 *
	 * Starting from the default cut lenght, and decreasing the lenght until we
	 * are ready to flush.
	 */
	cutLength := OutputBufferDefaultCutLength

	// We can't cut more than we have in the buffer.
	if len(b.bytes) < cutLength {
		cutLength = len(b.bytes)
	}

	/*
	 * Now comes the tricky part.
	 *
	 * We don't want to cut in the middle of an UTF-8 sequence.
	 *
	 * In the loop below, we are cutting off the last 3 charactes in case
	 * they are marked as the unicode continuation characters,
	 * since an unicode sequence can't be longer than 4 bytes.
	 *
	 * We do this unicode-based adjustment in two scenarios:
	 *   1 - The output buffer was not yet closed.
	 *       In this case, we always check for incomplete UTF-8 sequences,
	 *       because the buffer might not yet received them from the TTY.
	 *   2 - The output buffer was closed, but the data in there doesn't fit
	 *       in one chunk. Since in this case we know that no more bytes are coming
	 *       from the TTY, the only reason we'd have an incomplete UTF-8 sequence
	 *       is because we are cutting it here to fit it into the chunk.
	 */

	if !b.done || (b.done && cutLength == OutputBufferDefaultCutLength) {
		for i := 0; i < 4; i++ {
			if utf8.Valid(b.bytes[0:cutLength]) {
				break
			} else {
				cutLength--
			}
		}
	}

	if cutLength <= 0 {
		return
	}

	bytes := make([]byte, cutLength)
	copy(bytes, b.bytes[0:cutLength])
	b.bytes = b.bytes[cutLength:]

	// Make sure we normalize newline sequences, and flush the output to the consumer.
	output := strings.Replace(string(bytes), "\r\n", "\n", -1)
	log.Debugf("%d bytes flushed: %s", len(bytes), output)
	b.Consumer(output)
}

func (b *OutputBuffer) exponentialBackoff() *backoff.ExponentialBackOff {
	e := backoff.NewExponentialBackOff()

	/*
	 * We start with a 10ms interval between ticks,
	 * but if there’s nothing in the buffer for a while,
	 * 10ms is too little an interval.
	 */
	e.InitialInterval = 10 * time.Millisecond

	/*
	 * If there's no data in the buffer, we increase the delay.
	 * But, we also cap that delay to 1s, to make sure we don’t go too
	 * long without checking if a command goes too long without producing any output.
	 */
	e.MaxInterval = time.Second

	// We don't ever want the strategy to return backoff.Stop, so we don't set this.
	e.MaxElapsedTime = 0

	// We need to call Reset() before using it.
	e.Reset()

	return e
}

func (b *OutputBuffer) timeSinceLastAppend() time.Duration {
	if b.lastAppend != nil {
		return time.Since(*b.lastAppend)
	}

	return time.Millisecond
}

func (b *OutputBuffer) Close() {
	b.done = true

	// wait until buffer is empty, for at most 1s.
	log.Debugf("Waiting for buffer to be completely flushed...")
	err := retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "wait for all output to be flushed",
		MaxAttempts:          100,
		DelayBetweenAttempts: 10 * time.Millisecond,
		HideError:            true,
		Fn: func() error {
			if b.IsEmpty() {
				return nil
			}

			b.flush()
			return fmt.Errorf("not fully flushed")
		},
	})

	if err != nil {
		log.Error("Could not flush all the output in the buffer")
	}
}
