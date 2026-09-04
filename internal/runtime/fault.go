package runtime

import "fmt"

// Fault is a refusal this engine authored. The code is the stable name a client
// reads and the message explains it to a person, so neither has to be recovered
// by parsing the other back out of one string. A refusal that carried its code
// only in text had to be split again by every reader, and a bare code with no
// message read as no code at all.
type Fault struct {
	Code    string
	Message string
	Cause   error
}

func (f *Fault) Error() string {
	if f.Message == "" {
		return f.Code
	}
	return f.Code + ": " + f.Message
}

func (f *Fault) Unwrap() error { return f.Cause }

// fault names a refusal. The message is engine-authored text, never a path,
// an argument or a worker's own words.
func fault(code, message string) error { return &Fault{Code: code, Message: message} }

// faultf is fault with a formatted message; the format is engine-authored.
func faultf(code, format string, args ...any) error {
	return &Fault{Code: code, Message: fmt.Sprintf(format, args...)}
}

// wrapFault keeps the cause reachable for errors.Is/As while naming the refusal
// a client sees. The cause's text is not part of the message.
func wrapFault(code, message string, cause error) error {
	return &Fault{Code: code, Message: message, Cause: cause}
}
