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
	var size int
	for scanner.Scan() {
		i, err := lw.w.Write(scanner.Bytes())
		if err != nil {
			return size, err
		}
		_, err = lw.w.Write([]byte{'\n'})
		if err != nil {
			return size, err
		}
		i++
		time.Sleep(throttle)
	}
	return size, nil
}
