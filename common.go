package moshpit

import (
	"bufio"
	"bytes"
	"io"
)

// we assume that an AVI frame is never larger than 10MB for 4K video at 60fps
const maxAviFrameBytes = 10 * 1024 * 1024

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

// AviScanner creates a new AviReader with an increased buffer size.
func AviScanner(reader io.Reader) *AviReader {
	return &AviReader{
		bufReader: bufio.NewReaderSize(reader, maxAviFrameBytes),
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
			n, err := frameBuffer.ReadFrom(ar.bufReader)
			if err != nil && err != io.EOF {
				return nil, err
			}
			if n == 0 && err == io.EOF {
				break
			}
			continue
		}

		frameBuffer.Write(data[:index+len(frameDelim)])
		if _, err := ar.bufReader.Discard(index + len(frameDelim)); err != nil {
			return nil, err
		}
		return frameBuffer.Bytes(), nil
	}
	return frameBuffer.Bytes(), nil
}