package history_test

import (
	"bytes"
	"espore/cli/history"
	"fmt"
	"strings"
	"testing"

	"github.com/epiclabs-io/ut"
)

func TestHistory(tx *testing.T) {
	t := ut.BeginTest(tx, false)
	defer t.FinishTest()

	fileContent := "line1\nline2\nline3\n"

	var lines []string

	h, err := history.New(bytes.NewBufferString(fileContent), &history.Config{
		Limit: 5,
		OnAppend: func(line string) {
			lines = append(lines, line)
		},
	})
	t.Ok(err)

	// upon start, the pointer is at the end, so Current must return ""
	t.Equals("", h.Current())

	length := h.Len()
	t.Equals(length, 3)

	// go up the history one step
	up := h.Up()
	t.Equals("line3", up)

	// go up the history another step
	up = h.Up()
	t.Equals("line2", up)

	// go up the history another step
	up = h.Up()
	t.Equals("line1", up)

	// going up again does not have effect since we are at BOF
	up = h.Up()
	t.Equals("line1", up)

	t.Equals("line1", h.Current())

	// go down the history one step
	down := h.Down()
	t.Equals("line2", down)

	// go down the history another step
	down = h.Down()
	t.Equals("line3", down)

	// going past the last item returns ""
	down = h.Down()
	t.Equals("", down)

	// trying to go further down has no effect
	down = h.Down()
	t.Equals("", down)

	// add a new line
	h.Append("line4")
	t.Equals(lines, []string{"line4"})

	// repeating the last item has no effect
	h.Append("line4")
	t.Equals(lines, []string{"line4"})

	// adding a new item moves history past EOF, even if repeated
	t.Equals("", h.Current())

	// adding a different item causes the line to be appended
	h.Append("line5")
	t.Equals(lines, []string{"line4", "line5"})

	// adding an empty command is ignored
	h.Append("  ")
	h.Append("")
	t.Equals(lines, []string{"line4", "line5"})

	// adding a new item moves history past EOF
	t.Equals("", h.Current())

	// going up should retrieve the last item
	up = h.Up()
	t.Equals("line5", up)

	// after adding the above line, we should have 5 lines:
	length = h.Len()
	t.Equals(length, 5)

	// add a 6th line
	h.Append("line6")

	// length should still be 5 since the history length limit is set to 5
	length = h.Len()
	t.Equals(length, 5)

	// generate a long history:

	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString(fmt.Sprintf("line %d\n", i))
	}

	// start over
	h, err = history.New(bytes.NewBufferString(b.String()), &history.Config{
		Limit: 5,
		OnAppend: func(line string) {
			lines = append(lines, line)
		},
	})

	// length should be 5 since the history length limit is set to 5
	length = h.Len()
	t.Equals(length, 5)

	t.Ok(err)

}
