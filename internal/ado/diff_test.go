package ado

import (
	"testing"
)

// sampleDiff is a realistic unified diff output from `git diff`.
const sampleDiff = `diff --git a/internal/server/handler.go b/internal/server/handler.go
index abc1234..def5678 100644
--- a/internal/server/handler.go
+++ b/internal/server/handler.go
@@ -10,7 +10,10 @@ import (

 func handleLogin(w http.ResponseWriter, r *http.Request) {
 	username := r.FormValue("username")
-	query := "SELECT * FROM users WHERE name='" + username + "'"
+	password := r.FormValue("password")
+	row := db.QueryRow("SELECT id FROM users WHERE name=? AND password=?", username, password)
+	var userID int
+	if err := row.Scan(&userID); err != nil {
 		http.Error(w, "unauthorized", http.StatusUnauthorized)
 		return
 	}
diff --git a/internal/server/middleware.go b/internal/server/middleware.go
index 111aaaa..222bbbb 100644
--- a/internal/server/middleware.go
+++ b/internal/server/middleware.go
@@ -1,4 +1,6 @@
 package server
+
+// Authenticate wraps an http.Handler with session validation.
 func Authenticate(next http.Handler) http.Handler {
 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
 		next.ServeHTTP(w, r)
`

func TestParseUnifiedDiff_FileCount(t *testing.T) {
	files := parseUnifiedDiff(sampleDiff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestParseUnifiedDiff_FilePaths(t *testing.T) {
	files := parseUnifiedDiff(sampleDiff)
	if files[0].Path != "internal/server/handler.go" {
		t.Errorf("file[0].Path = %q, want %q", files[0].Path, "internal/server/handler.go")
	}
	if files[1].Path != "internal/server/middleware.go" {
		t.Errorf("file[1].Path = %q, want %q", files[1].Path, "internal/server/middleware.go")
	}
}

func TestParseUnifiedDiff_LineKinds(t *testing.T) {
	files := parseUnifiedDiff(sampleDiff)
	chunk := files[0].Chunks[0]

	// The deleted SQL line should be Kind=del with no LineNo.
	var delLine *DiffLine
	for i := range chunk.Lines {
		if chunk.Lines[i].Kind == "del" {
			delLine = &chunk.Lines[i]
			break
		}
	}
	if delLine == nil {
		t.Fatal("expected at least one deleted line in handler.go chunk")
	}
	if delLine.LineNo != 0 {
		t.Errorf("deleted line should have LineNo=0, got %d", delLine.LineNo)
	}

	// Added lines must have positive LineNo values.
	for _, l := range chunk.Lines {
		if l.Kind == "add" && l.LineNo <= 0 {
			t.Errorf("added line %q has LineNo=%d, want >0", l.Content, l.LineNo)
		}
	}
}

func TestParseUnifiedDiff_LineNumbers(t *testing.T) {
	files := parseUnifiedDiff(sampleDiff)
	chunk := files[0].Chunks[0]

	// Hunk starts at new-file line 10 per the @@ header.
	if chunk.NewStart != 10 {
		t.Errorf("chunk.NewStart = %d, want 10", chunk.NewStart)
	}

	// Collect added line numbers in order.
	var addedLines []int
	for _, l := range chunk.Lines {
		if l.Kind == "add" {
			addedLines = append(addedLines, l.LineNo)
		}
	}
	// There are 4 added lines starting from 12 (line 10 is context "func handleLogin",
	// line 11 is context "username :=", line 12 onward are the new additions).
	if len(addedLines) != 4 {
		t.Errorf("expected 4 added lines, got %d: %v", len(addedLines), addedLines)
	}
	// Added line numbers must be strictly increasing.
	for i := 1; i < len(addedLines); i++ {
		if addedLines[i] != addedLines[i-1]+1 {
			t.Errorf("added line numbers not contiguous: %v", addedLines)
		}
	}
}

func TestParseHunkStart(t *testing.T) {
	cases := []struct {
		header string
		want   int
	}{
		{"@@ -10,7 +10,8 @@ func main() {", 10},
		{"@@ -1,3 +1,4 @@", 1},
		{"@@ -0,0 +1 @@", 1}, // single-line addition at start of new file
		{"@@ -100,5 +200,3 @@", 200},
		{"@@ -1 +1 @@", 1}, // no comma (count omitted when 1)
	}

	for _, tc := range cases {
		got := parseHunkStart(tc.header)
		if got != tc.want {
			t.Errorf("parseHunkStart(%q) = %d, want %d", tc.header, got, tc.want)
		}
	}
}

func TestParseUnifiedDiff_EmptyInput(t *testing.T) {
	files := parseUnifiedDiff("")
	if len(files) != 0 {
		t.Errorf("expected 0 files for empty input, got %d", len(files))
	}
}
