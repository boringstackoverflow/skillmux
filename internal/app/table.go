package app

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func newTable(out io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
}

func tableRow(out io.Writer, cols ...string) {
	fmt.Fprintln(out, strings.Join(cols, "\t"))
}
