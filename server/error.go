package server

import (
	"errors"
	"fmt"
)

var (
	errUnknownCommand = errors.New(fmt.Sprintf("unknown command"))
)
