package plugins

import (
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

type Command struct {
	Name     string
	Category string
	Execute  func(*whatsmeow.Client, *events.Message, []string)
}

var Commands = make(map[string]Command)

func Register(name, category string, f func(*whatsmeow.Client, *events.Message, []string)) {
	Commands[name] = Command{Name: name, Category: category, Execute: f}
}