package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestDockerComposeDetectionAndTargets(t *testing.T) {
	text := "services:\n  web:\n    image: nginx\n  db:\n    image: postgres\nnetworks:\n  public:\n"
	if !isComposeFile("deploy.yaml", text) || !isNamedComposeFile("Docker-Compose.YML") || isComposeFile("ordinary.yaml", "services:\n  foo:\n    unknown: x\n") {
		t.Fatal("incorrect compose detection")
	}
	if isComposeContent(strings.Repeat("# padding\n", 7000) + text) {
		t.Fatal("scan must be bounded")
	}
	targets := composeTargets(text)
	if len(targets) != 3 || targets[2].service != "db" || targets[2].line != 3 {
		t.Fatalf("targets: %#v", targets)
	}
	decl := composeDeclarations(text)
	if decl["services"]["web"].line != 1 || decl["networks"]["public"].line != 6 {
		t.Fatalf("declarations: %#v", decl)
	}
	stages := dockerfileTargets("FROM golang AS build\nRUN go build\nFROM scratch\n")
	if len(stages) != 2 || stages[0].stage != "build" || !stages[1].last || !isDockerfile("api.Containerfile") {
		t.Fatalf("stages: %#v", stages)
	}
}
func TestDockerComposeCommand(t *testing.T) {
	p := map[string]any{"files": "compose.yml; extra file.yml", "services": "web, db", "project": "my app", "build": "true", "upOptions": `--scale "web=2"`}
	command, stop, err := dockerRunCommand("docker-compose", p, false)
	if err != nil {
		t.Fatal(err)
	}
	if command != "docker compose -f compose.yml -f 'extra file.yml' -p 'my app' up --build --scale web=2 web db" || stop == nil || *stop != "docker compose -f compose.yml -f 'extra file.yml' -p 'my app' down" {
		t.Fatalf("%s / %v", command, stop)
	}
	p["detach"] = "true"
	_, stop, err = dockerRunCommand("docker-compose", p, false)
	if err != nil || stop != nil {
		t.Fatalf("detached: %v %v", stop, err)
	}
	command, stop, err = dockerRunCommand("docker-dockerfile", map[string]any{"dockerfile": "app/Dockerfile", "runImage": "true", "ports": "8080:80", "buildOptions": `--label "hello=world wide"`}, false)
	if err != nil || !strings.Contains(command, "--label 'hello=world wide' app &&") || stop == nil || *stop != "docker stop app" {
		t.Fatalf("%s %v %v", command, stop, err)
	}
	args, err := dockerWords(`a "b c" 'd e'`)
	if err != nil || !reflect.DeepEqual(args, []string{"a", "b c", "d e"}) {
		t.Fatalf("%v %v", args, err)
	}
}
