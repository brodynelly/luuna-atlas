package atlas

import (
	"strings"
	"testing"
	"time"
)

func TestParityArgumentEdges(t *testing.T) {
	reference := parityReference(t)
	refDir, goDir := parityProjects(t)
	cases := [][]string{
		{"config", "--json", "show"}, {"config", "--quiet", "schema"}, {"config", "--format", "plain", "schema"},
		{"record", "-tconvention", "--help"}, {"record", "--bad", "--help"}, {"record", "--bad", "--version"},
		{"record", "--type", "--help"}, {"record", "--type", "nope", "--help"},
		{"--", "-h"}, {"--", "--help"}, {"--", "--version"}, {"--", "query", "--json"},
		{"-qv"}, {"-q", "-v"}, {"-vq"}, {"query", "-qv"}, {"query", "-hq"},
		{"record", "foo", "--type=convention", "--content=hello", "--dry-run", "--json"},
		{"record", "foo", "--content", "-leading", "--help"},
		{"--bad", "--help"}, {"--bad", "query", "--help"}, {"query", "--", "--help"},
		{"config", "--bad", "--help"}, {"config", "bad", "--help"},
		{"query", "--json=true"}, {"query", "--json=false"}, {"query", "--format=ids"},
		{"config", "--json", "show", "--path", "search"}, {"config", "--json"},
		{"config", "help"}, {"config", "help", "show"}, {"config", "help", "bad"},
		{"record", "code", "-1", "--help"}, {"query", "-1"}, {"record", "code", "-1", "--type", "convention", "--dry-run", "--json"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			step := parityStep{Args: args}
			parityCompare(t, strings.Join(args, " "), parityRun(t, reference, refDir, step, false), parityRun(t, reference, goDir, step, true), refDir, goDir, time.Now())
		})
	}
}
