package bufferedwriter

import "io"

type BufferedWriter struct {
	w      io.Writer
	writeC chan []byte
}

func New(writer io.Writer) *BufferedWriter {
	bw := &BufferedWriter{
		w:      writer,
		writeC: make(chan []byte, 100),
	}
	go func() {
		for data := range bw.writeC {
			bw.w.Write(data)
		}
	}()
	return bw
}

func (bw *BufferedWriter) Close() {
	close(bw.writeC)
}

func (bw *BufferedWriter) Write(p []byte) (int, error) {
	lenp := len(p)
	data := make([]byte, lenp, lenp)
	copy(data, p)
	bw.writeC <- data
	return lenp, nil
}
