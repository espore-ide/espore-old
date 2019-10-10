package escape_test

import (
	"bytes"
	"espore/cli/escape"
	"io/ioutil"
	"testing"

	"github.com/epiclabs-io/ut"
)

func TestEscape(tx *testing.T) {
	t := ut.BeginTest(tx, false)
	defer t.FinishTest()

	input := []byte{5, 1, 2, 5, 6, 3, 4, 5, 6, 7, 8, 9, 10, 11, 5, 6, 7, 5, 6, 7}
	seq := []byte{5, 6, 7}
	reader := bytes.NewReader(input)
	var seqCount = 0
	escape := escape.New(&escape.Config{
		Reader:   reader,
		Sequence: seq,
		Callback: func() {
			seqCount++
		},
	})

	data, err := ioutil.ReadAll(escape)
	t.Ok(err)

	t.Equals([]byte{5, 1, 2, 5, 6, 3, 4, 8, 9, 10, 11}, data)
	t.Assert(seqCount == 3, "Expected sequence count to be 3")
}
