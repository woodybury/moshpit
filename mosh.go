package moshpit

import (
	"bytes"
	"io"
	"golang.org/x/net/context"
)

// we assume that an AVI frame is never larger than 1MB
const maxAviFrameBytes = 1024 * 1024

// the AVI frame delimiter
var frameDelim = []byte{48, 48, 100, 99} // ASCII 00dc

var iframePrefix = []byte{0, 1, 176} // hex 0x0001B0
var pframePrefix = []byte{0, 1, 182} // hex 0x0001B6

// AviReader reads AVI frames from an io.Reader.
type AviReader struct {
	bufReader *bufio.Reader
}

// AviScanner creates a new AviReader.
func AviScanner(reader io.Reader) *AviReader {
	return &AviReader{
		bufReader: bufio.NewReader(reader),
	}
}

// ReadFrame reads the next AVI frame.
func (ar *AviReader) ReadFrame() ([]byte, error) {
	var frameBuffer bytes.Buffer
	for {
		data, err := ar.bufReader.Peek(maxAviFrameBytes)
		if err != nil && err != io.EOF {
			return nil, err
		}
		index := bytes.Index(data, frameDelim)
		if index == -1 {
			if _, err := frameBuffer.ReadFrom(ar.bufReader); err != nil && err != io.EOF {
				return nil, err
			}
			continue
		}

		frameBuffer.Write(data[:index+len(frameDelim)])
		if _, err := ar.bufReader.Discard(index + len(frameDelim)); err != nil {
			return nil, err
		}
		return frameBuffer.Bytes(), nil
	}
}

// RemoveFrames writes a copy of the AVI data from the input reader
// to the output writer, replacing the frames at the given indices
// with the following frame.
// Any errors encountered are sent to the error channel.
// The error channel is closed when processing is finished.
func RemoveFrames(ctx context.Context, input io.Reader, output io.Writer,
	framesToRemove []uint64, processedChan chan<- uint64, errorChan chan<- error) {

	defer close(errorChan)
	r := AviScanner(input)

	// counter of how many frames to duplicate
	duplicate := 0
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			frame, err := r.ReadFrame()
			if err != nil {
				if err == io.EOF {
					return
				}
				errorChan <- err
				return
			}

			if i == 0 {
				// write the original frames
				// without incrementing the counter
				// until the first I-Frame is encountered -
				// all frames before that are header frames
				if _, err := output.Write(frame); err != nil {
					errorChan <- err
					return
				}

				if bytes.Compare(frame[5:8], iframePrefix) == 0 {
					// we found the first I-Frame - increment
					// the counter to stop the search and begin moshing
					i++
				}

				continue
			}

			if contains(framesToRemove, uint64(i)) {
				duplicate++
			} else {
				duplicate++
				for duplicate > 0 {
					if _, err := output.Write(frame); err != nil {
						errorChan <- err
						return
					}
					duplicate--
				}
			}

			if i >= 0 {
				processedChan <- uint64(i)
			}

			i++
		}
	}
}

func contains(values []uint64, x uint64) bool {
	for _, val := range values {
		if val == x {
			return true
		}
	}
	return false
}