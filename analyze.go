package moshpit

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/crushedpixel/go-timecode/timecode"
	"golang.org/x/net/context"
)

type FrameType uint

const (
	Unknown FrameType = iota
	IFrame
	PFrame
)

// we assume that an AVI frame is never larger than 1MB
const maxAviFrameBytes = 1024 * 1024

// the AVI frame delimiter
var frameDelim = []byte{48, 48, 100, 99} // ASCII 00dc

var iframePrefix = []byte{0, 1, 176} // hex 0x0001B0
var pframePrefix = []byte{0, 1, 182} // hex 0x0001B6

// frameDelimSplitFunc is no longer needed

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

// AnalyzeFrames analyzes the frames in the given file,
// writing results to the channel provided.
// The file is assumed to have AVI format.
// Any errors encountered are sent to the error channel.
// The error channel is closed when processing is finished.
func AnalyzeFrames(ctx context.Context, inputFile io.Reader,
	framesChan chan<- FrameType, errorChan chan<- error) {

	defer close(errorChan)

	ar := AviScanner(inputFile)
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
			frame, err := ar.ReadFrame()
			if err != nil {
				if err == io.EOF {
					break
				}
				errorChan <- err
				return
			}

			frameType := Unknown
			if len(frame) >= 8 {
				if bytes.Compare(frame[5:8], pframePrefix) == 0 {
					frameType = PFrame
				} else if bytes.Compare(frame[5:8], iframePrefix) == 0 {
					frameType = IFrame
				}
			}

			framesChan <- frameType
		}
	}
}

type VideoTime struct {
	Time     time.Duration
	Frame    uint64
	Fps      float64
	timecode string
}

// Timecode returns the VideoTime formatted as a Timecode.
func (v *VideoTime) Timecode() string {
	return v.timecode
}

// regex extracting the fps value of the first stream
var ffmpegFPSRegex = regexp.MustCompile(`Stream #0:0[\s\S]* ([0-9.]*) fps`)

// regex extracting the pts_time value from a showinfo line
var ffmpegShowinfoTimestampRegex = regexp.MustCompile(`[Parsed_showinfo_1[\s\S]* pts_time:([0-9.]*)`)

// FindScenes uses ffmpeg to find scene changes in the input file,
// using the given similarity threshold between 0 and 1.
// The detection progress is frequently written to the
// progress channel as a value between 0.0 and 1.0.
// Any errors encountered are sent to the error channel.
// The error channel is closed when processing is finished.
func FindScenes(ctx context.Context, ffmpegPath string,
	ffmpegLogPath string, inputFile string, threshold float64,
	sceneTimeChan chan<- VideoTime, progressChan chan<- float64,
	errorChan chan<- error) {

	defer close(errorChan)
	if threshold < 0 || threshold > 1 {
		errorChan <- errors.New("scene detection threshold must be a value between 0 and 1")
		return
	}

	args := []string{
		"-i", inputFile,
		// apply the showinfo filter on all frames that are a scene change,
		// printing information about the frames to stderr
		"-filter:v", fmt.Sprintf("select='gte(scene,%f)',showinfo", threshold),
		// specify no output file, we're only interested
		// in the command line output
		"-f", "null", "-",
	}

	lineChan := make(chan string)
	errProxyChan := make(chan error)
	go runFFmpeg(ctx, ffmpegPath, args, ffmpegLogPath, progressChan, lineChan, errProxyChan)

	var fps float64
	var rate timecode.Rate
	for {
		select {
		case err, ok := <-errProxyChan:
			if ok {
				errorChan <- err
			}
			return
		case line := <-lineChan:
			if m := ffmpegFPSRegex.FindStringSubmatch(line); m != nil {
				// we found the fps value of the video stream
				var err error
				fps, err = strconv.ParseFloat(m[1], 64)
				if err != nil {
					errorChan <- fmt.Errorf("error parsing fps value: %s", err.Error())
					return
				}
				rate = timecode.NewFloatRate(float32(fps))
			}

			if m := ffmpegShowinfoTimestampRegex.FindStringSubmatch(line); m != nil {
				// we found the timestamp of a scene change
				if fps == 0 {
					// we haven't found the fps value, which should always
					// occur before the showinfo output
					errorChan <- errors.New("could not find fps value of input file")
					return
				}

				timestamp, err := strconv.ParseFloat(m[1], 64)
				if err != nil {
					errorChan <- fmt.Errorf("error parsing timestamp value: %s", err.Error())
					return
				}
				t := time.Duration(timestamp * float64(time.Second))

				// calculate the frame index of the scene change
				tc := timecode.New(t, rate)
				sceneTimeChan <- VideoTime{
					Time:     t,
					Frame:    uint64(tc.Frame()),
					Fps:      fps,
					timecode: tc.String(),
				}
			}
		}
	}
}