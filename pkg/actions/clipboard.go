package actions

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// CopyToClipboard copies text to the system clipboard.
// Detects the platform and available clipboard tool.
func CopyToClipboard(text string) error {
	name, args := clipboardCommand()
	if name == "" {
		return fmt.Errorf("no clipboard tool found — install xclip, xsel, or wl-copy")
	}

	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard copy failed (%s): %w", name, err)
	}
	return nil
}

// clipboardCommand returns the clipboard command and args for the current platform.
func clipboardCommand() (string, []string) {
	if runtime.GOOS == "darwin" {
		return "pbcopy", nil
	}

	// Linux: try wayland first, then X11 tools
	for _, candidate := range []struct {
		name string
		args []string
	}{
		{"wl-copy", nil},
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
	} {
		if _, err := exec.LookPath(candidate.name); err == nil {
			return candidate.name, candidate.args
		}
	}

	return "", nil
}
