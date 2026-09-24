package model

type Command string

const CommandClose Command = "/close"

func (c Command) String() string {
	return string(c)
}
