package shell

import (
	"testing"
	"time"

	assert "github.com/stretchr/testify/assert"
)

func Test__OutputBuffer__SimpleAscii(t *testing.T) {
	buffer := NewOutputBuffer()

	//
	// Making sure that the input is long enough to the flushed immidiately
	//
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)
	flushed, ok := buffer.Flush()

	assert.Equal(t, ok, true)
	assert.Equal(t, flushed, string(input))
}

func Test__OutputBuffer__SimpleAscii__ShorterThanMinimalCutLength(t *testing.T) {
	buffer := NewOutputBuffer()

	input := []byte("aaa")

	buffer.Append(input)
	flushed, ok := buffer.Flush()

	// We need to wait a bit before flushing, the buffer is still too short
	assert.Equal(t, ok, false)

	time.Sleep(OutputBufferMaxTimeSinceLastAppend)

	flushed, ok = buffer.Flush()
	assert.Equal(t, ok, true)
	assert.Equal(t, flushed, string(input))
}

func Test__OutputBuffer__SimpleAscii__LongerThanMinimalCutLength(t *testing.T) {
	buffer := NewOutputBuffer()

	//
	// Making sure that the input is long enough to have to be flushed two times.
	//
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength+50; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)

	flushed1, ok := buffer.Flush()
	assert.Equal(t, ok, true)
	assert.Equal(t, flushed1, string(input[:OutputBufferDefaultCutLength]))

	// We need to wait a bit before flushing, the buffer is still too short
	time.Sleep(OutputBufferMaxTimeSinceLastAppend)

	flushed2, ok := buffer.Flush()
	assert.Equal(t, ok, true)
	assert.Equal(t, flushed2, string(input[OutputBufferDefaultCutLength:]))
}

func Test__OutputBuffer__UTF8_Sequence__Simple(t *testing.T) {
	buffer := NewOutputBuffer()

	//
	// Making sure that the input is long enough to the flushed immidiately
	//
	input := []byte{}
	for len(input) <= OutputBufferDefaultCutLength {
		input = append(input, []byte("特")...)
	}

	buffer.Append(input)

	out := ""

	for !buffer.IsEmpty() {
		flushed, ok := buffer.Flush()

		if ok {
			out += flushed
		} else {
			time.Sleep(OutputBufferMaxTimeSinceLastAppend)
		}
	}

	assert.Equal(t, out, string(input))
}

func Test__OutputBuffer__UTF8_Sequence__Short(t *testing.T) {
	buffer := NewOutputBuffer()

	//
	// Making sure that the input is long enough to the flushed immidiately
	//
	input := []byte("特特特")

	buffer.Append(input)

	out := ""

	for !buffer.IsEmpty() {
		flushed, ok := buffer.Flush()

		if ok {
			out += flushed
		} else {
			time.Sleep(OutputBufferMaxTimeSinceLastAppend)
		}
	}

	assert.Equal(t, out, string(input))
}

func Test__OutputBuffer__InvalidUTF8_Sequence(t *testing.T) {
	buffer := NewOutputBuffer()

	//
	// Making sure that the input is long enough to the flushed immidiately
	//
	input := []byte{}
	for len(input) <= OutputBufferDefaultCutLength {
		input = append(input, []byte("\xF4\xBF\xBF\xBF")...)
	}

	buffer.Append(input)

	out := ""

	for !buffer.IsEmpty() {
		flushed, ok := buffer.Flush()

		if ok {
			out += flushed
		} else {
			time.Sleep(OutputBufferMaxTimeSinceLastAppend)
		}
	}

	assert.Equal(t, out, string(input))
}

func Test__OutputBuffer__FlushIgnoresCharactersThatAreNotUtf8Valid(t *testing.T) {
	//
	// We construct a 100 byte long string to enable a full flush.
	//
	// The first 99 bytes will come from the 3-byte long kanji character, while
	// the last byte will be a broken character

	buffer := NewOutputBuffer()

	input := ""
	for i := 0; i < 33; i++ {
		input += "特"
	}

	nonUtf8Chars := []byte{[]byte("特")[0]}

	// In total, we are inserting 100 bytes
	buffer.Append([]byte(input))
	buffer.Append(nonUtf8Chars)

	// In the output, we expect that the last broken byte is not returned.

	out, ok := buffer.Flush()

	assert.Equal(t, ok, true)
	assert.Equal(t, out, input)
}

func Test__OutputBuffer__FlushReturnsBytesThatAreBrokenAndSitInTheBufferForTooLong(t *testing.T) {
	//
	// We construct a 100 byte long string to enable a full flush.
	//
	// The first 99 bytes will come from the 3-byte long kanji character, while
	// the last byte will be a broken character
	//

	buffer := NewOutputBuffer()

	input := []byte{}
	for i := 0; i < 33; i++ {
		input = append(input, []byte("特")...)
	}
	input = append(input, []byte("特")[0])

	buffer.Append(input)

	// We wait for a while, and let the broken become bocome stale.
	// Stale characters are forced out of the buffer, even if they are not valid.
	time.Sleep(200 * time.Millisecond)

	out, ok := buffer.Flush()
	assert.Equal(t, ok, true)
	assert.Equal(t, out, string(input))
}
