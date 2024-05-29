package moshpit

import (
	"bufio"
	"bytes"
	"io"
)

const (
	// we assume that an AVI frame is never larger than 1MB
	maxAviFrameBytes = 1024 * 1024
)

// the AVI frame delimiter
var (
	frameDelim   = []byte{48, 48, 100, 99} // ASCII 00dc
	iframePrefix = []byte{0, 1, 176}       // hex 0x0001B0
	pframePrefix = []byte{0, 1, 182}       // hex 0x0001B6
)

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