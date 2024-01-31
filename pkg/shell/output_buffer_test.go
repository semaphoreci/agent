package shell

import (
	"context"
	"strings"
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
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	//
	// Making sure that the input is long enough to the flushed immediately
	//
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, strings.Join(output, ""), string(input))
}

func Test__OutputBuffer__SimpleAscii__ShorterThanMinimalCutLength(t *testing.T) {
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	input := []byte("aaa")
	buffer.Append(input)

	// output is too short, so it will only be flushed
	// when the max delay is reached.
	assert.Len(t, output, 0)

	// We need to wait a bit before flushing, the buffer is still too short
	assert.Eventually(t, func() bool { return strings.Join(output, "") == string(input) }, time.Second, 100*time.Millisecond)
	require.NoError(t, buffer.Close())
}

func Test__OutputBuffer__SimpleAscii__LongerThanMinimalCutLength(t *testing.T) {
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

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
	if assert.Len(t, output, 2) {
		assert.Equal(t, output[0], string(input[:OutputBufferDefaultCutLength]))
		assert.Equal(t, output[1], string(input[OutputBufferDefaultCutLength:]))
	}
}

func Test__OutputBuffer__SimpleAscii__ChunkIncreasesWhenClosed(t *testing.T) {
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength+50; i++ {
		input = append(input, 'a')
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())

	// everything is flushed in one chunk
	if assert.Len(t, output, 1) {
		assert.Equal(t, output[0], string(input))
	}
}

func Test__OutputBuffer__UTF8_Sequence__Simple(t *testing.T) {
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	//
	// Making sure that the input is long enough to the flushed immidiately
	//
	input := []byte{}
	for len(input) <= OutputBufferDefaultCutLength {
		input = append(input, []byte("特")...)
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, strings.Join(output, ""), string(input))
}

func Test__OutputBuffer__UTF8_Sequence__Short(t *testing.T) {
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	input := []byte("特特特")
	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, strings.Join(output, ""), string(input))
}

func Test__OutputBuffer__InvalidUTF8_Sequence(t *testing.T) {
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	//
	// Making sure that the input is long enough to the flushed immediately
	//
	input := []byte{}
	for len(input) <= OutputBufferDefaultCutLength {
		input = append(input, []byte("\xF4\xBF\xBF\xBF")...)
	}

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, strings.Join(output, ""), string(input))
}

func Test__OutputBuffer__FlushIgnoresCharactersThatAreNotUtf8Valid(t *testing.T) {
	//
	// We construct a 100 byte long string to enable a full flush.
	//
	// The first 99 bytes will come from the 3-byte long kanji character, while
	// the last byte will be a broken character
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	input := ""
	for i := 0; i < 33; i++ {
		input += "特"
	}

	nonUtf8Chars := []byte{[]byte("特")[0]}

	// In total, we are inserting 100 bytes
	buffer.Append([]byte(input))
	buffer.Append(nonUtf8Chars)

	// In the output, we expect that the last broken byte is not returned initially.
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, strings.Join(output, ""), input)
	require.NoError(t, buffer.Close())
}

func Test__OutputBuffer__FlushReturnsBytesThatAreBrokenAndSitInTheBufferForTooLong(t *testing.T) {
	//
	// We construct a 100 byte long string to enable a full flush.
	//
	// The first 99 bytes will come from the 3-byte long kanji character, while
	// the last byte will be a broken character
	//
	output := []string{}
	buffer, _ := NewOutputBuffer(func(s string) { output = append(output, s) })

	input := []byte{}
	for i := 0; i < 33; i++ {
		input = append(input, []byte("特")...)
	}
	input = append(input, []byte("特")[0])

	buffer.Append(input)
	require.NoError(t, buffer.Close())
	assert.Equal(t, strings.Join(output, ""), string(input))
}

func Test__OutputBuffer__DoesNotWaitForeverForOutputToBeFlushed(t *testing.T) {
	input := []byte{}
	for i := 0; i < OutputBufferDefaultCutLength*10; i++ {
		input = append(input, 'a')
	}

	buffer, _ := NewOutputBufferWithFlushTimeout(func(s string) {}, time.Second)

	// write a lot of data to the buffer
	for i := 0; i < 1000; i++ {
		buffer.Append(input)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// on a separate goroutine, we continuosly write
	// to make sure the buffer is never empty
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				buffer.Append(input)
			}
		}
	}()

	// here, we try to close, which will not work
	// since we will attempt to flush while the buffer is being continuosly written.
	err := buffer.Close()
	assert.ErrorContains(t, err, "context deadline exceeded")
	cancel()
}
