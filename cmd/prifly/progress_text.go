package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

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
