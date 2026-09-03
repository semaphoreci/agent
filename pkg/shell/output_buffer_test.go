package shell

import (
	"strings"
	"sync"
	"testing"
	"time"

	assert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test__OutputBuffer__RequiresConsumer(t *testing.T) {
	buffer, err := NewOutputBuffer(nil)
	assert.Error(t, err)
	assert.Nil(t, buffer)
}

func Test__OutputBuffer__SimpleAscii(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	//
	// Making sure that the input is long enough to the flushed immediately
	//
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, output.joined(), string(input))
}

func Test__OutputBuffer__SimpleAscii__ShorterThanMinimalCutLength(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	input := []byte("aaa")
	buffer.Append(input)

	// output is too short, so it will only be flushed
	// when the max delay is reached.
	assert.Equal(t, 0, output.count())

	// We need to wait a bit before flushing, the buffer is still too short
	assert.Eventually(t, func() bool { return output.joined() == string(input) }, time.Second, 100*time.Millisecond)
	require.NoError(t, buffer.Close())
}

func Test__OutputBuffer__SimpleAscii__LongerThanMinimalCutLength(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	//
	// Making sure that the input is long enough to have to be flushed two times.
	//
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength+50; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)

	// wait for the output to be flushed
	time.Sleep(time.Second)

	require.NoError(t, buffer.Close())
	if assert.Equal(t, 2, output.count()) {
		assert.Equal(t, string(input[:OutputBufferDefaultCutLength]), output.at(0))
		assert.Equal(t, string(input[OutputBufferDefaultCutLength:]), output.at(1))
	}
}

func Test__OutputBuffer__DoesNotSplitCRLFNewlinesAcrossChunks(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	input := strings.Repeat("a", OutputBufferDefaultCutLength-1) + "\r\nb"

	buffer.Append([]byte(input))
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, buffer.Close())

	if assert.Equal(t, 2, output.count()) {
		assert.Equal(t, strings.Repeat("a", OutputBufferDefaultCutLength-1), output.at(0))
		assert.Equal(t, "\nb", output.at(1))
	}

	assert.Equal(t, output.joined(), strings.ReplaceAll(input, "\r\n", "\n"))
}

func Test__OutputBuffer__SimpleAscii__ChunkIncreasesWhenClosed(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength+50; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())

	// everything is flushed in one chunk
	if assert.Equal(t, 1, output.count()) {
		assert.Equal(t, string(input), output.at(0))
	}
}

func Test__OutputBuffer__UTF8_Sequence__Simple(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	//
	// Making sure that the input is long enough to the flushed immidiately
	//
	input := []byte{}
	for len(input) <= OutputBufferDefaultCutLength {
		input = append(input, []byte("特")...)
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, output.joined(), string(input))
}

func Test__OutputBuffer__UTF8_Sequence__Short(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	input := []byte("特特特")
	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, output.joined(), string(input))
}

func Test__OutputBuffer__InvalidUTF8_Sequence(t *testing.T) {
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	//
	// Making sure that the input is long enough to the flushed immediately
	//
	input := []byte{}
	for len(input) <= OutputBufferDefaultCutLength {
		input = append(input, []byte("\xF4\xBF\xBF\xBF")...)
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, output.joined(), string(input))
}

func Test__OutputBuffer__FlushIgnoresCharactersThatAreNotUtf8Valid(t *testing.T) {
	//
	// We construct a 100 byte long string to enable a full flush.
	//
	// The first 99 bytes will come from the 3-byte long kanji character, while
	// the last byte will be a broken character
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	input := ""
	for i := 0; i < 33; i++ {
		input += "特"
	}

	nonUtf8Chars := []byte{[]byte("特")[0]}

	// In total, we are inserting 100 bytes
	buffer.Append([]byte(input))
	buffer.Append(nonUtf8Chars)

	/*
	 * The flusher backs off up to a second while the buffer is empty, so it may
	 * not have woken up yet - waiting a fixed few milliseconds races it. And the
	 * broken byte is flushed on its own once it has sat in the buffer for
	 * 100ms, so the assertion has to be about the first chunk rather than about
	 * everything flushed so far.
	 */
	assert.Eventually(t, func() bool {
		return output.count() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	// The last broken byte is not part of the first flush.
	assert.Equal(t, input, output.first())
	require.NoError(t, buffer.Close())
}

// The consumer is called from the buffer's own goroutine.
type collectedOutput struct {
	mu     sync.Mutex
	chunks []string
}

func (o *collectedOutput) append(chunk string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.chunks = append(o.chunks, chunk)
}

func (o *collectedOutput) joined() string {
	o.mu.Lock()
	defer o.mu.Unlock()

	return strings.Join(o.chunks, "")
}

func (o *collectedOutput) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	return len(o.chunks)
}

func (o *collectedOutput) at(index int) string {
	o.mu.Lock()
	defer o.mu.Unlock()

	if index >= len(o.chunks) {
		return ""
	}

	return o.chunks[index]
}

func (o *collectedOutput) first() string {
	return o.at(0)
}

func Test__OutputBuffer__FlushReturnsBytesThatAreBrokenAndSitInTheBufferForTooLong(t *testing.T) {
	//
	// We construct a 100 byte long string to enable a full flush.
	//
	// The first 99 bytes will come from the 3-byte long kanji character, while
	// the last byte will be a broken character
	//
	output := &collectedOutput{}
	buffer, _ := NewOutputBuffer(output.append)

	input := []byte{}
	for i := 0; i < 33; i++ {
		input = append(input, []byte("特")...)
	}
	input = append(input, []byte("特")[0])

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, output.joined(), string(input))
}

func Test__OutputBuffer__DoesNotWaitForeverForOutputToBeFlushed(t *testing.T) {
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength*10; i++ {
		input = append(input, 'a')
	}

	flushTimeout := 100 * time.Millisecond

	// A consumer slower than the flush timeout makes even a single flush overrun
	// the deadline, so Close() is guaranteed to give up with a context deadline
	// error instead of draining the buffer. This is deterministic; the previous
	// version raced a concurrent writer to keep the buffer non-empty, which could
	// intermittently let the buffer drain and made Close() return nil (flaky).
	buffer, _ := NewOutputBufferWithFlushTimeout(func(s string) {
		time.Sleep(3 * flushTimeout)
	}, flushTimeout)

	// Pre-fill so the buffer is non-empty when Close() starts flushing.
	for i := 0; i < 1000; i++ {
		buffer.Append(input)
	}

	// Close() flushes one chunk - which blocks in the slow consumer past the
	// deadline - and then returns the deadline error rather than draining the rest.
	err := buffer.Close()
	assert.ErrorContains(t, err, "context deadline exceeded")
}
