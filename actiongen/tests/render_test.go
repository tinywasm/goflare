package actiongen_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tinywasm/goflare/actiongen"
)

func TestRenderIsDeterministic(t *testing.T) {
	a := actiongen.Action{
		Name:        "Test Action",
		Description: "A test action",
		Author:      "test",
		Inputs: []actiongen.Input{
			{Name: "input1", Description: "Input 1", Default: "val1", Required: true},
			{Name: "input2", Description: "Input 2", Default: "val2", Required: false},
		},
		Outputs: []actiongen.Output{
			{Name: "out1", Description: "Out 1", Value: "val1"},
			{Name: "out2", Description: "Out 2", Value: "val2"},
		},
		Steps: []actiongen.Step{
			{Name: "Step 1", Run: "echo step 1"},
			{Name: "Step 2", Uses: "actions/checkout@v4"},
			{Name: "Step 3", Shell: "bash", Run: "echo step 3"},
		},
	}

	res1 := a.Render()
	res2 := a.Render()

	if !bytes.Equal(res1, res2) {
		t.Fatal("expected Render() output to be deterministic across calls")
	}
}

func TestRenderOmitsEmptyFields(t *testing.T) {
	a := actiongen.Action{
		Name: "Minimal Action",
		Steps: []actiongen.Step{
			{Name: "Minimal Step", Run: "echo hello"},
		},
	}

	out := string(a.Render())

	if strings.Contains(out, "if:") {
		t.Errorf("expected output to omit 'if:', got: %s", out)
	}
	if strings.Contains(out, "uses:") {
		t.Errorf("expected output to omit 'uses:', got: %s", out)
	}
	if strings.Contains(out, "id:") {
		t.Errorf("expected output to omit 'id:', got: %s", out)
	}
}

func TestRenderMultilineRun(t *testing.T) {
	a := actiongen.Action{
		Steps: []actiongen.Step{
			{
				Name: "Multiline",
				Run:  "line 1\nline 2\nline 3",
			},
		},
	}

	out := string(a.Render())

	expected := "      run: |\n        line 1\n        line 2\n        line 3\n"
	if !strings.Contains(out, expected) {
		t.Errorf("expected output to contain correctly indented multiline run block:\n%s\nGot:\n%s", expected, out)
	}
}

func TestRenderEmitsComments(t *testing.T) {
	a := actiongen.Action{
		Steps: []actiongen.Step{
			{
				Name:    "Step with comment",
				Run:     "echo hi",
				Comment: "This is a step comment\nIt spans multiple lines",
			},
		},
	}

	out := string(a.Render())

	if !strings.Contains(out, "    # This is a step comment\n    # It spans multiple lines\n") {
		t.Errorf("expected output to contain formatted comment before step, got: %s", out)
	}
}

func TestRenderStartsWithGeneratedHeader(t *testing.T) {
	a := actiongen.Action{
		Name: "Test Header",
	}

	out := string(a.Render())

	if !strings.HasPrefix(out, actiongen.HeaderGenerated) {
		t.Errorf("expected output to start with HeaderGenerated, got: %s", out)
	}
}

func TestRenderEndsWithSingleNewline(t *testing.T) {
	a := actiongen.Action{
		Name: "Test End Newline",
	}

	out := string(a.Render())

	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("expected output to end with exactly one newline")
	}
}
