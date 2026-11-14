package app

import "testing"

func TestUTF16OffsetConversions(t *testing.T) {
	text := "a😀b世界c"
	// bytes: a=1, 😀=4 (two UTF-16 units), b=1, 世=3, 界=3, c=1
	if got := byteToUTF16Offset(text, 0); got != 0 {
		t.Fatalf("start = %d", got)
	}
	if got := byteToUTF16Offset(text, 1); got != 1 {
		t.Fatalf("after a = %d", got)
	}
	if got := byteToUTF16Offset(text, 5); got != 3 {
		t.Fatalf("after emoji = %d", got)
	}
	if got := utf16ToByteOffset(text, 1); got != 1 {
		t.Fatalf("u1 byte = %d", got)
	}
	if got := utf16ToByteOffset(text, 3); got != 5 {
		t.Fatalf("u3 byte = %d", got)
	}
	if got := utf16ToByteOffset(text, 99); got != len(text) {
		t.Fatalf("past end = %d", got)
	}
	if !isPomPath("/p/pom.xml") || !isPomPath("pom.xml") || isPomPath("/p/pom.xml.bak") {
		t.Fatal("pom path detection")
	}
}
