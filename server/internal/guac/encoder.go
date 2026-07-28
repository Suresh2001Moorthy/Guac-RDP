package guac

import (
	"io"
)

// Encoder serializes Guacamole instructions to a writer.
type Encoder struct {
	writer io.Writer
}

// NewEncoder creates a new Guacamole instruction encoder.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		writer: w,
	}
}

// WriteInstruction encodes and writes a single instruction to the stream.
func (e *Encoder) WriteInstruction(ins *Instruction) error {
	_, err := io.WriteString(e.writer, ins.String())
	return err
}
