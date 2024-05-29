package moshpit

import (
	"bufio"
	"bytes"
	"io"
)

// We assume that an AVI frame is never larger than 10MB for 4K video at 60fps
const maxAviFrameBytes = 10 * 1024 * 1024

// The AVI frame delimiter
var (
	frameDelim   = []byte{48, 48, 100, 99} // ASCII 00dc
	iframePrefix = []byte{0, 1, 176}       // hex 0x0001B0
	pframePrefix = []byte{0, 1, 182}       // hex 0x0001B6
)

// AviReader reads AVI frames from an io.Reader.
type AviReader struct {
	bufReader *bufio.Reader
	buffer    []byte
}

// AviScanner creates a new AviReader with an increased buffer size.
func AviScanner(reader io.Reader) *AviReader {
	return &AviReader{
		bufReader: bufio.NewReaderSize(reader, 64*1024), // 64KB initial buffer size
		buffer:    make([]byte, 0, 64*1024),            // 64KB initial buffer capacity
	}
}

// ReadFrame reads the next AVI frame.
func (ar *AviReader) ReadFrame() ([]byte, error) {
	for {
		// Look for the frame delimiter in the current buffer
		index := bytes.Index(ar.buffer, frameDelim)
		if index != -1 {
			// We found a complete frame
			frame := ar.buffer[:index+len(frameDelim)]
			ar.buffer = ar.buffer[index+len(frameDelim):] // Remove the processed frame from the buffer
			return frame, nil
		}

		// If we don't have a complete frame, read more data
		chunk := make([]byte, 64*1024) // Read in chunks of 64KB
		n, err := ar.bufReader.Read(chunk)
		if err != nil {
			if err == io.EOF && len(ar.buffer) > 0 {
				// If EOF and we have leftover data in buffer, return it as the last frame
				frame := ar.buffer
				ar.buffer = nil
				return frame, nil
			}
			return nil, err
		}

		// Append the new chunk to the buffer
		ar.buffer = append(ar.buffer, chunk[:n]...)

		// Ensure we don't exceed the maximum buffer size
		if len(ar.buffer) > maxAviFrameBytes {
			return nil, io.ErrShortBuffer
		}
	}
}