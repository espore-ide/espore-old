package session

import (
	"bufio"
	"bytes"
	"io"
	"time"
)

type LineWriter struct {
	w io.Writer
	b bytes.Buffer
}

func NewLineWriter(writer io.Writer) *LineWriter {
	return &LineWriter{
		w: writer,
	}
}

func (lw *LineWriter) Write(data []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := append(scanner.Bytes(), 10) // add a LF at the end
		i, err := lw.w.Write(line)
		if err != nil {
			return i, err
		}
		time.Sleep(throttle)
	}
	return len(data), nil
}
