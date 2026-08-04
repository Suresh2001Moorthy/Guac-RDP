package guac

import (
	"fmt"
	"strings"
)

// Instruction represents a single Guacamole protocol command.
type Instruction struct {
	Opcode string
	Args   []string
}

// NewInstruction creates a new instruction with the given opcode and arguments.
func NewInstruction(opcode string, args ...string) *Instruction {
	return &Instruction{
		Opcode: opcode,
		Args:   args,
	}
}

// String returns the Guacamole-encoded string representation of the instruction.
func (i *Instruction) String() string {
	parts := make([]string, 0, len(i.Args)+1)
	parts = append(parts, fmt.Sprintf("%d.%s", len(i.Opcode), i.Opcode))
	
	for _, arg := range i.Args {
		parts = append(parts, fmt.Sprintf("%d.%s", len(arg), arg))
	}
	
	return strings.Join(parts, ",") + ";"
}
