package main

import (
	"strings"
	"testing"
)

func TestHelpTextCoversCommands(t *testing.T) {
	for topic, text := range commandHelp {
		if !strings.Contains(text, "Usage: timetracker") {
			t.Errorf("help topic %q has no usage line", topic)
		}
	}
}

func TestHelpTextRejectsUnknownTopic(t *testing.T) {
	if _, err := helpText("not-a-command"); err == nil {
		t.Fatal("helpText accepted an unknown topic")
	}
}
