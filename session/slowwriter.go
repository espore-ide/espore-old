package session

import (
	"io"
	"time"
)

type SlowWriter struct {
	w io.Writer
}

func NewSlowWriter(writer io.Writer) *SlowWriter {
	return &SlowWriter{
		w: writer,
	}
}

func (sw *SlowWriter) Write(data []byte) (int, error) {
	size := len(data)
	for {
		thisChunk := chunkSize
		if thisChunk > len(data) {
			thisChunk = len(data)
		}
		if _, err := sw.w.Write(data[:thisChunk]); err != nil {
			return 0, err
		}
		data = data[thisChunk:]
		if len(data) == 0 {
			break
		}
		time.Sleep(throttle)
	}
	return size, nil
}
