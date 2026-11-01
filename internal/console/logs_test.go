package console

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLiveLogWriterFollowsLatestOutput(t *testing.T) {
	for _, width := range []int{40, 80} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			checkLiveLogTail(t, width)
		})
	}
}

func checkLiveLogTail(t *testing.T, width int) {
	t.Helper()
	reader, file, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	original := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = original }()
	writer := &liveLogWriter{title: "Build", rows: 10, width: width}
	for i := 0; i < 12; i++ {
		if _, err := fmt.Fprintf(writer, "step %02d %s\n", i, strings.Repeat("x", 40)); err != nil {
			t.Fatal(err)
		}
	}
	file.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	frames := strings.Split(string(output), "╭")
	latest := frames[len(frames)-1]
	if !strings.Contains(latest, "step 11") {
		t.Fatalf("latest log missing from final frame: %s", latest)
	}
	if strings.Contains(latest, "step 00") {
		t.Fatalf("old log still visible in final frame: %s", latest)
	}
}
