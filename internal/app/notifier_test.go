package app

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalNotifierWritesBellAndMessage(t *testing.T) {
	var output bytes.Buffer
	notifier := TerminalNotifier{Writer: &output}

	err := notifier.Notify(context.Background(), "Pomodoro complete", "focus block")

	require.NoError(t, err)
	require.Equal(t, "\aPomodoro complete: focus block\n", output.String())
}
