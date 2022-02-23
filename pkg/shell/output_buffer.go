package shell

import (
	"strings"
	"time"
	"unicode/utf8"

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
//
// - If the UTF-8 sequence is complete. Cutting the UTF-8 sequence in half
//   leads to undefined (?) characters in the UI.
//

const OutputBufferMaxTimeSinceLastAppend = 100 * time.Millisecond
const OutputBufferDefaultCutLength = 100

type OutputBuffer struct {
	bytes []byte

	lastAppend *time.Time
}

func NewOutputBuffer() *OutputBuffer {
	return &OutputBuffer{bytes: []byte{}}
}

func (b *OutputBuffer) Append(bytes []byte) {
	now := time.Now()
	b.lastAppend = &now

	b.bytes = append(b.bytes, bytes...)
}

func (b *OutputBuffer) IsEmpty() bool {
	return len(b.bytes) == 0
}

func (b *OutputBuffer) Flush() (string, bool) {
	if b.IsEmpty() {
		return "", false
	}

	timeSinceLastAppend := 1 * time.Millisecond
	if b.lastAppend != nil {
		timeSinceLastAppend = time.Since(*b.lastAppend)
	}

	log.Debugf("Flushing. %d bytes in the buffer", len(b.bytes))

	// We don't want to flush too often.
	//
	// We either:
	//
	//   - wait till there is enough in the buffer
	//   - wait till the dat sitting in the buffer is old enough

	if len(b.bytes) < OutputBufferDefaultCutLength && timeSinceLastAppend < OutputBufferMaxTimeSinceLastAppend {
		return "", false
	}

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

	if timeSinceLastAppend < OutputBufferMaxTimeSinceLastAppend {
		for i := 0; i < 4; i++ {
			if utf8.Valid(b.bytes[0:cutLength]) {
				break
			} else {
				cutLength--
			}
		}
	}

	// Flushing...

	output := make([]byte, cutLength)
	copy(output, b.bytes[0:cutLength])
	b.bytes = b.bytes[cutLength:]

	return strings.Replace(string(output), "\r\n", "\n", -1), true
}
