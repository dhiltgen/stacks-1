package e2e

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/assert"
	"gotest.tools/icmd"
)

const (
	GoldenOutFilename = "output.regexp-golden"
)

func TestRunCLIScenarios(t *testing.T) {
	// Find all the test fixtures with the expected files to run
	err := filepath.Walk("./pkg/compose/tests/fixtures/", func(path string, info os.FileInfo, err error) error {

		// Ignore directories
		if info != nil && info.IsDir() {
			return nil
		}
		filename := filepath.Base(path)
		if filename == "docker-compose.yml" || filename == "docker-compose.yaml" {
			// Use the parent dir as the testcase name
			tcName := filepath.Base(filepath.Dir(path))
			t.Run(tcName, func(t *testing.T) {
				runScenario(t, path)
			})

		}
		return nil
	})
	assert.NilError(t, err)
}

func runScenario(t *testing.T, composeFilename string) {
	// Check for golden files
	dir := filepath.Dir(composeFilename)
	goldenFilename, err := filepath.Abs(filepath.Join(dir, GoldenOutFilename))
	assert.NilError(t, err)
	_, err = os.Stat(goldenFilename)
	if err != nil && os.IsNotExist(err) {
		t.Skipf("%s does not contain %s", dir, GoldenOutFilename)
	}
	var out strings.Builder
	// Scenario:
	stackname := filepath.Base(dir)
	// * Run through a standard set of operations with a single aggregate stdout/stderr output - w/marker for each command
	// * docker stack deploy $path $name
	cmd := []string{"stack", "deploy", "-c", composeFilename, stackname}
	result := icmd.RunCommand("docker", cmd...)
	defer icmd.RunCommand("docker", "stack", "rm",
		stackname)
	result.Assert(t, icmd.Success)
	out.WriteString("CMD: " + strings.Join(cmd, " ") + "\n")
	out.WriteString(result.Combined())
	// * Loop for $timeout until docker stack service matches golden
	// TODO - need to support converging until it works
	cmd = []string{"stack", "services", stackname}
	result = icmd.RunCommand("docker", cmd...)
	result.Assert(t, icmd.Success)
	out.WriteString("\nCMD: " + strings.Join(cmd, " ") + "\n")
	out.WriteString(result.Combined())
	// * docker stack ps
	// TODO - need to support converging until it works

	/* TODO - this doesn't work yet since it's not wired up on the backend...
	cmd = []string{"stack", "ps", stackname}
	result = icmd.RunCommand("docker", cmd...)
	result.Assert(t, icmd.Success)
	out.WriteString("\nCMD: " + strings.Join(cmd, " ") + "\n")
	out.WriteString(result.Combined())
	*/
	// * Loop for $timeout until docker stack inspect $name matches golden
	cmd = []string{"stack", "inspect", stackname}
	result = icmd.RunCommand("docker", cmd...)
	result.Assert(t, icmd.Success)
	out.WriteString("\nCMD: " + strings.Join(cmd, " ") + "\n")
	out.WriteString(result.Combined())

	goldenData, err := ioutil.ReadFile(goldenFilename)
	assert.NilError(t, err)
	same, errstr := RegexpGoldenCompare(strings.TrimSpace(out.String()), strings.TrimSpace(string(goldenData)))
	assert.Assert(t, same,
		"Mismatch starts at: %s\nActual: %s\n", errstr, out.String())
}

// TODO consider extracting the rest of this out as a more re-usable lib
const (
	START = "{{"
	END   = "}}"
)

// RegexpGoldenCompare takes two input strings to compare
// the expected string can contain regular expressions within
// '{{' '}}' blocks which will be evaluated during comparison
// using the regexp library
// If there's a mismatch, the string will summarize where the difference starts
func RegexpGoldenCompare(input, expected string) (bool, string) {
	// Find the first regexp block in expected
	startI := strings.Index(expected, START)
	if startI < 0 {
		// No regexp, just do straight comparison
		cmp := strings.Compare(input, expected) == 0
		if !cmp {
			return cmp, input
		}
		return cmp, ""
	} else if startI > 0 {
		// Some prefix without regexp to compare first
		if strings.Compare(input[0:startI], expected[0:startI]) != 0 {
			return false, input
		}
	}
	endI := strings.Index(expected, END)
	if endI < 0 {
		// Malformed
		return false, input
	}
	re, err := regexp.Compile(expected[startI+len(START) : endI])
	if err != nil {
		// Malformed
		return false, input[startI:]
	}
	match := re.FindStringIndex(input[startI:])
	if match == nil {
		// Not matched at all
		return false, input[startI:]
	}
	if match[0] != 0 {
		// Match doesn't start where it's supposed to
		return false, input[startI:]
	}
	// Match worked, now recurse for the rest of the input
	return RegexpGoldenCompare(input[startI+match[1]:], expected[endI+2:])
}
