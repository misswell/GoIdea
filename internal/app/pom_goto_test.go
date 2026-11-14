package app

import (
	"strings"
	"testing"
)

const pomGotoSample = `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <properties>
    <example.version>1.0</example.version>
  </properties>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.example</groupId>
        <artifactId>managed</artifactId>
        <version>2.0</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>lib</artifactId>
      <version>${example.version}</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <artifactId>example-plugin</artifactId>
      </plugin>
    </plugins>
  </build>
</project>
`

func TestPropertyRefAt(t *testing.T) {
	text := pomGotoSample
	at := strings.Index(text, "${example.version}")
	for _, pos := range []int{at, at + 1, at + 5, at + len("${example.version}") - 1} {
		from, to, ok := propertyRefAt(text, pos)
		if !ok || from != at || to != at+len("${example.version}") {
			t.Fatalf("pos %d: ref (%d,%d,%v)", pos, from, to, ok)
		}
	}
	if _, _, ok := propertyRefAt(text, at+len("${example.version}")); ok {
		t.Fatal("past the closing brace should not resolve")
	}
	if _, _, ok := propertyRefAt("a ${unclosed", 3); ok {
		t.Fatal("unclosed reference should not resolve")
	}
}

func TestPomGotoEligibility(t *testing.T) {
	text := pomGotoSample
	at := func(s string) int { return strings.Index(text, s) }
	cases := []struct {
		name string
		pos  int
		want bool
	}{
		{"property reference", at("${example.version}"), true},
		{"dependency id", at("<artifactId>lib<"), true},
		{"managed dependency", at("<artifactId>managed<"), true},
		{"plugin id", at("<artifactId>example-plugin<"), true},
		{"project root", at("<project>"), false},
		{"properties block", at("<properties>"), false},
		{"dependencyManagement outside an entry", at("<dependencyManagement>"), false},
	}
	for _, c := range cases {
		if got := pomGotoEligible(text, c.pos); got != c.want {
			t.Errorf("%s: eligible=%v want %v", c.name, got, c.want)
		}
	}
}

func TestPomGotoIgnoresNonPomFiles(t *testing.T) {
	if !isPomPath("/p/app/pom.xml") || isPomPath("/p/app/build.gradle") {
		t.Fatal("pom path detection")
	}
}
