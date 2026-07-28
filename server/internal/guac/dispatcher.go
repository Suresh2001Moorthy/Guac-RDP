package guac

// Handler interface for handling parsed Guacamole instructions.
type Handler interface {
	HandleInstruction(ins *Instruction) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as Handlers.
type HandlerFunc func(ins *Instruction) error

func (f HandlerFunc) HandleInstruction(ins *Instruction) error {
	return f(ins)
}

// Dispatcher routes instructions to appropriate handlers based on their Opcode.
type Dispatcher struct {
	handlers map[string]Handler
	fallback Handler
}

// NewDispatcher creates a new instruction dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
	}
}

// Register maps an opcode to a handler.
func (d *Dispatcher) Register(opcode string, handler Handler) {
	d.handlers[opcode] = handler
}

// SetFallback sets a handler for unknown opcodes.
func (d *Dispatcher) SetFallback(handler Handler) {
	d.fallback = handler
}

// Dispatch processes an instruction and invokes the appropriate handler.
func (d *Dispatcher) Dispatch(ins *Instruction) error {
	if handler, ok := d.handlers[ins.Opcode]; ok {
		return handler.HandleInstruction(ins)
	}
	if d.fallback != nil {
		return d.fallback.HandleInstruction(ins)
	}
	// Unknown instructions are silently ignored as per protocol design if no fallback
	return nil
}
