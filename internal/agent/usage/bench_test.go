package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkRead measures the reader over a synthetic corpus of about 64 MiB
// (assistant lines with a long content block, as real transcripts have) and
// reports MB/s. The performance budget of issue #29 is 3 s for 1 GB, that is
// about 333 MB/s.
func BenchmarkRead(b *testing.B) {
	dir := b.TempDir()
	pad := strings.Repeat("x", 3000)
	var size int64
	for f := 0; f < 16; f++ {
		var sb strings.Builder
		for i := 0; i < 1000; i++ {
			fmt.Fprintf(&sb, `{"type":"assistant","timestamp":"2026-11-20T09:%02d:00.000Z","sessionId":"s%d","cwd":"/w/p%d","entrypoint":"cli","requestId":"r%d-%d","message":{"id":"m%d-%d","role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"%s"}],"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":1,"cache_read_input_tokens":100}}}`+"\n",
				i%60, f, f%4, f, i, f, i, pad)
		}
		size += int64(sb.Len())
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("s%d.jsonl", f)), []byte(sb.String()), 0o644); err != nil {
			b.Fatal(err)
		}
		// Plus lines that carry no usage, which the reader must skip cheaply.
		noise := strings.Repeat(`{"type":"user","message":{"content":"`+pad+`"}}`+"\n", 1000)
		size += int64(len(noise))
		fh, _ := os.OpenFile(filepath.Join(dir, fmt.Sprintf("s%d.jsonl", f)), os.O_APPEND|os.O_WRONLY, 0o644)
		fh.WriteString(noise)
		fh.Close()
	}
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := Read([]Root{{Dir: dir, Source: "claude"}})
		if err != nil || len(c.Entries) != 16000 {
			b.Fatalf("entries = %d, err %v", len(c.Entries), err)
		}
	}
}
