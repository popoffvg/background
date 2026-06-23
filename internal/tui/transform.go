package tui

import "strings"

// stripCommandBuffer turns an edited template buffer into a single shell
// command string: drops lines starting with '#' (after trimming leading
// whitespace) and blank lines, joins the remainder with newlines, trims.
func stripCommandBuffer(buf string) string {
	var lines []string
	for _, ln := range strings.Split(buf, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		lines = append(lines, ln)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// transformTemplate builds the pre-filled editor template for a task.
func transformTemplate(id, command string) string {
	return "# Transform the output of task " + id + " (" + command + ").\n" +
		"# The task's log file path is available in the $FILE environment variable.\n" +
		"# Example:  grep -i error \"$FILE\" | sort | uniq -c\n" +
		"# Lines starting with # are ignored. Write your shell command below, then save and quit.\n\n"
}

// editorArgv splits $EDITOR (fallback vi) into argv. Simple space split — does
// not handle quoted args (documented limitation).
func editorArgv(editorEnv string) []string {
	e := strings.TrimSpace(editorEnv)
	if e == "" {
		e = "vi"
	}
	return strings.Fields(e)
}
