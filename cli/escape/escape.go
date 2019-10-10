package escape

import (
	"bytes"
	"io"
)

type Config struct {
	Reader   io.Reader
	Sequence []byte
	Callback func()
}

type escape struct {
	Config
	buf  bytes.Buffer
	data []byte
	pos  int
}

func New(config *Config) *escape {
	return &escape{
		Config: *config,
		data:   make([]byte, 1024),
	}
}

func (esc *escape) Read(p []byte) (int, error) {
	i, err := esc.buf.Read(p)
	if i == len(p) || (err != nil && err != io.EOF) {
		return i, err
	}
	p = p[i:]

	i, err = esc.Reader.Read(esc.data)
	if err != nil && err != io.EOF {
		return 0, err
	}

	if len(esc.Sequence) == 0 {
		esc.buf.Write(esc.data[:i])
	} else {
		for n := 0; n < i; n++ {
			b := esc.data[n]
			if b == esc.Sequence[esc.pos] {
				esc.pos++
				if esc.pos == len(esc.Sequence) {
					esc.Callback()
					esc.pos = 0
				}
			} else {
				esc.buf.Write(esc.Sequence[:esc.pos])
				esc.pos = 0
				esc.buf.WriteByte(b)
			}
		}
	}

	return esc.buf.Read(p)
}
