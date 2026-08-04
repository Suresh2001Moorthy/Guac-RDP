package guac

import (
	"bufio"
	"errors"
	"io"
	"strconv"
)

var (
	ErrConnectionClosed = errors.New("connection closed")
	ErrProtocolError    = errors.New("guacamole protocol error")
)

// Parser reads and decodes Guacamole instructions from a reader.
type Parser struct {
	reader *bufio.Reader
}

// NewParser creates a new Guacamole instruction parser.
func NewParser(r io.Reader) *Parser {
	return &Parser{
		reader: bufio.NewReader(r),
	}
}

// ReadInstruction reads the next complete instruction from the stream.
func (p *Parser) ReadInstruction() (*Instruction, error) {
	var elements []string

	for {
		// Read length prefix
		lengthStr, err := p.reader.ReadString('.')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, ErrConnectionClosed
			}
			return nil, err
		}

		// Remove the trailing '.'
		lengthStr = lengthStr[:len(lengthStr)-1]
		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return nil, ErrProtocolError
		}

		// Read the element content
		elementData := make([]byte, length)
		_, err = io.ReadFull(p.reader, elementData)
		if err != nil {
			return nil, err
		}
		
		elements = append(elements, string(elementData))

		// Read the delimiter (',' or ';')
		delimiter, err := p.reader.ReadByte()
		if err != nil {
			return nil, err
		}

		if delimiter == ';' {
			break
		} else if delimiter != ',' {
			return nil, ErrProtocolError
		}
	}

	if len(elements) == 0 {
		return nil, ErrProtocolError
	}

	return &Instruction{
		Opcode: elements[0],
		Args:   elements[1:],
	}, nil
}
