package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// renderProgramProgress prints one line per program attempt. A count is shown
// only as the program gave it; a settled attempt's report is marked as its
// last word, because it proves nothing about how the attempt ended.
func renderProgramProgress(out io.Writer, view prifly.ProgramProgressView) error {
	if len(view.Attempts) == 0 {
		_, err := fmt.Fprintln(out, "no program attempts in "+view.RunID)
		return err
	}
	for _, a := range view.Attempts {
		line := []string{a.StageID, a.AttemptID, a.State}
		if a.Phase != "" {
			phase := "phase=" + a.Phase
			if a.Current != nil {
				phase += " " + strconv.FormatInt(*a.Current, 10)
				if a.Total != nil {
					phase += "/" + strconv.FormatInt(*a.Total, 10)
				}
			}
			line = append(line, phase)
		}
		if a.Message != "" {
			line = append(line, strconv.Quote(a.Message))
		}
		if a.Observed != nil {
			line = append(line, "at "+a.Observed.Format("15:04:05"))
		}
		if a.Settled {
			line = append(line, "(attempt "+a.AttemptStatus+"; last report, not its outcome)")
		}
		if a.Rejected != 0 {
			line = append(line, fmt.Sprintf("[%d reports refused]", a.Rejected))
		}
		if a.Truncated {
			line = append(line, "[past the bound; the rest was discarded]")
		}
		if _, err := fmt.Fprintln(out, strings.Join(line, "  ")); err != nil {
			return err
		}
	}
	return nil
}

// renderProgramOutput prints what a program opened of its output. The text is
// the program's own and may carry terminal control sequences; they are shown,
// escaped, and never sent to this terminal to act on.
func renderProgramOutput(out io.Writer, view prifly.ProgramOutputView) error {
	if !view.Disclosed {
		_, err := fmt.Fprintf(out, "%s: this program's executor does not declare live_output; nothing of its output is kept\n", view.AttemptID)
		return err
	}
	header := fmt.Sprintf("%s %s  %s  limit %d bytes per stream", view.StageID, view.AttemptID, view.State, view.OutputLimitBytes)
	if view.Settled {
		header += "  (attempt " + view.AttemptStatus + "; its output, not its outcome)"
	}
	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}
	if len(view.Streams) == 0 {
		_, err := fmt.Fprintln(out, "no output kept yet")
		return err
	}
	for _, stream := range view.Streams {
		note := fmt.Sprintf("--- %s bytes %d..%d", stream.Stream, stream.StartOffset, stream.EndOffset)
		if stream.StartOffset > 0 {
			note += fmt.Sprintf(" (the first %d bytes are not kept)", stream.StartOffset)
		}
		if stream.Gap {
			note += " (with gaps: parts did not fit the tail)"
		}
		if _, err := fmt.Fprintln(out, note); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, terminalSafe(stream.Text)); err != nil {
			return err
		}
	}
	return nil
}

// terminalSafe keeps line breaks and tabs and shows every other control
// character as an escape, so program output cannot move this terminal.
func terminalSafe(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
			continue
		}
		fmt.Fprintf(&b, "\\x%02x", r)
	}
	return b.String()
}
