package shell

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
//   the buffer for more than 100 milisecond. The reasoning here is that
//   it should take no more than 100 milliseconds for the TTY to flush its
//   output.
//         if the TTY has no output, but the command is still running, it takes more than 100ms for that to happen.
//
// - If the UTF-8 sequence is complete. Cutting the UTF-8 sequence in half
//   leads to undefined (?) characters in the UI.
//

const OutputBufferMaxTimeSinceLastAppend = 100 * time.Millisecond
const OutputBufferDefaultCutLength = 100

type OutputBuffer struct {
	OnFlush    func(string)
	bytes      []byte
	mu         sync.Mutex
	done       bool
	lastAppend *time.Time
}

func NewOutputBuffer(onFlushFn func(string)) (*OutputBuffer, error) {
	if onFlushFn == nil {
		return nil, fmt.Errorf("output buffer requires an onFlushFn")
	}

	b := &OutputBuffer{OnFlush: onFlushFn, bytes: []byte{}}
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
	for {

		/*
		 * If the buffer was closed (command finished),
		 * we end the flushing goroutine.
		 */
		if b.done {
			log.Debugf("The output buffer was closed - stopping.")
			break
		}

		/*
		 * If there's nothing to flush, we wait a little bit.
		 */
		if b.IsEmpty() {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		timeSinceLastAppend := b.timeSinceLastAppend()

		/*
		 * If there's recent, but not enough data in the buffer, we wait a little more time.
		 */
		if len(b.bytes) < OutputBufferDefaultCutLength && timeSinceLastAppend < OutputBufferMaxTimeSinceLastAppend {
			log.Debugf("The output buffer has only %d bytes and the flush was %v ago - waiting...", len(b.bytes), timeSinceLastAppend)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		/*
		 * Here, we know that the data in the buffer is either above the chunk size we want, or is old enough.
		 * Flush, everything we can.
		 */
		b.flush()
	}
}

func (b *OutputBuffer) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()

	log.Debugf("%d bytes in the buffer - flushing...", len(b.bytes))

	//
	// First we determine how much to cut.
	//
	// We don't want to flush too much in any iteration, but neither we want to
	// flush too little.
	//
	// Starting from the default cut lenght, and decreasing the lenght until we
	// are ready to flush.
	//
	cutLength := OutputBufferDefaultCutLength

	//
	// We can't cut more than we have in the buffer.
	//
	if len(b.bytes) < cutLength {
		cutLength = len(b.bytes)
	}

	//
	// Now comes the tricky part.
	//
	// We don't want to cut in the middle of an UTF-8 sequence.
	//
	// In the below loop, we are cutting of the last 3 charactes in case
	// they are marked as the unicode continuation characters.
	//
	// An unicode sequence can't be longer than 4 bytes
	//
	//
	// If there is only broken bytes in the buffer, we don't want to wait
	// indefinetily. We only run this check if the last insert was recent enough.
	//

	if b.timeSinceLastAppend() < OutputBufferMaxTimeSinceLastAppend && !b.done {
		for i := 0; i < 4; i++ {
			if utf8.Valid(b.bytes[0:cutLength]) {
				break
			} else {
				cutLength--
			}
		}
	}

	if cutLength == 0 {
		return
	}

	bytes := make([]byte, cutLength)
	copy(bytes, b.bytes[0:cutLength])
	b.bytes = b.bytes[cutLength:]

	/*
	 * Make sure we normalize newline sequences, and send the chunk back to its consumer.
	 */
	output := strings.Replace(string(bytes), "\r\n", "\n", -1)
	log.Debugf("%d bytes flushed", len(bytes))
	b.OnFlush(output)
}

func (b *OutputBuffer) timeSinceLastAppend() time.Duration {
	if b.lastAppend != nil {
		return time.Since(*b.lastAppend)
	}

	return time.Millisecond
}

func (b *OutputBuffer) Close() {
	// stop concurrent flushing goroutine
	b.done = true

	// wait until buffer is empty, for 1s.
	log.Debugf("Waiting for buffer to be completely flushed...")
	retry.RetryWithConstantWait(retry.RetryOptions{
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
}
