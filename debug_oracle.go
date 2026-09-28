package main
import (
	"fmt"
	"os"
	"regexp"
	"strings"
)
func main() {
	b, _ := os.ReadFile("organic-tests/lab/20260928T141612Z-seed-20260928153742/artifacts/META02/dotenv-only-stdout.txt")
	out := string(b)
	layerPattern := regexp.MustCompile(`(?m)DIAGNOSIS\s+The chain breaks at:\s+(.+)$`)
	match := layerPattern.FindStringSubmatch(out)
	if len(match) == 2 {
		fmt.Printf("FailingLayer: %q\n", strings.TrimSpace(match[1]))
	} else {
		fmt.Println("No match for FailingLayer")
	}
	
	failurePattern := regexp.MustCompile(`(?m) \[([a-z0-9_]+)\]$`)
	kinds := failurePattern.FindAllStringSubmatch(out, -1)
	if len(kinds) > 0 {
		fmt.Printf("FailureKind: %q\n", kinds[len(kinds)-1][1])
	} else {
		fmt.Println("No match for FailureKind")
	}
}
